package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func validGoogleCredential() *model.GoogleWorkspaceCredential {
	now := time.Now()
	return &model.GoogleWorkspaceCredential{
		TeamID: "T0123ABCD",
		UserID: "U0123ABCD",
		RefreshToken: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte("ciphertext"),
		},
		Scopes:    []string{"https://www.googleapis.com/auth/gmail.readonly"},
		Subject:   "1234567890",
		Email:     "alice@example.com",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestGoogleWorkspaceCredential_Validate(t *testing.T) {
	gt.NoError(t, validGoogleCredential().Validate())

	cases := map[string]func(c *model.GoogleWorkspaceCredential){
		"empty team":       func(c *model.GoogleWorkspaceCredential) { c.TeamID = "" },
		"invalid user":     func(c *model.GoogleWorkspaceCredential) { c.UserID = "alice" },
		"empty key name":   func(c *model.GoogleWorkspaceCredential) { c.RefreshToken.KeyName = "" },
		"empty ciphertext": func(c *model.GoogleWorkspaceCredential) { c.RefreshToken.Ciphertext = nil },
		"empty scopes":     func(c *model.GoogleWorkspaceCredential) { c.Scopes = nil },
		"empty subject":    func(c *model.GoogleWorkspaceCredential) { c.Subject = "" },
		"empty email":      func(c *model.GoogleWorkspaceCredential) { c.Email = "" },
		"empty created_at": func(c *model.GoogleWorkspaceCredential) { c.CreatedAt = time.Time{} },
		"empty updated_at": func(c *model.GoogleWorkspaceCredential) { c.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validGoogleCredential()
			mutate(c)
			gt.Error(t, c.Validate())
		})
	}
}

func TestGoogleWorkspaceCredential_Key(t *testing.T) {
	gt.Value(t, validGoogleCredential().Key()).Equal(model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"})
}
