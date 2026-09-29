package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func validClaim() *model.SlackEventClaim {
	now := time.Now()
	return &model.SlackEventClaim{
		EventID:   "Ev0123ABCD",
		ClaimedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
}

func TestSlackEventClaim_Validate(t *testing.T) {
	gt.NoError(t, validClaim().Validate())

	cases := map[string]func(c *model.SlackEventClaim){
		"empty event ID":              func(c *model.SlackEventClaim) { c.EventID = "" },
		"slash in event ID":           func(c *model.SlackEventClaim) { c.EventID = "Ev01/23" },
		"empty claimed_at":            func(c *model.SlackEventClaim) { c.ClaimedAt = time.Time{} },
		"expires_at equal claimed_at": func(c *model.SlackEventClaim) { c.ExpiresAt = c.ClaimedAt },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validClaim()
			mutate(c)
			gt.Error(t, c.Validate())
		})
	}
}
