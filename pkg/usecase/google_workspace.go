package usecase

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/utils/errutil"
)

const googleWorkspaceCallbackPath = "/api/v1/integrations/google-workspace/callback"

// googleWorkspaceScopes is requested at connect. The identity scopes are not
// checked against the granted list; FetchIdentity proves them instead.
var googleWorkspaceScopes = []string{
	"openid",
	"email",
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/gmail.readonly",
}

// googleWorkspaceRequiredScopes must all be granted, or the connection is
// rejected. Google lets a user uncheck individual scopes on the consent
// screen.
var googleWorkspaceRequiredScopes = []string{
	"https://www.googleapis.com/auth/calendar.readonly",
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/gmail.readonly",
}

type GoogleWorkspaceConfig struct {
	BaseURL string // scheme://host[:port], no trailing slash
}

func (c GoogleWorkspaceConfig) callbackURL() string {
	return c.BaseURL + googleWorkspaceCallbackPath
}

type GoogleWorkspaceStatus struct {
	Connected bool
	Email     string // empty when not connected
}

// GoogleWorkspaceUseCase connects and disconnects a user's Google Workspace
// account. It only obtains and stores the grant; reading Calendar, Drive, or
// Gmail with it is not implemented yet.
type GoogleWorkspaceUseCase struct {
	oauth  interfaces.GoogleOAuth
	access *GoogleWorkspaceAccess
	cfg    GoogleWorkspaceConfig
	now    func() time.Time
}

func NewGoogleWorkspaceUseCase(oauth interfaces.GoogleOAuth, access *GoogleWorkspaceAccess, cfg GoogleWorkspaceConfig) *GoogleWorkspaceUseCase {
	return &GoogleWorkspaceUseCase{oauth: oauth, access: access, cfg: cfg, now: time.Now}
}

// AuthorizeURL returns Google's authorization URL for key. A user who already
// has a connected account gets ErrGoogleWorkspaceAlreadyConnected: the account
// has to be disconnected before another connection is made.
func (uc *GoogleWorkspaceUseCase) AuthorizeURL(ctx context.Context, key model.UserKey, state string) (string, error) {
	if err := uc.ensureNotConnected(ctx, key); err != nil {
		return "", err
	}
	return uc.oauth.AuthorizeURL(state, uc.cfg.callbackURL(), googleWorkspaceScopes), nil
}

func (uc *GoogleWorkspaceUseCase) ensureNotConnected(ctx context.Context, key model.UserKey) error {
	status, err := uc.access.Connection(ctx, key)
	if err != nil {
		return err
	}
	if status.Connected {
		return goerr.Wrap(ErrGoogleWorkspaceAlreadyConnected, "google workspace is already connected",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

// HandleCallback exchanges code, checks the grant, and stores the refresh
// token for key.
//
// Google revokes a grant per Google account, not per token, so a rejected
// grant is revoked only once its account is known not to be connected to
// anyone: revoking it otherwise would end that connection. The account
// already connected to key is protected by refusing the callback before the
// exchange; an account connected to another user is protected by
// AccountInUse. When the account cannot be identified, nothing is revoked.
func (uc *GoogleWorkspaceUseCase) HandleCallback(ctx context.Context, key model.UserKey, code string) error {
	if err := uc.ensureNotConnected(ctx, key); err != nil {
		return err
	}

	res, err := uc.oauth.ExchangeCode(ctx, code, uc.cfg.callbackURL())
	if err != nil {
		return goerr.Wrap(err, "failed to exchange google oauth code",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	vals := []goerr.Option{
		goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID),
		goerr.V("granted_scopes", res.Scopes),
	}

	identity, err := uc.oauth.FetchIdentity(ctx, res.AccessToken)
	if err != nil {
		return goerr.Wrap(err, "failed to fetch google account identity", vals...)
	}
	if identity.Subject == "" || identity.Email == "" {
		return goerr.Wrap(ErrGoogleConnectRejected, "google account identity is incomplete",
			append(vals, goerr.V("has_subject", identity.Subject != ""), goerr.V("has_email", identity.Email != ""))...)
	}

	inUse, err := uc.access.AccountInUse(ctx, key, identity.Subject)
	if err != nil {
		return goerr.Wrap(err, "failed to check the google account", vals...)
	}
	if inUse {
		return goerr.Wrap(ErrGoogleAccountInUse, "google account is connected to another user", vals...)
	}

	var missing []string
	for _, scope := range googleWorkspaceRequiredScopes {
		if !slices.Contains(res.Scopes, scope) {
			missing = append(missing, scope)
		}
	}
	if len(missing) > 0 {
		return uc.reject(ctx, res, goerr.Wrap(ErrGoogleScopeNotGranted, "required google scope was not granted",
			append(vals, goerr.V("missing_scopes", missing))...))
	}
	if res.RefreshToken == "" {
		return uc.reject(ctx, res, goerr.Wrap(ErrGoogleConnectRejected, "google returned no refresh token", vals...))
	}

	err = uc.access.Store(ctx, key, res.RefreshToken, res.Scopes, identity, uc.now())
	switch {
	case err == nil:
		return nil
	// Another request connected an account in the meantime; the grant may
	// belong to that connection, so it is not revoked.
	case errors.Is(err, interfaces.ErrAlreadyExists):
		return goerr.Wrap(ErrGoogleWorkspaceAlreadyConnected, "google workspace was connected by another request", vals...)
	case errors.Is(err, interfaces.ErrGoogleAccountInUse):
		return goerr.Wrap(ErrGoogleAccountInUse, "google account was connected to another user by another request", vals...)
	default:
		return uc.reject(ctx, res, goerr.Wrap(err, "failed to store google refresh token", vals...))
	}
}

// reject revokes the grant res came with and returns cause. Revoking the
// refresh token also ends the access token; without one, the access token is
// revoked. A failed revocation is recorded here because cause is what the
// caller needs. Callers must have made sure that the grant's Google account is
// not connected to any user.
func (uc *GoogleWorkspaceUseCase) reject(ctx context.Context, res *model.GoogleOAuthResult, cause error) error {
	token := string(res.RefreshToken)
	if token == "" {
		token = string(res.AccessToken)
	}
	if err := uc.oauth.Revoke(ctx, token); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to revoke a rejected google grant"), "google grant may remain")
	}
	return cause
}

// Status reads only the stored credential. It does not ask Google whether the
// token is still valid.
func (uc *GoogleWorkspaceUseCase) Status(ctx context.Context, key model.UserKey) (*GoogleWorkspaceStatus, error) {
	return uc.access.Connection(ctx, key)
}

// Disconnect revokes the stored grant at Google and then deletes it. When
// Google cannot be reached, the credential is kept so that the page never
// shows the account as disconnected while Google still honors the grant.
func (uc *GoogleWorkspaceUseCase) Disconnect(ctx context.Context, key model.UserKey) error {
	token, err := uc.access.Token(ctx, key)
	if errors.Is(err, ErrGoogleWorkspaceNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := uc.oauth.Revoke(ctx, string(token.RefreshToken)); err != nil {
		if !errors.Is(err, interfaces.ErrGoogleTokenInvalid) {
			return goerr.Wrap(err, "failed to revoke google refresh token",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		errutil.Handle(ctx, goerr.Wrap(err, "google refresh token was already invalid",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.T(errutil.TagBenign)),
			"deleting an invalid google refresh token")
	}

	return uc.access.Delete(ctx, token)
}
