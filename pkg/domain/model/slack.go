package model

import (
	"regexp"

	"github.com/m-mizutani/goerr/v2"
)

// SlackTeamID is a Slack workspace ID such as "T0123ABCD".
type SlackTeamID string

// SlackUserID is a Slack user ID such as "U0123ABCD" or "W0123ABCD".
type SlackUserID string

// SlackUserToken is a plaintext Slack user token. It is never persisted and
// never logged; only its KMS ciphertext is stored.
type SlackUserToken string

var (
	slackTeamIDPattern = regexp.MustCompile(`^T[A-Z0-9]+$`)
	slackUserIDPattern = regexp.MustCompile(`^[UW][A-Z0-9]+$`)
)

func (x SlackTeamID) Validate() error {
	if !slackTeamIDPattern.MatchString(string(x)) {
		return goerr.New("invalid slack team ID", goerr.V("team_id", string(x)))
	}
	return nil
}

func (x SlackUserID) Validate() error {
	if !slackUserIDPattern.MatchString(string(x)) {
		return goerr.New("invalid slack user ID", goerr.V("user_id", string(x)))
	}
	return nil
}

// UserKey identifies one user. Every piece of data that belongs to a user is
// stored under the document path derived from this key.
type UserKey struct {
	TeamID SlackTeamID
	UserID SlackUserID
}

func (k UserKey) Validate() error {
	if err := k.TeamID.Validate(); err != nil {
		return err
	}
	if err := k.UserID.Validate(); err != nil {
		return err
	}
	return nil
}

// SlackOAuthResult is what oauth.v2.access returns for the authorizing user.
type SlackOAuthResult struct {
	TeamID      SlackTeamID
	UserID      SlackUserID
	AccessToken SlackUserToken `masq:"secret"`
	TokenType   string
	Scopes      []string
}

// SlackIdentity is the team and user a token belongs to, as reported by
// auth.test.
type SlackIdentity struct {
	TeamID SlackTeamID
	UserID SlackUserID
}
