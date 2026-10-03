package repository_test

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

const testKeyName = "projects/p/locations/l/keyRings/r/cryptoKeys/k"

// randomGitHubUserID keeps parallel runs against one Firestore from sharing
// GitHub accounts.
func randomGitHubUserID() model.GitHubUserID {
	return model.GitHubUserID(rand.Int64N(1<<50) + 1)
}

func newGitHubCredential(key model.UserKey, id model.GitHubUserID, connectionID, access string) *model.GitHubCredential {
	now := time.Now().UTC()
	return &model.GitHubCredential{
		TeamID:       key.TeamID,
		UserID:       key.UserID,
		ConnectionID: connectionID,
		GitHubUserID: id,
		GitHubLogin:  "octocat",
		AccessToken: model.EncryptedData{
			KeyName:    testKeyName,
			Ciphertext: []byte(access),
		},
		AccessTokenExpiresAt: now.Add(8 * time.Hour),
		RefreshToken: &model.EncryptedData{
			KeyName:    testKeyName,
			Ciphertext: []byte("refresh-" + access),
		},
		RefreshTokenExpiresAt: now.Add(4000 * time.Hour),
		CreatedAt:             now.Add(-time.Hour),
		UpdatedAt:             now,
	}
}

func TestGitHubCredentialRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		id := randomGitHubUserID()
		cred := newGitHubCredential(key, id, "conn-1", "access")
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, cred)).Required()

		got, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.ConnectionID).Equal("conn-1")
		gt.Value(t, got.GitHubUserID).Equal(id)
		gt.String(t, got.GitHubLogin).Equal("octocat")
		gt.String(t, got.AccessToken.KeyName).Equal(testKeyName)
		gt.Value(t, got.AccessToken.Ciphertext).Equal([]byte("access"))
		gt.Value(t, got.RefreshToken).NotNil().Required()
		gt.String(t, got.RefreshToken.KeyName).Equal(testKeyName)
		gt.Value(t, got.RefreshToken.Ciphertext).Equal([]byte("refresh-access"))
		timeEqual(t, got.AccessTokenExpiresAt, cred.AccessTokenExpiresAt)
		timeEqual(t, got.RefreshTokenExpiresAt, cred.RefreshTokenExpiresAt)
		gt.String(t, got.RefreshLeaseID).Equal("")
		gt.Bool(t, got.RefreshLeaseExpiresAt.IsZero()).True()
		timeEqual(t, got.CreatedAt, cred.CreatedAt)
		timeEqual(t, got.UpdatedAt, cred.UpdatedAt)
	})

	runRepositoryTest(t, "tokens that do not expire", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGitHubCredential(key, randomGitHubUserID(), "conn-1", "access")
		cred.RefreshToken = nil
		cred.AccessTokenExpiresAt = time.Time{}
		cred.RefreshTokenExpiresAt = time.Time{}
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, cred)).Required()

		got, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.RefreshToken).Nil()
		gt.Bool(t, got.AccessTokenExpiresAt.IsZero()).True()
		gt.Bool(t, got.RefreshTokenExpiresAt.IsZero()).True()
	})

	runRepositoryTest(t, "missing credential is not found", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.GitHubCredential().Get(testContext(t), randomUserKey(t))
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "another user's key does not read the credential", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		owner := randomUserKey(t)
		other := model.UserKey{TeamID: owner.TeamID, UserID: model.SlackUserID("U" + randomSuffix(t))}
		gt.NoError(t, repo.GitHubCredential().Create(ctx, owner, newGitHubCredential(owner, randomGitHubUserID(), "c", "x"))).Required()

		_, err := repo.GitHubCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "mismatched key is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		other := randomUserKey(t)
		id := randomGitHubUserID()
		err := repo.GitHubCredential().Create(ctx, key, newGitHubCredential(other, id, "c", "x"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)

		_, err = repo.GitHubCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		inUse, err := repo.GitHubCredential().AccountInUse(ctx, randomUserKey(t), id)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "invalid credential is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newGitHubCredential(key, randomGitHubUserID(), "c", "x")
		cred.GitHubLogin = ""
		gt.Error(t, repo.GitHubCredential().Create(ctx, key, cred))

		_, err := repo.GitHubCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "a user with a connection cannot create another", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		second := randomGitHubUserID()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, randomGitHubUserID(), "first", "first"))).Required()

		err := repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, second, "second", "second"))
		gt.Error(t, err).Is(interfaces.ErrAlreadyExists)

		got, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, got.ConnectionID).Equal("first")
		inUse, err := repo.GitHubCredential().AccountInUse(ctx, randomUserKey(t), second)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "one github account is connected to one user only", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		id := randomGitHubUserID()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, alice, newGitHubCredential(alice, id, "a", "alice"))).Required()

		err := repo.GitHubCredential().Create(ctx, bob, newGitHubCredential(bob, id, "b", "bob"))
		gt.Error(t, err).Is(interfaces.ErrGitHubAccountInUse)
		_, err = repo.GitHubCredential().Get(ctx, bob)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		inUse, err := repo.GitHubCredential().AccountInUse(ctx, bob, id)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).True()
		inUse, err = repo.GitHubCredential().AccountInUse(ctx, alice, id)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	runRepositoryTest(t, "only one of two users connecting one github account at the same time succeeds", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		id := randomGitHubUserID()
		keys := []model.UserKey{randomUserKey(t), randomUserKey(t)}
		errs := make([]error, len(keys))
		var wg sync.WaitGroup
		for i, key := range keys {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, id, "c", "x"))
			}()
		}
		wg.Wait()

		succeeded := 0
		for _, err := range errs {
			if err == nil {
				succeeded++
			} else {
				gt.Error(t, err).Is(interfaces.ErrGitHubAccountInUse)
			}
		}
		gt.Number(t, succeeded).Equal(1)
	})

	runRepositoryTest(t, "invalid github user ID is rejected", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.GitHubCredential().AccountInUse(testContext(t), randomUserKey(t), 0)
		gt.Error(t, err)
	})

	runRepositoryTest(t, "refresh lease", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		now := time.Now().UTC()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, randomGitHubUserID(), "c", "x"))).Required()

		got, acquired, err := repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-a", now, now.Add(time.Minute))
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()
		gt.String(t, got.RefreshLeaseID).Equal("lease-a")

		stored, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, stored.RefreshLeaseID).Equal("lease-a")
		timeEqual(t, stored.RefreshLeaseExpiresAt, now.Add(time.Minute))

		// Another instance cannot take a held lease and sees the holder.
		got, acquired, err = repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-b", now.Add(30*time.Second), now.Add(90*time.Second))
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).False()
		gt.String(t, got.RefreshLeaseID).Equal("lease-a")

		// After the lease expires, another instance takes it.
		_, acquired, err = repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-b", now.Add(time.Minute), now.Add(2*time.Minute))
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()
	})

	runRepositoryTest(t, "refresh lease of a missing connection is not found", func(t *testing.T, repo interfaces.Repository) {
		now := time.Now()
		_, _, err := repo.GitHubCredential().AcquireRefreshLease(testContext(t), randomUserKey(t), "lease", now, now.Add(time.Minute))
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "only one of many instances acquires the refresh lease", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		now := time.Now().UTC()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, randomGitHubUserID(), "c", "x"))).Required()

		const n = 20
		acquired := make([]bool, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, acquired[i], errs[i] = repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-"+randomSuffix(t), now, now.Add(time.Minute))
			}()
		}
		wg.Wait()

		count := 0
		for i := range n {
			gt.NoError(t, errs[i])
			if acquired[i] {
				count++
			}
		}
		gt.Number(t, count).Equal(1)
	})

	runRepositoryTest(t, "replace while holding the lease", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		id := randomGitHubUserID()
		now := time.Now().UTC()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, id, "c", "old"))).Required()
		_, acquired, err := repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-a", now, now.Add(time.Minute))
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		next := newGitHubCredential(key, id, "c", "new")
		replaced, err := repo.GitHubCredential().ReplaceIfLeaseHeld(ctx, key, "lease-b", next)
		gt.NoError(t, err).Required()
		gt.Bool(t, replaced).False()

		replaced, err = repo.GitHubCredential().ReplaceIfLeaseHeld(ctx, key, "lease-a", next)
		gt.NoError(t, err).Required()
		gt.Bool(t, replaced).True()
		got, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.AccessToken.Ciphertext).Equal([]byte("new"))
		gt.String(t, got.RefreshLeaseID).Equal("")
	})

	runRepositoryTest(t, "replace is refused for another connection or a deleted one", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		id := randomGitHubUserID()
		now := time.Now().UTC()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, id, "c1", "old"))).Required()
		_, _, err := repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-a", now, now.Add(time.Minute))
		gt.NoError(t, err).Required()

		replaced, err := repo.GitHubCredential().ReplaceIfLeaseHeld(ctx, key, "lease-a", newGitHubCredential(key, id, "c2", "new"))
		gt.NoError(t, err).Required()
		gt.Bool(t, replaced).False()

		deleted, err := repo.GitHubCredential().DeleteIfConnection(ctx, key, "c1")
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		replaced, err = repo.GitHubCredential().ReplaceIfLeaseHeld(ctx, key, "lease-a", newGitHubCredential(key, id, "c1", "new"))
		gt.NoError(t, err).Required()
		gt.Bool(t, replaced).False()
		_, err = repo.GitHubCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "release refresh lease", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		now := time.Now().UTC()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, randomGitHubUserID(), "c", "x"))).Required()
		_, _, err := repo.GitHubCredential().AcquireRefreshLease(ctx, key, "lease-a", now, now.Add(time.Minute))
		gt.NoError(t, err).Required()

		gt.NoError(t, repo.GitHubCredential().ReleaseRefreshLease(ctx, key, "lease-b"))
		got, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, got.RefreshLeaseID).Equal("lease-a")

		gt.NoError(t, repo.GitHubCredential().ReleaseRefreshLease(ctx, key, "lease-a"))
		got, err = repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, got.RefreshLeaseID).Equal("")
		gt.Bool(t, got.RefreshLeaseExpiresAt.IsZero()).True()

		gt.NoError(t, repo.GitHubCredential().ReleaseRefreshLease(ctx, randomUserKey(t), "lease-a"))
	})

	runRepositoryTest(t, "delete by connection frees the github account", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		id := randomGitHubUserID()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, alice, newGitHubCredential(alice, id, "c1", "x"))).Required()

		deleted, err := repo.GitHubCredential().DeleteIfConnection(ctx, alice, "other")
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
		_, err = repo.GitHubCredential().Get(ctx, alice)
		gt.NoError(t, err).Required()

		deleted, err = repo.GitHubCredential().DeleteIfConnection(ctx, alice, "c1")
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		_, err = repo.GitHubCredential().Get(ctx, alice)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		inUse, err := repo.GitHubCredential().AccountInUse(ctx, bob, id)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, bob, newGitHubCredential(bob, id, "c2", "bob"))).Required()
	})

	runRepositoryTest(t, "delete of a missing connection succeeds", func(t *testing.T, repo interfaces.Repository) {
		deleted, err := repo.GitHubCredential().DeleteIfConnection(testContext(t), randomUserKey(t), "c")
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
	})

	runRepositoryTest(t, "slack, google, and github credentials are separate documents", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "slack-ciphertext"))).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, randomSubject(t), "google-ciphertext"))).Required()
		gt.NoError(t, repo.GitHubCredential().Create(ctx, key, newGitHubCredential(key, randomGitHubUserID(), "c", "github-ciphertext"))).Required()

		slack, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, slack.AccessToken.Ciphertext).Equal([]byte("slack-ciphertext"))
		google, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, google.RefreshToken.Ciphertext).Equal([]byte("google-ciphertext"))
		github, err := repo.GitHubCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, github.AccessToken.Ciphertext).Equal([]byte("github-ciphertext"))
	})
}
