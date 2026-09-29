package model

import (
	"regexp"
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// eventIDPattern also guarantees the ID is usable as a Firestore document ID.
var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// SlackEventClaim records that one Slack event ID has been accepted, so a
// redelivered event is processed only once.
type SlackEventClaim struct {
	EventID   string
	ClaimedAt time.Time
	ExpiresAt time.Time
}

func (c *SlackEventClaim) Validate() error {
	if !eventIDPattern.MatchString(c.EventID) {
		return goerr.New("invalid slack event ID", goerr.V("event_id", c.EventID))
	}
	if c.ClaimedAt.IsZero() {
		return goerr.New("empty slack event claimed_at")
	}
	if !c.ExpiresAt.After(c.ClaimedAt) {
		return goerr.New("slack event expires_at must be after claimed_at")
	}
	return nil
}
