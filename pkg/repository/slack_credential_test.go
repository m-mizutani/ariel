package repository_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

func newCredential(key model.UserKey, ciphertext string) *model.SlackCredential {
	now := time.Now().UTC()
	return &model.SlackCredential{
		TeamID: key.TeamID,
		UserID: key.UserID,
		AccessToken: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte(ciphertext),
		},
		Scopes:    []string{"search:read", "users:read"},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
}

func TestSlackCredentialRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newCredential(key, "ciphertext-bytes")
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, cred)).Required()

		got, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.AccessToken.KeyName).Equal("projects/p/locations/l/keyRings/r/cryptoKeys/k")
		gt.Value(t, got.AccessToken.Ciphertext).Equal([]byte("ciphertext-bytes"))
		gt.Value(t, got.Scopes).Equal([]string{"search:read", "users:read"})
		timeEqual(t, got.CreatedAt, cred.CreatedAt)
		timeEqual(t, got.UpdatedAt, cred.UpdatedAt)
	})

	runRepositoryTest(t, "another user's key does not read the credential", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		owner := randomUserKey(t)
		other := model.UserKey{TeamID: owner.TeamID, UserID: model.SlackUserID("U" + randomSuffix(t))}
		gt.NoError(t, repo.SlackCredential().Put(ctx, owner, newCredential(owner, "owner-secret"))).Required()

		_, err := repo.SlackCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "mismatched key is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		other := randomUserKey(t)
		err := repo.SlackCredential().Put(ctx, key, newCredential(other, "x"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)

		_, err = repo.SlackCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		_, err = repo.SlackCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "put overwrites", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "first"))).Required()
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "second"))).Required()

		got, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.AccessToken.Ciphertext).Equal([]byte("second"))
	})

	runRepositoryTest(t, "delete when unchanged", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newCredential(key, "x")
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, cred)).Required()

		deleted, err := repo.SlackCredential().DeleteIfUnchanged(ctx, key, cred)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		_, err = repo.SlackCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "a replaced credential is kept", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		old := newCredential(key, "old-ciphertext")
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, old)).Required()
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "new-ciphertext"))).Required()

		deleted, err := repo.SlackCredential().DeleteIfUnchanged(ctx, key, old)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
		got, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.AccessToken.Ciphertext).Equal([]byte("new-ciphertext"))
	})

	runRepositoryTest(t, "delete of a missing credential succeeds", func(t *testing.T, repo interfaces.Repository) {
		key := randomUserKey(t)
		deleted, err := repo.SlackCredential().DeleteIfUnchanged(testContext(t), key, newCredential(key, "x"))
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
	})
}
