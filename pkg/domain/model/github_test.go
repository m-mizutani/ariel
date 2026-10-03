package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

func validGitHubCredential() *model.GitHubCredential {
	now := time.Now()
	return &model.GitHubCredential{
		TeamID:       "T0123ABCD",
		UserID:       "U0123ABCD",
		ConnectionID: "0190c6a4-0000-7000-8000-000000000001",
		GitHubUserID: 583231,
		GitHubLogin:  "octocat",
		AccessToken: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte("access"),
		},
		AccessTokenExpiresAt: now.Add(8 * time.Hour),
		RefreshToken: &model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte("refresh"),
		},
		RefreshTokenExpiresAt: now.Add(4000 * time.Hour),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func TestGitHubCredential_Validate(t *testing.T) {
	gt.NoError(t, validGitHubCredential().Validate())

	t.Run("tokens that do not expire", func(t *testing.T) {
		c := validGitHubCredential()
		c.RefreshToken = nil
		c.AccessTokenExpiresAt = time.Time{}
		c.RefreshTokenExpiresAt = time.Time{}
		gt.NoError(t, c.Validate())
	})

	t.Run("with a lease", func(t *testing.T) {
		c := validGitHubCredential()
		c.RefreshLeaseID = "lease"
		c.RefreshLeaseExpiresAt = time.Now().Add(time.Minute)
		gt.NoError(t, c.Validate())
	})

	cases := map[string]func(c *model.GitHubCredential){
		"invalid key":                    func(c *model.GitHubCredential) { c.UserID = "alice" },
		"empty connection ID":            func(c *model.GitHubCredential) { c.ConnectionID = "" },
		"zero github user ID":            func(c *model.GitHubCredential) { c.GitHubUserID = 0 },
		"negative github user ID":        func(c *model.GitHubCredential) { c.GitHubUserID = -1 },
		"empty login":                    func(c *model.GitHubCredential) { c.GitHubLogin = "" },
		"empty access ciphertext":        func(c *model.GitHubCredential) { c.AccessToken.Ciphertext = nil },
		"empty refresh ciphertext":       func(c *model.GitHubCredential) { c.RefreshToken.Ciphertext = nil },
		"expiring without refresh token": func(c *model.GitHubCredential) { c.RefreshToken = nil },
		"refresh token without expiry":   func(c *model.GitHubCredential) { c.RefreshTokenExpiresAt = time.Time{} },
		"refresh token without access expiry": func(c *model.GitHubCredential) {
			c.AccessTokenExpiresAt = time.Time{}
		},
		"lease ID without expiry": func(c *model.GitHubCredential) { c.RefreshLeaseID = "lease" },
		"lease expiry without ID": func(c *model.GitHubCredential) { c.RefreshLeaseExpiresAt = time.Now() },
		"empty created_at":        func(c *model.GitHubCredential) { c.CreatedAt = time.Time{} },
		"empty updated_at":        func(c *model.GitHubCredential) { c.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validGitHubCredential()
			mutate(c)
			gt.Error(t, c.Validate())
		})
	}
}

func TestGitHubCredential_NeedsRefresh(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	margin := 5 * time.Minute
	c := validGitHubCredential()

	c.AccessTokenExpiresAt = now.Add(margin + time.Nanosecond)
	gt.Bool(t, c.NeedsRefresh(now, margin)).False()

	c.AccessTokenExpiresAt = now.Add(margin)
	gt.Bool(t, c.NeedsRefresh(now, margin)).True()

	c.AccessTokenExpiresAt = now
	gt.Bool(t, c.NeedsRefresh(now, margin)).True()

	c.AccessTokenExpiresAt = time.Time{}
	gt.Bool(t, c.NeedsRefresh(now, margin)).False()
}

func TestGitHubCredential_RefreshExpired(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	c := validGitHubCredential()

	c.RefreshTokenExpiresAt = now.Add(time.Nanosecond)
	gt.Bool(t, c.RefreshExpired(now)).False()

	c.RefreshTokenExpiresAt = now
	gt.Bool(t, c.RefreshExpired(now)).True()

	c.RefreshToken = nil
	c.RefreshTokenExpiresAt = time.Time{}
	gt.Bool(t, c.RefreshExpired(now)).False()
}

func TestGitHubCredential_LeaseHeld(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	c := validGitHubCredential()
	gt.Bool(t, c.LeaseHeld(now)).False()

	c.RefreshLeaseID = "lease"
	c.RefreshLeaseExpiresAt = now.Add(time.Nanosecond)
	gt.Bool(t, c.LeaseHeld(now)).True()

	c.RefreshLeaseExpiresAt = now
	gt.Bool(t, c.LeaseHeld(now)).False()
}

func TestGitHubToken_Validate(t *testing.T) {
	now := time.Now()
	expiring := model.GitHubToken{
		AccessToken:           "ghu_a",
		AccessTokenExpiresAt:  now.Add(8 * time.Hour),
		RefreshToken:          "ghr_r",
		RefreshTokenExpiresAt: now.Add(4000 * time.Hour),
	}
	gt.NoError(t, expiring.Validate())
	gt.NoError(t, (&model.GitHubToken{AccessToken: "ghu_a"}).Validate())

	cases := map[string]func(x *model.GitHubToken){
		"empty access token":      func(x *model.GitHubToken) { x.AccessToken = "" },
		"no refresh token":        func(x *model.GitHubToken) { x.RefreshToken = "" },
		"no access token expiry":  func(x *model.GitHubToken) { x.AccessTokenExpiresAt = time.Time{} },
		"no refresh token expiry": func(x *model.GitHubToken) { x.RefreshTokenExpiresAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			x := expiring
			mutate(&x)
			gt.Error(t, x.Validate())
		})
	}
}

func TestGitHubIdentity_Validate(t *testing.T) {
	gt.NoError(t, (&model.GitHubIdentity{ID: 583231, Login: "octocat"}).Validate())
	gt.Error(t, (&model.GitHubIdentity{ID: 0, Login: "octocat"}).Validate())
	gt.Error(t, (&model.GitHubIdentity{ID: -1, Login: "octocat"}).Validate())
	gt.Error(t, (&model.GitHubIdentity{ID: 583231}).Validate())
}
