package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/domain/model/auth"
)

func validSession(secret auth.SessionSecret) *auth.Session {
	now := time.Now()
	return &auth.Session{
		ID:         auth.NewSessionID(),
		SecretHash: secret.Hash(),
		TeamID:     "T0123ABCD",
		UserID:     "U0123ABCD",
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
	}
}

func TestSession_Validate(t *testing.T) {
	gt.NoError(t, validSession(auth.NewSessionSecret()).Validate())

	cases := map[string]func(s *auth.Session){
		"empty ID":             func(s *auth.Session) { s.ID = "" },
		"non UUID ID":          func(s *auth.Session) { s.ID = "not-a-uuid" },
		"empty secret hash":    func(s *auth.Session) { s.SecretHash = nil },
		"short secret hash":    func(s *auth.Session) { s.SecretHash = []byte("short") },
		"empty team":           func(s *auth.Session) { s.TeamID = "" },
		"empty user":           func(s *auth.Session) { s.UserID = "" },
		"empty created_at":     func(s *auth.Session) { s.CreatedAt = time.Time{} },
		"expires_at not after": func(s *auth.Session) { s.ExpiresAt = s.CreatedAt },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := validSession(auth.NewSessionSecret())
			mutate(s)
			gt.Error(t, s.Validate())
		})
	}
}

func TestSession_VerifySecret(t *testing.T) {
	secret := auth.NewSessionSecret()
	s := validSession(secret)

	gt.Bool(t, s.VerifySecret(secret)).True()
	gt.Bool(t, s.VerifySecret("")).False()

	altered := []byte(secret)
	if altered[0] == 'A' {
		altered[0] = 'B'
	} else {
		altered[0] = 'A'
	}
	gt.Bool(t, s.VerifySecret(auth.SessionSecret(altered))).False()
}

func TestSession_IsExpired(t *testing.T) {
	s := validSession(auth.NewSessionSecret())
	gt.Bool(t, s.IsExpired(s.ExpiresAt.Add(-time.Second))).False()
	gt.Bool(t, s.IsExpired(s.ExpiresAt)).True()
	gt.Bool(t, s.IsExpired(s.ExpiresAt.Add(time.Second))).True()
}

func TestSession_Key(t *testing.T) {
	s := validSession(auth.NewSessionSecret())
	gt.Value(t, s.Key()).Equal(model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"})
}

func TestNewSessionSecret(t *testing.T) {
	a := auth.NewSessionSecret()
	b := auth.NewSessionSecret()
	gt.Value(t, a).NotEqual(b)
	gt.Number(t, len(a)).Equal(43)
}

func TestNewSessionID(t *testing.T) {
	gt.NoError(t, auth.NewSessionID().Validate())
}

func TestSessionContext(t *testing.T) {
	_, ok := auth.SessionFromContext(context.Background())
	gt.Bool(t, ok).False()

	s := validSession(auth.NewSessionSecret())
	got, ok := auth.SessionFromContext(auth.ContextWithSession(context.Background(), s))
	gt.Bool(t, ok).True()
	gt.Value(t, got).Equal(s)
}
