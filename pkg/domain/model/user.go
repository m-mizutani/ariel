package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// User is a Slack user of the configured workspace who signed in on the web
// at least once.
type User struct {
	TeamID    SlackTeamID
	UserID    SlackUserID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u *User) Key() UserKey {
	return UserKey{TeamID: u.TeamID, UserID: u.UserID}
}

func (u *User) Validate() error {
	if err := u.Key().Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if u.Name == "" {
		return goerr.New("empty user name")
	}
	if u.CreatedAt.IsZero() {
		return goerr.New("empty user created_at")
	}
	if u.UpdatedAt.IsZero() {
		return goerr.New("empty user updated_at")
	}
	return nil
}
