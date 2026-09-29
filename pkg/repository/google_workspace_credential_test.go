package repository_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func newGoogleCredential(key model.UserKey, ciphertext string) *model.GoogleWorkspaceCredential {
	now := time.Now().UTC()
	return &model.GoogleWorkspaceCredential{
		TeamID: key.TeamID,
		UserID: key.UserID,
		RefreshToken: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte(ciphertext),
		},
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/gmail.readonly",
		},
		Subject:   "1234567890",
		Email:     "alice@example.com",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
}

func TestGoogleWorkspaceCredentialRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGoogleCredential(key, "ciphertext-bytes")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, cred)).Required()

		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.RefreshToken.KeyName).Equal("projects/p/locations/l/keyRings/r/cryptoKeys/k")
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("ciphertext-bytes"))
		gt.Value(t, got.Scopes).Equal([]string{"openid", "https://www.googleapis.com/auth/gmail.readonly"})
		gt.String(t, got.Subject).Equal("1234567890")
		gt.String(t, got.Email).Equal("alice@example.com")
		timeEqual(t, got.CreatedAt, cred.CreatedAt)
		timeEqual(t, got.UpdatedAt, cred.UpdatedAt)
	})

	runRepositoryTest(t, "missing credential is not found", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.GoogleWorkspaceCredential().Get(testContext(t), randomUserKey(t))
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "another user's key does not read the credential", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		owner := randomUserKey(t)
		other := model.UserKey{TeamID: owner.TeamID, UserID: model.SlackUserID("U" + randomSuffix(t))}
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, owner, newGoogleCredential(owner, "owner-secret"))).Required()

		_, err := repo.GoogleWorkspaceCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "mismatched key is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		other := randomUserKey(t)
		err := repo.GoogleWorkspaceCredential().Put(ctx, key, newGoogleCredential(other, "x"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)

		_, err = repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		_, err = repo.GoogleWorkspaceCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "invalid credential is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGoogleCredential(key, "x")
		cred.Email = ""
		gt.Error(t, repo.GoogleWorkspaceCredential().Put(ctx, key, cred))

		_, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "slack and google credentials are separate documents", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "slack-ciphertext"))).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, newGoogleCredential(key, "google-ciphertext"))).Required()

		slack, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, slack.AccessToken.Ciphertext).Equal([]byte("slack-ciphertext"))
		google, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, google.RefreshToken.Ciphertext).Equal([]byte("google-ciphertext"))
	})

	runRepositoryTest(t, "put overwrites", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, newGoogleCredential(key, "first"))).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, newGoogleCredential(key, "second"))).Required()

		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("second"))
	})

	runRepositoryTest(t, "delete when unchanged", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGoogleCredential(key, "x")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, cred)).Required()

		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, key, cred)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		_, err = repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "a replaced credential is kept", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		old := newGoogleCredential(key, "old-ciphertext")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, old)).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Put(ctx, key, newGoogleCredential(key, "new-ciphertext"))).Required()

		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, key, old)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("new-ciphertext"))
	})

	runRepositoryTest(t, "delete of a missing credential succeeds", func(t *testing.T, repo interfaces.Repository) {
		key := randomUserKey(t)
		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(testContext(t), key, newGoogleCredential(key, "x"))
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
	})
}
