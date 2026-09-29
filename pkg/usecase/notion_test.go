package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/usecase"
)

func (f *notionFixture) assertNothingStored(t *testing.T) {
	t.Helper()
	_, err := f.repo.NotionCredential().Get(context.Background(), testKey)
	gt.Error(t, err).Is(interfaces.ErrNotFound)
}

func TestNotionUseCase_AuthorizeURL(t *testing.T) {
	ctx := context.Background()

	t.Run("not connected", func(t *testing.T) {
		f := newNotionFixture()
		got, err := f.uc.AuthorizeURL(ctx, testKey, "s1")
		gt.NoError(t, err).Required()
		gt.String(t, got).Equal("https://api.notion.com/v1/oauth/authorize?state=s1")
		gt.Value(t, f.oauth.authorizes).Equal([]notionAuthorizeCall{{State: "s1", RedirectURI: notionCallbackURL}})
	})

	t.Run("connected", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		_, err := f.uc.AuthorizeURL(ctx, testKey, "s1")
		gt.Error(t, err).Is(usecase.ErrNotionAlreadyConnected)
		gt.Array(t, f.oauth.authorizes).Length(0)
	})

	t.Run("needs reconnect", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		markNeedsReconnect(t, f)
		_, err := f.uc.AuthorizeURL(ctx, testKey, "s1")
		gt.NoError(t, err)
		gt.Array(t, f.oauth.authorizes).Length(1)
	})
}

func TestNotionUseCase_HandleCallback(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()

	gt.NoError(t, f.uc.HandleCallback(ctx, testKey, "code-1")).Required()

	gt.Value(t, f.oauth.exchanges).Equal([]exchangeCall{{Code: "code-1", RedirectURI: notionCallbackURL}})
	gt.Array(t, f.oauth.revoked()).Length(0)
	cred := f.credential(t)
	gt.Value(t, cred.Key()).Equal(testKey)
	gt.Value(t, cred.WorkspaceID).Equal(notionWorkspace)
	gt.String(t, cred.WorkspaceName).Equal("Example")
	gt.String(t, cred.BotID).Equal("bot-1")
	gt.Value(t, cred.NotionUserID).Equal(notionAlice)
	gt.String(t, cred.NotionUserName).Equal("Alice Example")
	gt.Bool(t, cred.CreatedAt.Equal(f.now)).True()
	gt.Bool(t, cred.UpdatedAt.Equal(f.now)).True()
	gt.Value(t, f.storedTokens(t)).Equal(firstTokens)

	inUse, err := f.access.AccountInUse(ctx, otherKey, notionAlice)
	gt.NoError(t, err).Required()
	gt.Bool(t, inUse).True()
}

func TestNotionUseCase_HandleCallbackWhenConnected(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)

	err := f.uc.HandleCallback(context.Background(), testKey, "code-1")
	gt.Error(t, err).Is(usecase.ErrNotionAlreadyConnected)
	gt.Array(t, f.oauth.exchanges).Length(0)
	gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
}

func TestNotionUseCase_Reconnect(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	created := f.now
	markNeedsReconnect(t, f)

	f.now = f.now.Add(time.Hour)
	f.oauth.result.Tokens = model.NotionTokens{AccessToken: "access-9", RefreshToken: "refresh-9"}
	f.oauth.result.OwnerUserID = notionBob
	f.oauth.result.OwnerName = "Bob Example"
	gt.NoError(t, f.uc.HandleCallback(ctx, testKey, "code-2")).Required()

	cred := f.credential(t)
	gt.Bool(t, cred.NeedsReconnect).False()
	gt.Bool(t, cred.CreatedAt.Equal(created)).True()
	gt.Bool(t, cred.UpdatedAt.Equal(f.now)).True()
	gt.Value(t, cred.NotionUserID).Equal(notionBob)
	gt.Value(t, f.storedTokens(t)).Equal(model.NotionTokens{AccessToken: "access-9", RefreshToken: "refresh-9"})

	inUse, err := f.access.AccountInUse(ctx, otherKey, notionAlice)
	gt.NoError(t, err).Required()
	gt.Bool(t, inUse).False()
	gt.Array(t, f.oauth.revoked()).Length(0)
}

func TestNotionUseCase_HandleCallbackRejections(t *testing.T) {
	cases := map[string]struct {
		mutate     func(res *model.NotionOAuthResult)
		want       error
		wantRevoke []model.NotionAccessToken
	}{
		"workspace authorization": {
			mutate: func(res *model.NotionOAuthResult) { res.OwnerType = "workspace"; res.OwnerUserID = "" },
			want:   usecase.ErrNotionConnectRejected,
		},
		"no owner": {
			mutate: func(res *model.NotionOAuthResult) { res.OwnerUserID = "" },
			want:   usecase.ErrNotionConnectRejected,
		},
		"another workspace": {
			mutate:     func(res *model.NotionOAuthResult) { res.WorkspaceID = "11111111-2222-4333-8444-555555555555" },
			want:       usecase.ErrNotionWrongWorkspace,
			wantRevoke: []model.NotionAccessToken{"access-1"},
		},
		"no refresh token": {
			mutate:     func(res *model.NotionOAuthResult) { res.Tokens.RefreshToken = "" },
			want:       usecase.ErrNotionConnectRejected,
			wantRevoke: []model.NotionAccessToken{"access-1"},
		},
		"no bot": {
			mutate:     func(res *model.NotionOAuthResult) { res.BotID = "" },
			want:       usecase.ErrNotionConnectRejected,
			wantRevoke: []model.NotionAccessToken{"access-1"},
		},
		"no access token": {
			mutate: func(res *model.NotionOAuthResult) { res.Tokens.AccessToken = "" },
			want:   usecase.ErrNotionConnectRejected,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newNotionFixture()
			tc.mutate(f.oauth.result)

			err := f.uc.HandleCallback(context.Background(), testKey, "code-1")
			gt.Error(t, err).Is(tc.want)
			gt.Value(t, f.oauth.revoked()).Equal(tc.wantRevoke)
			gt.Array(t, f.cipher.encryptions).Length(0)
			f.assertNothingStored(t)
		})
	}
}

func TestNotionUseCase_AccountOfAnotherUser(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	gt.NoError(t, f.access.Store(ctx, otherKey, aliceGrant(model.NotionTokens{AccessToken: "bob-access", RefreshToken: "bob-refresh"}))).Required()

	err := f.uc.HandleCallback(ctx, testKey, "code-1")
	gt.Error(t, err).Is(usecase.ErrNotionAccountInUse)
	gt.Array(t, f.oauth.revoked()).Length(0)
	f.assertNothingStored(t)
}

// Two callbacks of one user that run at once: the second finds the credential
// the first stored and must not revoke its own authorization, which may be the
// same connection.
func TestNotionUseCase_ConcurrentCallback(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	// The first callback stores its credential while this one is past its
	// first check.
	wrapped := &storingOAuth{fakeNotionOAuth: f.oauth, beforeExchange: func() {
		f.storeTokens(t, model.NotionTokens{AccessToken: "other-access", RefreshToken: "other-refresh"})
	}}
	uc := usecase.NewNotionUseCase(wrapped, f.access, usecase.NotionConfig{BaseURL: "https://ariel.example.com", WorkspaceID: notionWorkspace})

	err := uc.HandleCallback(ctx, testKey, "code-1")
	gt.Error(t, err).Is(usecase.ErrNotionAlreadyConnected)
	gt.Array(t, f.oauth.revoked()).Length(0)
	gt.Value(t, f.storedTokens(t)).Equal(model.NotionTokens{AccessToken: "other-access", RefreshToken: "other-refresh"})
}

// storingOAuth runs beforeExchange before the code exchange.
type storingOAuth struct {
	*fakeNotionOAuth
	beforeExchange func()
}

func (s *storingOAuth) ExchangeCode(ctx context.Context, code, redirectURI string) (*model.NotionOAuthResult, error) {
	s.beforeExchange()
	return s.fakeNotionOAuth.ExchangeCode(ctx, code, redirectURI)
}

func TestNotionUseCase_StoreFailure(t *testing.T) {
	f := newNotionFixture()
	f.cipher.encryptErr = errors.New("kms unavailable")

	err := f.uc.HandleCallback(context.Background(), testKey, "code-1")
	gt.Value(t, err).NotNil()
	gt.Value(t, f.oauth.revoked()).Equal([]model.NotionAccessToken{"access-1"})
	f.assertNothingStored(t)
}

func TestNotionUseCase_RevokeFailureKeepsTheCause(t *testing.T) {
	f := newNotionFixture()
	f.oauth.result.WorkspaceID = "11111111-2222-4333-8444-555555555555"
	f.oauth.revokeErr = errors.New("notion unavailable")

	err := f.uc.HandleCallback(context.Background(), testKey, "code-1")
	gt.Error(t, err).Is(usecase.ErrNotionWrongWorkspace)
}

func TestNotionUseCase_ExchangeFailure(t *testing.T) {
	f := newNotionFixture()
	f.oauth.exchangeErr = errors.New("invalid code")

	gt.Value(t, f.uc.HandleCallback(context.Background(), testKey, "code-1")).NotNil()
	gt.Array(t, f.oauth.revoked()).Length(0)
	f.assertNothingStored(t)
}

func TestNotionUseCase_Status(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()

	status, err := f.uc.Status(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.NotionStatus{})

	f.storeTokens(t, firstTokens)
	status, err = f.uc.Status(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.NotionStatus{Connected: true, UserName: "Alice Example", WorkspaceName: "Example"})

	markNeedsReconnect(t, f)
	calls := len(f.clients.recorded())
	status, err = f.uc.Status(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.NotionStatus{Connected: true, NeedsReconnect: true, UserName: "Alice Example", WorkspaceName: "Example"})
	gt.Array(t, f.clients.recorded()).Length(calls)
}

func TestNotionUseCase_Disconnect(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)

		gt.NoError(t, f.uc.Disconnect(ctx, testKey)).Required()
		gt.Value(t, f.oauth.revoked()).Equal([]model.NotionAccessToken{"access-1"})
		f.assertNothingStored(t)
		inUse, err := f.access.AccountInUse(ctx, otherKey, notionAlice)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
	})

	t.Run("token already invalid", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		f.oauth.revokeErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")

		gt.NoError(t, f.uc.Disconnect(ctx, testKey)).Required()
		f.assertNothingStored(t)
	})

	t.Run("revocation failure keeps the credential", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		f.oauth.revokeErr = errors.New("notion unavailable")

		gt.Value(t, f.uc.Disconnect(ctx, testKey)).NotNil()
		gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
	})

	t.Run("not connected", func(t *testing.T) {
		f := newNotionFixture()
		gt.NoError(t, f.uc.Disconnect(ctx, testKey))
		gt.Array(t, f.oauth.revoked()).Length(0)
	})

	t.Run("decryption failure", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		f.cipher.decryptErr = errors.New("kms key disabled")

		gt.Value(t, f.uc.Disconnect(ctx, testKey)).NotNil()
		gt.Array(t, f.oauth.revoked()).Length(0)
		f.cipher.decryptErr = nil
		gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
	})
}
