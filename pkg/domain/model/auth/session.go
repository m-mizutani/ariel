package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// SessionID identifies one web session. It is a UUIDv7 string.
type SessionID string

func NewSessionID() SessionID {
	return SessionID(uuid.Must(uuid.NewV7()).String())
}

func (x SessionID) Validate() error {
	if x == "" {
		return goerr.New("empty session ID")
	}
	if _, err := uuid.Parse(string(x)); err != nil {
		return goerr.Wrap(err, "invalid session ID format")
	}
	return nil
}

// SessionSecret is the random value stored in the browser cookie. Only its
// SHA-256 hash is persisted.
type SessionSecret string

func NewSessionSecret() SessionSecret {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(goerr.Wrap(err, "failed to generate session secret"))
	}
	return SessionSecret(base64.RawURLEncoding.EncodeToString(b))
}

// Hash returns SHA-256 of the secret. A salt or a slow hash is unnecessary
// because the secret is 32 random bytes.
func (x SessionSecret) Hash() []byte {
	h := sha256.Sum256([]byte(x))
	return h[:]
}

// Session is the sign-in state of one browser.
type Session struct {
	ID         SessionID
	SecretHash []byte
	TeamID     model.SlackTeamID
	UserID     model.SlackUserID
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

func (s *Session) Key() model.UserKey {
	return model.UserKey{TeamID: s.TeamID, UserID: s.UserID}
}

func (s *Session) Validate() error {
	if err := s.ID.Validate(); err != nil {
		return goerr.Wrap(err, "invalid session ID")
	}
	if len(s.SecretHash) != sha256.Size {
		return goerr.New("invalid session secret hash length", goerr.V("length", len(s.SecretHash)))
	}
	if err := s.Key().Validate(); err != nil {
		return goerr.Wrap(err, "invalid session user key")
	}
	if s.CreatedAt.IsZero() {
		return goerr.New("empty session created_at")
	}
	if !s.ExpiresAt.After(s.CreatedAt) {
		return goerr.New("session expires_at must be after created_at")
	}
	return nil
}

func (s *Session) IsExpired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

func (s *Session) VerifySecret(secret SessionSecret) bool {
	return subtle.ConstantTimeCompare(s.SecretHash, secret.Hash()) == 1
}

type ctxSessionKey struct{}

func ContextWithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxSessionKey{}, s)
}

func SessionFromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(ctxSessionKey{}).(*Session)
	return s, ok
}
