package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// newNotionCredential returns a credential of key for the Notion account
// notionUserID. Tests pass a random account so parallel runs against one
// Firestore do not share Notion accounts.
func newNotionCredential(key model.UserKey, notionUserID model.NotionUserID, ciphertext string) *model.NotionCredential {
	now := time.Now().UTC()
	return &model.NotionCredential{
		TeamID: key.TeamID,
		UserID: key.UserID,
		Tokens: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte(ciphertext),
		},
		WorkspaceID:    "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b",
		WorkspaceName:  "Example",
		BotID:          "b1c2d3e4-0000-4000-8000-000000000001",
		NotionUserID:   notionUserID,
		NotionUserName: "Alice Example",
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now,
	}
}

func randomNotionUserID() model.NotionUserID {
	return model.NotionUserID(uuid.NewString())
}

func notionAccountInUse(t *testing.T, repo interfaces.Repository, key model.UserKey, id model.NotionUserID) bool {
	t.Helper()
	inUse, err := repo.NotionCredential().AccountInUse(testContext(t), key, id)
	gt.NoError(t, err).Required()
	return inUse
}

func TestNotionCredentialRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		notionUser := randomNotionUserID()
		cred := newNotionCredential(key, notionUser, "ciphertext-bytes")
		cred.NeedsReconnect = true
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, cred)).Required()

		got, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.Tokens.KeyName).Equal("projects/p/locations/l/keyRings/r/cryptoKeys/k")
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("ciphertext-bytes"))
		gt.Value(t, got.WorkspaceID).Equal(model.NotionWorkspaceID("0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b"))
		gt.String(t, got.WorkspaceName).Equal("Example")
		gt.String(t, got.BotID).Equal("b1c2d3e4-0000-4000-8000-000000000001")
		gt.Value(t, got.NotionUserID).Equal(notionUser)
		gt.String(t, got.NotionUserName).Equal("Alice Example")
		gt.Bool(t, got.NeedsReconnect).True()
		timeEqual(t, got.CreatedAt, cred.CreatedAt)
		timeEqual(t, got.UpdatedAt, cred.UpdatedAt)
	})

	runRepositoryTest(t, "missing credential is not found", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.NotionCredential().Get(testContext(t), randomUserKey(t))
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "another user's key does not read the credential", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		owner := randomUserKey(t)
		other := model.UserKey{TeamID: owner.TeamID, UserID: model.SlackUserID("U" + randomSuffix(t))}
		gt.NoError(t, repo.NotionCredential().Create(ctx, owner, newNotionCredential(owner, randomNotionUserID(), "owner-secret"))).Required()

		_, err := repo.NotionCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "mismatched key is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		other := randomUserKey(t)
		notionUser := randomNotionUserID()
		err := repo.NotionCredential().Create(ctx, key, newNotionCredential(other, notionUser, "x"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)

		_, err = repo.NotionCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		_, err = repo.NotionCredential().Get(ctx, other)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
		gt.Bool(t, notionAccountInUse(t, repo, randomUserKey(t), notionUser)).False()
	})

	runRepositoryTest(t, "invalid credential is rejected and nothing is stored", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newNotionCredential(key, randomNotionUserID(), "x")
		cred.BotID = ""
		gt.Error(t, repo.NotionCredential().Create(ctx, key, cred))

		_, err := repo.NotionCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "notion, slack, and google credentials are separate documents", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		gt.NoError(t, repo.SlackCredential().Put(ctx, key, newCredential(key, "slack-ciphertext"))).Required()
		gt.NoError(t, repo.GoogleWorkspaceCredential().Create(ctx, key, newGoogleCredential(key, randomSubject(t), "google-ciphertext"))).Required()
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, newNotionCredential(key, randomNotionUserID(), "notion-ciphertext"))).Required()

		slack, err := repo.SlackCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, slack.AccessToken.Ciphertext).Equal([]byte("slack-ciphertext"))
		google, err := repo.GoogleWorkspaceCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, google.RefreshToken.Ciphertext).Equal([]byte("google-ciphertext"))
		notion, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, notion.Tokens.Ciphertext).Equal([]byte("notion-ciphertext"))
	})

	runRepositoryTest(t, "a user with a credential cannot create another", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		second := randomNotionUserID()
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, newNotionCredential(key, randomNotionUserID(), "first"))).Required()

		err := repo.NotionCredential().Create(ctx, key, newNotionCredential(key, second, "second"))
		gt.Error(t, err).Is(interfaces.ErrAlreadyExists)

		got, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("first"))
		gt.Bool(t, notionAccountInUse(t, repo, randomUserKey(t), second)).False()
	})

	runRepositoryTest(t, "one notion account is connected to one user only", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		notionUser := randomNotionUserID()
		gt.NoError(t, repo.NotionCredential().Create(ctx, alice, newNotionCredential(alice, notionUser, "alice"))).Required()

		err := repo.NotionCredential().Create(ctx, bob, newNotionCredential(bob, notionUser, "bob"))
		gt.Error(t, err).Is(interfaces.ErrNotionAccountInUse)
		_, err = repo.NotionCredential().Get(ctx, bob)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		gt.Bool(t, notionAccountInUse(t, repo, bob, notionUser)).True()
		gt.Bool(t, notionAccountInUse(t, repo, alice, notionUser)).False()
	})

	runRepositoryTest(t, "an unknown notion account is not in use", func(t *testing.T, repo interfaces.Repository) {
		gt.Bool(t, notionAccountInUse(t, repo, randomUserKey(t), randomNotionUserID())).False()
	})

	runRepositoryTest(t, "invalid notion user ID is rejected", func(t *testing.T, repo interfaces.Repository) {
		for _, id := range []model.NotionUserID{"", "a/b"} {
			_, err := repo.NotionCredential().AccountInUse(testContext(t), randomUserKey(t), id)
			gt.Error(t, err)
		}
	})

	runRepositoryTest(t, "update when unchanged replaces the credential", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		notionUser := randomNotionUserID()
		old := newNotionCredential(key, notionUser, "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, old)).Required()

		next := newNotionCredential(key, notionUser, "new")
		next.NeedsReconnect = true
		updated, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, next)
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).True()

		got, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("new"))
		gt.Bool(t, got.NeedsReconnect).True()
		gt.Bool(t, notionAccountInUse(t, repo, randomUserKey(t), notionUser)).True()
	})

	runRepositoryTest(t, "update of a replaced credential does nothing", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		notionUser := randomNotionUserID()
		old := newNotionCredential(key, notionUser, "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, old)).Required()
		current := newNotionCredential(key, notionUser, "current")
		updated, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, current)
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).True()

		// Another instance that read the old credential arrives late.
		updated, err = repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, newNotionCredential(key, notionUser, "late"))
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).False()
		got, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("current"))
	})

	runRepositoryTest(t, "update of a missing credential does nothing", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		cred := newNotionCredential(key, randomNotionUserID(), "x")
		updated, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, cred, cred)
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).False()
		_, err = repo.NotionCredential().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "update to another notion account moves the ownership", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		first := randomNotionUserID()
		second := randomNotionUserID()
		old := newNotionCredential(key, first, "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, old)).Required()

		updated, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, newNotionCredential(key, second, "new"))
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).True()

		someone := randomUserKey(t)
		gt.Bool(t, notionAccountInUse(t, repo, someone, first)).False()
		gt.Bool(t, notionAccountInUse(t, repo, someone, second)).True()
		gt.NoError(t, repo.NotionCredential().Create(ctx, someone, newNotionCredential(someone, first, "someone"))).Required()
	})

	runRepositoryTest(t, "update to a notion account of another user is rejected", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		aliceAccount := randomNotionUserID()
		bobAccount := randomNotionUserID()
		gt.NoError(t, repo.NotionCredential().Create(ctx, bob, newNotionCredential(bob, bobAccount, "bob"))).Required()
		old := newNotionCredential(alice, aliceAccount, "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, alice, old)).Required()

		_, err := repo.NotionCredential().UpdateIfUnchanged(ctx, alice, old, newNotionCredential(alice, bobAccount, "new"))
		gt.Error(t, err).Is(interfaces.ErrNotionAccountInUse)

		got, err := repo.NotionCredential().Get(ctx, alice)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("old"))
		gt.Bool(t, notionAccountInUse(t, repo, bob, aliceAccount)).True()
		gt.Bool(t, notionAccountInUse(t, repo, alice, bobAccount)).True()
	})

	runRepositoryTest(t, "update with a mismatched key is rejected", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		old := newNotionCredential(key, randomNotionUserID(), "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, old)).Required()

		_, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, newNotionCredential(randomUserKey(t), randomNotionUserID(), "new"))
		gt.Error(t, err).Is(interfaces.ErrKeyMismatch)
	})

	runRepositoryTest(t, "delete when unchanged frees the notion account", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		alice := randomUserKey(t)
		bob := randomUserKey(t)
		notionUser := randomNotionUserID()
		cred := newNotionCredential(alice, notionUser, "x")
		gt.NoError(t, repo.NotionCredential().Create(ctx, alice, cred)).Required()

		deleted, err := repo.NotionCredential().DeleteIfUnchanged(ctx, alice, cred)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		_, err = repo.NotionCredential().Get(ctx, alice)
		gt.Error(t, err).Is(interfaces.ErrNotFound)

		gt.Bool(t, notionAccountInUse(t, repo, bob, notionUser)).False()
		gt.NoError(t, repo.NotionCredential().Create(ctx, bob, newNotionCredential(bob, notionUser, "bob"))).Required()
	})

	runRepositoryTest(t, "a credential stored after the read is kept", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		notionUser := randomNotionUserID()
		old := newNotionCredential(key, notionUser, "old")
		gt.NoError(t, repo.NotionCredential().Create(ctx, key, old)).Required()
		updated, err := repo.NotionCredential().UpdateIfUnchanged(ctx, key, old, newNotionCredential(key, notionUser, "new"))
		gt.NoError(t, err).Required()
		gt.Bool(t, updated).True()

		deleted, err := repo.NotionCredential().DeleteIfUnchanged(ctx, key, old)
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
		got, err := repo.NotionCredential().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Tokens.Ciphertext).Equal([]byte("new"))
		gt.Bool(t, notionAccountInUse(t, repo, randomUserKey(t), notionUser)).True()
	})

	runRepositoryTest(t, "delete of a missing credential succeeds", func(t *testing.T, repo interfaces.Repository) {
		key := randomUserKey(t)
		deleted, err := repo.NotionCredential().DeleteIfUnchanged(testContext(t), key, newNotionCredential(key, randomNotionUserID(), "x"))
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).False()
	})
}
