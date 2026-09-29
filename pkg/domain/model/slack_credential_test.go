package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func validCredential() *model.SlackCredential {
	now := time.Now()
	return &model.SlackCredential{
		TeamID: "T0123ABCD",
		UserID: "U0123ABCD",
		AccessToken: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte("ciphertext"),
		},
		Scopes:    []string{"search:read"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSlackCredential_Validate(t *testing.T) {
	gt.NoError(t, validCredential().Validate())

	cases := map[string]func(c *model.SlackCredential){
		"empty team":       func(c *model.SlackCredential) { c.TeamID = "" },
		"empty user":       func(c *model.SlackCredential) { c.UserID = "" },
		"empty key name":   func(c *model.SlackCredential) { c.AccessToken.KeyName = "" },
		"empty ciphertext": func(c *model.SlackCredential) { c.AccessToken.Ciphertext = nil },
		"empty scopes":     func(c *model.SlackCredential) { c.Scopes = nil },
		"empty created_at": func(c *model.SlackCredential) { c.CreatedAt = time.Time{} },
		"empty updated_at": func(c *model.SlackCredential) { c.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validCredential()
			mutate(c)
			gt.Error(t, c.Validate())
		})
	}
}

func TestSlackCredential_Key(t *testing.T) {
	gt.Value(t, validCredential().Key()).Equal(model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"})
}
