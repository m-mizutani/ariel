package usecase

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
)

const slackAuthorizeURL = "https://slack.com/oauth/v2/authorize"

// slackUserScopes is the user scope list requested at login. It must match
// oauth_config.scopes.user in docs/slack-app-manifest.yaml.
var slackUserScopes = []string{"search:read"}

type AuthConfig struct {
	ClientID   string
	BaseURL    string // scheme://host[:port], no trailing slash
	TeamID     model.SlackTeamID
	SessionTTL time.Duration
	// NoAuthUserID, when set, disables Slack authorization: every sign-in
	// becomes this user of TeamID. It exists for E2E tests and local
	// development; the CLI accepts it only with the in-memory repository.
	NoAuthUserID model.SlackUserID
}

// noAuthCode is the authorization code AuthorizeURL puts into the callback URL
// in no-auth mode. HandleCallback does not check it; the callback still goes
// through the state cookie check and session creation.
const noAuthCode = "no-auth"

func (c AuthConfig) callbackURL() string {
	return c.BaseURL + "/api/v1/auth/callback"
}

type AuthUseCase struct {
	repo        interfaces.Repository
	oauth       interfaces.SlackOAuth
	bot         interfaces.SlackBot
	access      *SlackUserAccess
	userClients interfaces.SlackUserClientFactory
	cfg         AuthConfig
	now         func() time.Time
}

func NewAuthUseCase(repo interfaces.Repository, oauth interfaces.SlackOAuth, bot interfaces.SlackBot,
	access *SlackUserAccess, userClients interfaces.SlackUserClientFactory, cfg AuthConfig) *AuthUseCase {
	return &AuthUseCase{
		repo:        repo,
		oauth:       oauth,
		bot:         bot,
		access:      access,
		userClients: userClients,
		cfg:         cfg,
		now:         time.Now,
	}
}

// AuthorizeURL returns the Slack OAuth v2 authorization URL. Only user scopes
// are requested; the bot token is installed separately by an administrator.
func (uc *AuthUseCase) AuthorizeURL(state string) string {
	if uc.cfg.NoAuthUserID != "" {
		return uc.cfg.callbackURL() + "?" + url.Values{"code": {noAuthCode}, "state": {state}}.Encode()
	}

	params := url.Values{}
	params.Set("client_id", uc.cfg.ClientID)
	params.Set("user_scope", strings.Join(slackUserScopes, ","))
	params.Set("redirect_uri", uc.cfg.callbackURL())
	params.Set("state", state)
	params.Set("team", string(uc.cfg.TeamID))
	return slackAuthorizeURL + "?" + params.Encode()
}

// HandleCallback completes a login: it exchanges the code for the user's
// token, checks that the token belongs to the authorizing user of the
// configured workspace, stores the user and the encrypted token, and creates
// a web session. Nothing is stored when a check fails.
func (uc *AuthUseCase) HandleCallback(ctx context.Context, code string) (*auth.Session, auth.SessionSecret, error) {
	if uc.cfg.NoAuthUserID != "" {
		return uc.signInWithoutSlack(ctx)
	}

	res, err := uc.oauth.ExchangeCode(ctx, code, uc.cfg.callbackURL())
	if err != nil {
		return nil, "", goerr.Wrap(err, "failed to exchange slack oauth code")
	}

	key := model.UserKey{TeamID: res.TeamID, UserID: res.UserID}
	if err := uc.verifyOAuthResult(res, key); err != nil {
		return nil, "", err
	}

	identity, err := uc.userClients.New(res.AccessToken).AuthTest(ctx)
	if err != nil {
		return nil, "", goerr.Wrap(err, "failed to verify slack user token")
	}
	if identity.TeamID != key.TeamID || identity.UserID != key.UserID {
		return nil, "", goerr.Wrap(ErrLoginRejected, "auth.test identity differs from the oauth result",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID),
			goerr.V("auth_test_team_id", identity.TeamID), goerr.V("auth_test_user_id", identity.UserID))
	}

	name, err := uc.bot.GetUserName(ctx, key.UserID)
	if err != nil {
		return nil, "", goerr.Wrap(err, "failed to get slack user name")
	}

	now := uc.now()
	if err := uc.saveUser(ctx, key, name, now); err != nil {
		return nil, "", err
	}
	if err := uc.access.Store(ctx, key, res.AccessToken, res.Scopes, now); err != nil {
		return nil, "", goerr.Wrap(err, "failed to store slack user token")
	}
	return uc.createSession(ctx, key, now)
}

// signInWithoutSlack signs in the configured no-auth user. No Slack token is
// obtained, so the user is shown as not linked to Slack.
func (uc *AuthUseCase) signInWithoutSlack(ctx context.Context) (*auth.Session, auth.SessionSecret, error) {
	key := model.UserKey{TeamID: uc.cfg.TeamID, UserID: uc.cfg.NoAuthUserID}
	now := uc.now()
	if err := uc.saveUser(ctx, key, string(key.UserID), now); err != nil {
		return nil, "", err
	}
	return uc.createSession(ctx, key, now)
}

func (uc *AuthUseCase) createSession(ctx context.Context, key model.UserKey, now time.Time) (*auth.Session, auth.SessionSecret, error) {
	secret := auth.NewSessionSecret()
	session := &auth.Session{
		ID:         auth.NewSessionID(),
		SecretHash: secret.Hash(),
		TeamID:     key.TeamID,
		UserID:     key.UserID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(uc.cfg.SessionTTL),
	}
	if err := uc.repo.Session().Create(ctx, session); err != nil {
		return nil, "", goerr.Wrap(err, "failed to create session")
	}
	return session, secret, nil
}

func (uc *AuthUseCase) verifyOAuthResult(res *model.SlackOAuthResult, key model.UserKey) error {
	vals := []goerr.Option{goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID)}
	if res.TeamID != uc.cfg.TeamID {
		return goerr.Wrap(ErrLoginRejected, "authorization is for another workspace",
			append(vals, goerr.V("expected_team_id", uc.cfg.TeamID))...)
	}
	if err := key.Validate(); err != nil {
		return goerr.Wrap(ErrLoginRejected, "invalid slack user key in oauth result", vals...)
	}
	if res.TokenType != "user" {
		return goerr.Wrap(ErrLoginRejected, "oauth result is not a user token",
			append(vals, goerr.V("token_type", res.TokenType))...)
	}
	if res.AccessToken == "" {
		return goerr.Wrap(ErrLoginRejected, "oauth result has no user token", vals...)
	}
	for _, scope := range slackUserScopes {
		if !slices.Contains(res.Scopes, scope) {
			return goerr.Wrap(ErrLoginRejected, "required user scope was not granted",
				append(vals, goerr.V("missing_scope", scope), goerr.V("granted_scopes", res.Scopes))...)
		}
	}
	return nil
}

func (uc *AuthUseCase) saveUser(ctx context.Context, key model.UserKey, name string, now time.Time) error {
	createdAt := now
	existing, err := uc.repo.User().Get(ctx, key)
	switch {
	case err == nil:
		createdAt = existing.CreatedAt
	case !errors.Is(err, interfaces.ErrNotFound):
		return goerr.Wrap(err, "failed to load user")
	}

	user := &model.User{
		TeamID:    key.TeamID,
		UserID:    key.UserID,
		Name:      name,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	if err := uc.repo.User().Put(ctx, user); err != nil {
		return goerr.Wrap(err, "failed to save user")
	}
	return nil
}

// Authenticate returns the session when id exists, has not expired, and
// secret matches. Every failure of that kind is ErrUnauthenticated.
func (uc *AuthUseCase) Authenticate(ctx context.Context, id auth.SessionID, secret auth.SessionSecret) (*auth.Session, error) {
	if err := id.Validate(); err != nil {
		return nil, goerr.Wrap(ErrUnauthenticated, "malformed session ID")
	}
	session, err := uc.repo.Session().Get(ctx, id)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			return nil, goerr.Wrap(ErrUnauthenticated, "session not found", goerr.V("session_id", id))
		}
		return nil, goerr.Wrap(err, "failed to load session")
	}
	if session.IsExpired(uc.now()) {
		return nil, goerr.Wrap(ErrUnauthenticated, "session expired", goerr.V("session_id", id))
	}
	if !session.VerifySecret(secret) {
		return nil, goerr.Wrap(ErrUnauthenticated, "session secret mismatch", goerr.V("session_id", id))
	}
	return session, nil
}

// Logout deletes the session of this browser. Only the holder of both the
// session ID and its secret can end the session; anything else is ignored.
// The Slack user token is kept, because mentions in Slack keep arriving
// regardless of the browser state.
func (uc *AuthUseCase) Logout(ctx context.Context, id auth.SessionID, secret auth.SessionSecret) error {
	session, err := uc.Authenticate(ctx, id, secret)
	if errors.Is(err, ErrUnauthenticated) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := uc.repo.Session().Delete(ctx, session.ID); err != nil {
		return goerr.Wrap(err, "failed to delete session")
	}
	return nil
}

type Me struct {
	TeamID         model.SlackTeamID
	UserID         model.SlackUserID
	Name           string
	SlackConnected bool
}

func (uc *AuthUseCase) Me(ctx context.Context, key model.UserKey) (*Me, error) {
	user, err := uc.repo.User().Get(ctx, key)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to load user")
	}
	connected, err := uc.access.Connected(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Me{
		TeamID:         user.TeamID,
		UserID:         user.UserID,
		Name:           user.Name,
		SlackConnected: connected,
	}, nil
}
