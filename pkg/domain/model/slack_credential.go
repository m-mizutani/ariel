package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// EncryptedData is a ciphertext produced by Cloud KMS. KeyName is the crypto
// key (not the key version) used for encryption; KMS selects the version from
// the ciphertext on decryption.
type EncryptedData struct {
	KeyName    string
	Ciphertext []byte
}

func (x *EncryptedData) Validate() error {
	if x.KeyName == "" {
		return goerr.New("empty key name")
	}
	if len(x.Ciphertext) == 0 {
		return goerr.New("empty ciphertext")
	}
	return nil
}

// SlackCredential holds one user's Slack user token as KMS ciphertext and the
// scopes Slack granted with it.
type SlackCredential struct {
	TeamID      SlackTeamID
	UserID      SlackUserID
	AccessToken EncryptedData
	Scopes      []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c *SlackCredential) Key() UserKey {
	return UserKey{TeamID: c.TeamID, UserID: c.UserID}
}

func (c *SlackCredential) Validate() error {
	if err := c.Key().Validate(); err != nil {
		return goerr.Wrap(err, "invalid slack credential key")
	}
	if err := c.AccessToken.Validate(); err != nil {
		return goerr.Wrap(err, "invalid slack credential access token")
	}
	if len(c.Scopes) == 0 {
		return goerr.New("empty slack credential scopes")
	}
	if c.CreatedAt.IsZero() {
		return goerr.New("empty slack credential created_at")
	}
	if c.UpdatedAt.IsZero() {
		return goerr.New("empty slack credential updated_at")
	}
	return nil
}
