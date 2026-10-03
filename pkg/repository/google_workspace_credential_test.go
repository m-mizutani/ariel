package repository_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

// newGoogleCredential returns a credential of key for the Google account
// subject. Tests pass a random subject so parallel runs against one Firestore
// do not share Google accounts.
func newGoogleCredential(key model.UserKey, subject, ciphertext string) *model.GoogleWorkspaceCredential {
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
		Subject:   subject,
		Email:     "alice@example.com",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
}

func randomSubject(t *testing.T) string {
	t.Helper()
	return "sub-" + randomSuffix(t)
}

func TestGoogleWorkspaceCredentialRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		subject := randomSubject(t)
		cred := newGoogleCredential(key, subject, "ciphertext-bytes")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, cred)).Required()

		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.RefreshToken.KeyName).Equal("projects/p/locations/l/keyRings/r/cryptoKeys/k")
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("ciphertext-bytes"))
		gt.Value(t, got.Scopes).Equal([]string{"openid", "https://www.googleapis.com/auth/gmail.readonly"})
		gt.String(t, got.Subject).Equal(subject)
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
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, owner, newGoogleCredential(owner, randomSubject(t), "owner-secret"))).Required()

		_, err := repo.GoogleWorkspaceCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "mismatched key is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		other := randomUserKey(t)
		subject := randomSubject(t)
		err := repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(other, subject, "x"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)

		_, err = repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		_, err = repo.GoogleWorkspaceCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(ctx, randomUserKey(t), subject)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "invalid credential is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGoogleCredential(key, randomSubject(t), "x")
		cred.Email = ""
		gt.Error(t, repo.GoogleWorkspaceCredential().Create(ctx, key, cred))

		_, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "slack and google credentials are separate documents", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "slack-ciphertext"))).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, randomSubject(t), "google-ciphertext"))).Required()

		slack, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, slack.AccessToken.Ciphertext).Equal([]byte("slack-ciphertext"))
		google, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, google.RefreshToken.Ciphertext).Equal([]byte("google-ciphertext"))
	})

	runRepositoryTest(t, "a user with a credential cannot create another", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		first := randomSubject(t)
		second := randomSubject(t)
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, first, "first"))).Required()

		err := repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, second, "second"))
		gt.Error(t, err).Is(interfaces.ErrAlreadyExists)

		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("first"))
		// The rejected account is not connected to anyone.
		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(ctx, randomUserKey(t), second)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "one google account is connected to one user only", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		subject := randomSubject(t)
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, alice, newGoogleCredential(alice, subject, "alice"))).Required()

		err := repo.GoogleWorkspaceCredential().Create(ctx, bob, newGoogleCredential(bob, subject, "bob"))
		gt.Error(t, err).Is(interfaces.ErrGoogleAccountInUse)
		_, err = repo.GoogleWorkspaceCredential().Get(ctx, bob)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(ctx, bob, subject)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).True()
		inUse, err = repo.GoogleWorkspaceCredential().AccountInUse(ctx, alice, subject)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "an unknown google account is not in use", func(t *testing.T, repo interfaces.Repository) {
		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(testContext(t), randomUserKey(t), randomSubject(t))
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "empty subject is rejected", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.GoogleWorkspaceCredential().AccountInUse(testContext(t), randomUserKey(t), "")
		gt.Error(t, err)
	})

	runRepositoryTest(t, "delete when unchanged frees the google account", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		subject := randomSubject(t)
		cred := newGoogleCredential(alice, subject, "x")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, alice, cred)).Required()

		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, alice, cred)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		_, err = repo.GoogleWorkspaceCredential().Get(ctx, alice)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(ctx, bob, subject)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, bob, newGoogleCredential(bob, subject, "bob"))).Required()
	})

	runRepositoryTest(t, "a credential stored after the read is kept", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		old := newGoogleCredential(key, randomSubject(t), "old-ciphertext")
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, old)).Required()
		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, key, old)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		newSubject := randomSubject(t)
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, newSubject, "new-ciphertext"))).Required()

		// A second disconnection that read the old credential arrives late.
		deleted, err = repo.GoogleWorkspaceCredential().DeleteIfUnchanged(ctx, key, old)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
		got, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("new-ciphertext"))
		inUse, err := repo.GoogleWorkspaceCredential().AccountInUse(ctx, randomUserKey(t), newSubject)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).True()
	})

	runRepositoryTest(t, "delete of a missing credential succeeds", func(t *testing.T, repo interfaces.Repository) {
		key := randomUserKey(t)
		deleted, err := repo.GoogleWorkspaceCredential().DeleteIfUnchanged(testContext(t), key, newGoogleCredential(key, randomSubject(t), "x"))
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
	})
}
