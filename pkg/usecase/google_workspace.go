package usecase

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/utils/errutil"
)

const googleWorkspaceCallbackPath = "/api/integrations/google-workspace/callback"

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

func (uc *GoogleWorkspaceUseCase) AuthorizeURL(state string) string {
	return uc.oauth.AuthorizeURL(state, uc.cfg.callbackURL(), googleWorkspaceScopes)
}

// HandleCallback exchanges code, checks the grant, and stores the refresh
// token for key. A grant that is rejected after the exchange is revoked at
// Google so no unused grant remains, and nothing is stored.
func (uc *GoogleWorkspaceUseCase) HandleCallback(ctx context.Context, key model.UserKey, code string) error {
	res, err := uc.oauth.ExchangeCode(ctx, code, uc.cfg.callbackURL())
	if err != nil {
		return goerr.Wrap(err, "failed to exchange google oauth code",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	vals := []goerr.Option{
		goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID),
		goerr.V("granted_scopes", res.Scopes),
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

	identity, err := uc.oauth.FetchIdentity(ctx, res.AccessToken)
	if err != nil {
		return uc.reject(ctx, res, goerr.Wrap(err, "failed to fetch google account identity", vals...))
	}
	if identity.Subject == "" || identity.Email == "" {
		return uc.reject(ctx, res, goerr.Wrap(ErrGoogleConnectRejected, "google account identity is incomplete",
			append(vals, goerr.V("has_subject", identity.Subject != ""), goerr.V("has_email", identity.Email != ""))...))
	}

	if err := uc.access.Store(ctx, key, res.RefreshToken, res.Scopes, identity, uc.now()); err != nil {
		return uc.reject(ctx, res, goerr.Wrap(err, "failed to store google refresh token", vals...))
	}
	return nil
}

// reject revokes the grant res came with and returns cause. Revoking the
// refresh token also ends the access token; without one, the access token is
// revoked. A failed revocation is recorded here because cause is what the
// caller needs.
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
