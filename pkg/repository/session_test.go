package repository_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
)

func newSession(t *testing.T) (*auth.Session, auth.SessionSecret) {
	t.Helper()
	key := randomUserKey(t)
	secret := auth.NewSessionSecret()
	now := time.Now().UTC()
	return &auth.Session{
		ID:         auth.NewSessionID(),
		SecretHash: secret.Hash(),
		TeamID:     key.TeamID,
		UserID:     key.UserID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(7 * 24 * time.Hour),
	}, secret
}

func TestSessionRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		session, secret := newSession(t)
		gt.NoError(t, repo.Session().Create(ctx, session)).Required()

		got, err := repo.Session().Get(ctx, session.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(session.ID)
		gt.Value(t, got.SecretHash).Equal(session.SecretHash)
		gt.Value(t, got.TeamID).Equal(session.TeamID)
		gt.Value(t, got.UserID).Equal(session.UserID)
		timeEqual(t, got.CreatedAt, session.CreatedAt)
		timeEqual(t, got.ExpiresAt, session.ExpiresAt)
		gt.Bool(t, got.VerifySecret(secret)).True()
	})

	runRepositoryTest(t, "duplicate ID is ErrAlreadyExists", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		session, _ := newSession(t)
		gt.NoError(t, repo.Session().Create(ctx, session)).Required()
		gt.Error(t, repo.Session().Create(ctx, session)).Is(interfaces.ErrAlreadyExists)
	})

	runRepositoryTest(t, "delete", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		session, _ := newSession(t)
		gt.NoError(t, repo.Session().Create(ctx, session)).Required()
		gt.NoError(t, repo.Session().Delete(ctx, session.ID)).Required()

		_, err := repo.Session().Get(ctx, session.ID)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "delete of a missing session succeeds", func(t *testing.T, repo interfaces.Repository) {
		gt.NoError(t, repo.Session().Delete(testContext(t), auth.NewSessionID()))
	})

	runRepositoryTest(t, "missing session is ErrNotFound", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.Session().Get(testContext(t), auth.NewSessionID())
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})
}
