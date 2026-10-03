package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
	"github.com/m-mizutani/robin/pkg/usecase"
)

// clientToken returns the access token the client from Client calls GitHub
// with.
func (f *githubFixture) clientToken(t *testing.T) model.GitHubAccessToken {
	t.Helper()
	client, err := f.access.Client(context.Background(), testKey)
	gt.NoError(t, err).Required()
	before := len(f.users.tokens)
	_, err = client.GetUser(context.Background())
	gt.NoError(t, err).Required()
	gt.Array(t, f.users.tokens).Length(before + 1).Required()
	return f.users.tokens[before]
}

func TestGitHubUserAccess_ClientWithValidToken(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8*time.Hour - 5*time.Minute - time.Second)

	gt.Value(t, f.clientToken(t)).Equal("ghu_first")
	gt.Array(t, f.oauth.refreshed()).Length(0)
}

func TestGitHubUserAccess_ClientWithTokensThatDoNotExpire(t *testing.T) {
	f := newGitHubFixture()
	f.oauth.token = &model.GitHubToken{AccessToken: "ghu_forever"}
	f.connect(t)
	f.advance(10000 * time.Hour)

	gt.Value(t, f.clientToken(t)).Equal("ghu_forever")
	gt.Array(t, f.oauth.refreshed()).Length(0)
}

func TestGitHubUserAccess_ClientRefreshesTokenAboutToExpire(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8*time.Hour - 5*time.Minute)

	gt.Value(t, f.clientToken(t)).Equal("ghu_refreshed1")
	gt.Value(t, f.oauth.refreshed()).Equal([]model.GitHubRefreshToken{"ghr_first"})

	cred := f.stored(t)
	gt.String(t, cred.ConnectionID).Equal("id-1")
	gt.String(t, cred.RefreshLeaseID).Equal("")
	gt.Bool(t, cred.RefreshLeaseExpiresAt.IsZero()).True()
	gt.Value(t, cred.AccessTokenExpiresAt).Equal(f.now.Add(8 * time.Hour))
	gt.Value(t, cred.UpdatedAt).Equal(f.now)
	refresh, err := f.cipher.Decrypt(context.Background(), cred.RefreshToken, usecase.GitHubRefreshTokenAADForTest(testKey))
	gt.NoError(t, err).Required()
	gt.String(t, string(refresh)).Equal("ghr_refreshed1")

	// The refreshed token is used until it is about to expire again.
	gt.Value(t, f.clientToken(t)).Equal("ghu_refreshed1")
	gt.Array(t, f.oauth.refreshed()).Length(1)
}

// The call to GitHub is bounded well within the refresh lease, so the holder
// never uses the refresh token after another instance could take the lease.
func TestGitHubUserAccess_RefreshCallIsBounded(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)

	before := time.Now()
	gt.Value(t, f.clientToken(t)).Equal("ghu_refreshed1")
	gt.Array(t, f.oauth.refreshDeadlines).Length(1).Required()
	d := f.oauth.refreshDeadlines[0]
	gt.Bool(t, d.ok).True()
	gt.Bool(t, d.deadline.Before(before.Add(31*time.Second))).True()
}

func TestGitHubUserAccess_ClientRefreshTokenRejected(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	f.oauth.refresh = func(model.GitHubRefreshToken) (*model.GitHubToken, error) {
		return nil, goerr.Wrap(interfaces.ErrGitHubTokenInvalid, "bad_refresh_token")
	}

	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGitHubNotConnected)
	f.assertNothingStored(t)
}

func TestGitHubUserAccess_ClientRefreshFails(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	f.oauth.refresh = func(model.GitHubRefreshToken) (*model.GitHubToken, error) {
		return nil, errors.New("github unavailable")
	}

	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err)
	gt.Bool(t, errors.Is(err, usecase.ErrGitHubNotConnected)).False()

	cred := f.stored(t)
	gt.String(t, cred.RefreshLeaseID).Equal("")
	access, err := f.cipher.Decrypt(context.Background(), &cred.AccessToken, usecase.GitHubAccessTokenAADForTest(testKey))
	gt.NoError(t, err).Required()
	gt.String(t, string(access)).Equal("ghu_first")
}

func TestGitHubUserAccess_ClientRefreshReturnsInvalidToken(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	f.oauth.refresh = func(model.GitHubRefreshToken) (*model.GitHubToken, error) {
		return &model.GitHubToken{AccessToken: "ghu_x", AccessTokenExpiresAt: time.Now()}, nil
	}

	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err)
	gt.String(t, f.stored(t).RefreshLeaseID).Equal("")
}

func TestGitHubUserAccess_ClientRefreshTokenExpired(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(4416 * time.Hour)

	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGitHubNotConnected)
	gt.Array(t, f.oauth.refreshed()).Length(0)
	f.assertNothingStored(t)
}

// Another instance holds the lease and saves the refreshed tokens while this
// one waits. This instance uses them instead of refreshing again with the
// refresh token that the other instance already used.
func TestGitHubUserAccess_ClientWaitsForAnotherInstance(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	now := f.clock()
	held, acquired, err := f.repo.GitHubCredential().AcquireRefreshLease(context.Background(), testKey, "other-instance", now, now.Add(time.Minute))
	gt.NoError(t, err).Required()
	gt.Bool(t, acquired).True()

	sleeps := 0
	f.onSleep = func() {
		sleeps++
		if sleeps != 2 {
			return
		}
		access, err := f.cipher.Encrypt(context.Background(), []byte("ghu_other"), usecase.GitHubAccessTokenAADForTest(testKey))
		gt.NoError(t, err).Required()
		refresh, err := f.cipher.Encrypt(context.Background(), []byte("ghr_other"), usecase.GitHubRefreshTokenAADForTest(testKey))
		gt.NoError(t, err).Required()
		next := *held
		next.AccessToken = *access
		next.AccessTokenExpiresAt = f.clock().Add(8 * time.Hour)
		next.RefreshToken = refresh
		next.RefreshLeaseID = ""
		next.RefreshLeaseExpiresAt = time.Time{}
		replaced, err := f.repo.GitHubCredential().ReplaceIfLeaseHeld(context.Background(), testKey, "other-instance", &next)
		gt.NoError(t, err).Required()
		gt.Bool(t, replaced).True()
	}

	gt.Value(t, f.clientToken(t)).Equal("ghu_other")
	gt.Array(t, f.oauth.refreshed()).Length(0)
	gt.Number(t, sleeps).Equal(2)
}

// The instance holding the lease stopped without releasing it. Once the lease
// expires, the waiting instance refreshes itself.
func TestGitHubUserAccess_ClientTakesOverAnAbandonedLease(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	now := f.clock()
	_, _, err := f.repo.GitHubCredential().AcquireRefreshLease(context.Background(), testKey, "stopped-instance", now, now.Add(time.Minute))
	gt.NoError(t, err).Required()

	gt.Value(t, f.clientToken(t)).Equal("ghu_refreshed1")
	gt.Value(t, f.oauth.refreshed()).Equal([]model.GitHubRefreshToken{"ghr_first"})
	gt.Bool(t, f.clock().Sub(now) >= time.Minute).True()
}

func TestGitHubUserAccess_ClientGivesUpWaiting(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	// Another instance keeps renewing its lease.
	f.onSleep = func() {
		now := f.clock()
		gt.NoError(t, f.repo.GitHubCredential().ReleaseRefreshLease(context.Background(), testKey, "busy-instance"))
		_, _, err := f.repo.GitHubCredential().AcquireRefreshLease(context.Background(), testKey, "busy-instance", now, now.Add(time.Minute))
		gt.NoError(t, err).Required()
	}
	now := f.clock()
	_, _, err := f.repo.GitHubCredential().AcquireRefreshLease(context.Background(), testKey, "busy-instance", now, now.Add(time.Minute))
	gt.NoError(t, err).Required()

	_, err = f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGitHubRefreshTimeout)
	gt.Array(t, f.oauth.refreshed()).Length(0)
}

func TestGitHubUserAccess_ClientStopsWhenCancelled(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	now := f.clock()
	_, _, err := f.repo.GitHubCredential().AcquireRefreshLease(context.Background(), testKey, "other-instance", now, now.Add(time.Minute))
	gt.NoError(t, err).Required()
	f.access.SetClockForTest(f.clock, func(context.Context, time.Duration) error { return context.Canceled }, f.newID)

	_, err = f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(context.Canceled)
}

// The user disconnects while this instance refreshes. The refreshed tokens are
// not saved, and the user is not connected.
func TestGitHubUserAccess_ClientDisconnectedDuringRefresh(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	f.advance(8 * time.Hour)
	f.oauth.refresh = func(model.GitHubRefreshToken) (*model.GitHubToken, error) {
		deleted, err := f.repo.GitHubCredential().DeleteIfConnection(context.Background(), testKey, "id-1")
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True()
		return expiringToken(f.clock(), "late"), nil
	}

	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGitHubNotConnected)
	f.assertNothingStored(t)
}

func TestGitHubUserAccess_ClientNotConnected(t *testing.T) {
	f := newGitHubFixture()
	_, err := f.access.Client(context.Background(), testKey)
	gt.Error(t, err).Is(usecase.ErrGitHubNotConnected)
}

// A ciphertext copied into another user's document cannot be decrypted,
// because the additional authenticated data names its owner.
func TestGitHubUserAccess_CiphertextOfAnotherUser(t *testing.T) {
	f := newGitHubFixture()
	f.connect(t)
	cred := f.stored(t)
	cred.TeamID = otherKey.TeamID
	cred.UserID = otherKey.UserID
	cred.GitHubUserID = 1
	gt.NoError(t, f.repo.GitHubCredential().Create(context.Background(), otherKey, cred)).Required()

	_, err := f.access.Client(context.Background(), otherKey)
	gt.Error(t, err)
}
