package repository_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/repository/firestore"
	"github.com/m-mizutani/ariel/pkg/repository/memory"
)

// runRepositoryTest runs the same test body against the memory and the
// Firestore implementation so both behave identically.
func runRepositoryTest(t *testing.T, name string, fn func(t *testing.T, repo interfaces.Repository)) {
	t.Helper()
	t.Run(name+"/memory", func(t *testing.T) {
		t.Parallel()
		fn(t, memory.New())
	})
	t.Run(name+"/firestore", func(t *testing.T) {
		t.Parallel()
		fn(t, newFirestoreRepository(t))
	})
}

// newFirestoreRepository connects to TEST_FIRESTORE_PROJECT_ID when set, and
// otherwise to a local emulator on 127.0.0.1:28615. It never skips: a missing
// emulator makes the test fail instead of passing silently.
func newFirestoreRepository(t *testing.T) interfaces.Repository {
	t.Helper()

	projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
	databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
	if projectID == "" {
		projectID = "test-project"
		// os.Setenv (not t.Setenv) because these tests run in parallel; the
		// value is identical for every caller.
		if _, ok := os.LookupEnv("FIRESTORE_EMULATOR_HOST"); !ok {
			gt.NoError(t, os.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:28615")).Required()
		}
	}

	repo, err := firestore.New(context.Background(), projectID, databaseID)
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, repo.Close())
	})
	return repo
}

// testContext bounds each repository call. The Firestore client retries an
// unreachable emulator without limit, so without a deadline a missing
// emulator hangs the suite instead of failing it.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	_, err := rand.Read(b)
	gt.NoError(t, err).Required()
	return strings.ToUpper(hex.EncodeToString(b))
}

func randomUserKey(t *testing.T) model.UserKey {
	t.Helper()
	return model.UserKey{
		TeamID: model.SlackTeamID("T" + randomSuffix(t)),
		UserID: model.SlackUserID("U" + randomSuffix(t)),
	}
}

func timeEqual(t *testing.T, got, want time.Time) {
	t.Helper()
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	gt.Bool(t, diff < time.Millisecond).True()
}

func TestUserRepository(t *testing.T) {
	runRepositoryTest(t, "round trip", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		created := time.Now().Add(-time.Hour).UTC()
		updated := time.Now().UTC()
		user := &model.User{
			TeamID:    key.TeamID,
			UserID:    key.UserID,
			Name:      "Alice Example",
			CreatedAt: created,
			UpdatedAt: updated,
		}
		gt.NoError(t, repo.User().Put(ctx, user)).Required()

		got, err := repo.User().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TeamID).Equal(key.TeamID)
		gt.Value(t, got.UserID).Equal(key.UserID)
		gt.String(t, got.Name).Equal("Alice Example")
		timeEqual(t, got.CreatedAt, created)
		timeEqual(t, got.UpdatedAt, updated)
	})

	runRepositoryTest(t, "put overwrites", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		key := randomUserKey(t)
		now := time.Now().UTC()
		gt.NoError(t, repo.User().Put(ctx, &model.User{TeamID: key.TeamID, UserID: key.UserID, Name: "Before", CreatedAt: now, UpdatedAt: now})).Required()
		gt.NoError(t, repo.User().Put(ctx, &model.User{TeamID: key.TeamID, UserID: key.UserID, Name: "After", CreatedAt: now, UpdatedAt: now.Add(time.Minute)})).Required()

		got, err := repo.User().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, got.Name).Equal("After")
		timeEqual(t, got.UpdatedAt, now.Add(time.Minute))
	})

	runRepositoryTest(t, "missing user is ErrNotFound", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.User().Get(testContext(t), randomUserKey(t))
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})

	runRepositoryTest(t, "same user ID in two teams are distinct users", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		userID := model.SlackUserID("U" + randomSuffix(t))
		keyA := model.UserKey{TeamID: model.SlackTeamID("T" + randomSuffix(t)), UserID: userID}
		keyB := model.UserKey{TeamID: model.SlackTeamID("T" + randomSuffix(t)), UserID: userID}
		now := time.Now().UTC()
		gt.NoError(t, repo.User().Put(ctx, &model.User{TeamID: keyA.TeamID, UserID: userID, Name: "In team A", CreatedAt: now, UpdatedAt: now})).Required()
		gt.NoError(t, repo.User().Put(ctx, &model.User{TeamID: keyB.TeamID, UserID: userID, Name: "In team B", CreatedAt: now, UpdatedAt: now})).Required()

		gotA, err := repo.User().Get(ctx, keyA)
		gt.NoError(t, err).Required()
		gt.String(t, gotA.Name).Equal("In team A")
		gotB, err := repo.User().Get(ctx, keyB)
		gt.NoError(t, err).Required()
		gt.String(t, gotB.Name).Equal("In team B")
	})

	runRepositoryTest(t, "invalid user is rejected", func(t *testing.T, repo interfaces.Repository) {
		key := randomUserKey(t)
		err := repo.User().Put(testContext(t), &model.User{TeamID: key.TeamID, UserID: key.UserID})
		gt.Value(t, err).NotNil()
	})
}
