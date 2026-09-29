package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// GoogleRefreshToken is a plaintext Google refresh token. It is never
// persisted and never logged; only its KMS ciphertext is stored.
type GoogleRefreshToken string

// GoogleAccessToken is a plaintext Google access token. It is never persisted
// and never logged.
type GoogleAccessToken string

// GoogleOAuthResult is what Google's token endpoint returns for an
// authorization code.
type GoogleOAuthResult struct {
	AccessToken GoogleAccessToken `masq:"secret"`
	// RefreshToken is empty when Google returned none.
	RefreshToken GoogleRefreshToken `masq:"secret"`
	Scopes       []string
}

// GoogleIdentity is the Google account a token belongs to, as reported by the
// OpenID Connect userinfo endpoint.
type GoogleIdentity struct {
	// Subject is the "sub" claim: unique among Google accounts and never
	// reused.
	Subject string
	Email   string
}

// GoogleWorkspaceCredential holds one user's Google refresh token as KMS
// ciphertext, the scopes Google granted with it, and the Google account it
// belongs to.
type GoogleWorkspaceCredential struct {
	TeamID       SlackTeamID
	UserID       SlackUserID
	RefreshToken EncryptedData
	Scopes       []string
	Subject      string
	Email        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GoogleWorkspaceAccount records which user a Google account is connected to,
// so that one Google account is never connected to two users: Google revokes
// a grant per Google account and project, and one user's disconnection would
// end the other user's access.
type GoogleWorkspaceAccount struct {
	Subject   string
	TeamID    SlackTeamID
	UserID    SlackUserID
	CreatedAt time.Time
}

func (a *GoogleWorkspaceAccount) Key() UserKey {
	return UserKey{TeamID: a.TeamID, UserID: a.UserID}
}

func (c *GoogleWorkspaceCredential) Key() UserKey {
	return UserKey{TeamID: c.TeamID, UserID: c.UserID}
}

func (c *GoogleWorkspaceCredential) Validate() error {
	if err := c.Key().Validate(); err != nil {
		return goerr.Wrap(err, "invalid google workspace credential key")
	}
	if err := c.RefreshToken.Validate(); err != nil {
		return goerr.Wrap(err, "invalid google workspace credential refresh token")
	}
	if len(c.Scopes) == 0 {
		return goerr.New("empty google workspace credential scopes")
	}
	if c.Subject == "" {
		return goerr.New("empty google workspace credential subject")
	}
	if c.Email == "" {
		return goerr.New("empty google workspace credential email")
	}
	if c.CreatedAt.IsZero() {
		return goerr.New("empty google workspace credential created_at")
	}
	if c.UpdatedAt.IsZero() {
		return goerr.New("empty google workspace credential updated_at")
	}
	return nil
}
