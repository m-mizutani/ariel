package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/repository/memory"
	"github.com/m-mizutani/ariel/pkg/usecase"
)

const (
	notionWorkspace   = model.NotionWorkspaceID("0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b")
	notionAlice       = model.NotionUserID("7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e")
	notionBob         = model.NotionUserID("8c4d2f3e-5a6b-4c7d-8e9f-2a3b4c5d6e7f")
	notionPageID      = model.NotionObjectID("0f4a2b1c3d4e4f508a6b7c8d9e0f1a2b")
	notionCallbackURL = "https://ariel.example.com/api/v1/integrations/notion/callback"
)

type notionAuthorizeCall struct {
	State       string
	RedirectURI string
}

// fakeNotionOAuth returns configured results and records every call Ariel
// would make to Notion's OAuth endpoints.
type fakeNotionOAuth struct {
	mu            sync.Mutex
	result        *model.NotionOAuthResult
	exchangeErr   error
	refreshResult *model.NotionOAuthResult
	refreshErr    error
	// onRefresh runs inside RefreshToken, standing for another instance that
	// acts while this refresh is in flight.
	onRefresh func()
	revokeErr error

	authorizes []notionAuthorizeCall
	exchanges  []exchangeCall
	refreshes  []model.NotionRefreshToken
	revokes    []model.NotionAccessToken
}

func newFakeNotionOAuth() *fakeNotionOAuth {
	return &fakeNotionOAuth{
		result: &model.NotionOAuthResult{
			Tokens:        model.NotionTokens{AccessToken: "access-1", RefreshToken: "refresh-1"},
			BotID:         "bot-1",
			WorkspaceID:   notionWorkspace,
			WorkspaceName: "Example",
			OwnerType:     "user",
			OwnerUserID:   notionAlice,
			OwnerName:     "Alice Example",
		},
		refreshResult: &model.NotionOAuthResult{
			Tokens: model.NotionTokens{AccessToken: "access-2", RefreshToken: "refresh-2"},
		},
	}
}

func (f *fakeNotionOAuth) AuthorizeURL(state, redirectURI string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorizes = append(f.authorizes, notionAuthorizeCall{State: state, RedirectURI: redirectURI})
	return "https://api.notion.com/v1/oauth/authorize?state=" + state
}

func (f *fakeNotionOAuth) ExchangeCode(_ context.Context, code, redirectURI string) (*model.NotionOAuthResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exchanges = append(f.exchanges, exchangeCall{Code: code, RedirectURI: redirectURI})
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	res := *f.result
	return &res, nil
}

func (f *fakeNotionOAuth) RefreshToken(_ context.Context, refreshToken model.NotionRefreshToken) (*model.NotionOAuthResult, error) {
	f.mu.Lock()
	f.refreshes = append(f.refreshes, refreshToken)
	hook, res, err := f.onRefresh, f.refreshResult, f.refreshErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return nil, err
	}
	out := *res
	return &out, nil
}

func (f *fakeNotionOAuth) Revoke(_ context.Context, token model.NotionAccessToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokes = append(f.revokes, token)
	return f.revokeErr
}

func (f *fakeNotionOAuth) revoked() []model.NotionAccessToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.NotionAccessToken(nil), f.revokes...)
}

// notionCall is one read the fake client received.
type notionCall struct {
	Token  model.NotionAccessToken
	Method string
	ID     model.NotionObjectID
	Input  any
}

// fakeNotionClients answers every read with fixed results. errs makes the
// client of a token fail every call.
type fakeNotionClients struct {
	mu    sync.Mutex
	errs  map[model.NotionAccessToken]error
	calls []notionCall
}

var (
	notionListResult = &model.NotionList{Results: []json.RawMessage{json.RawMessage(`{"object":"page","id":"p1"}`)}, NextCursor: "c2", HasMore: true}
	notionObject     = json.RawMessage(`{"object":"page","id":"p1"}`)
)

func (f *fakeNotionClients) New(token model.NotionAccessToken) interfaces.NotionClient {
	return &fakeNotionClient{factory: f, token: token}
}

func (f *fakeNotionClients) record(c notionCall) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	return f.errs[c.Token]
}

func (f *fakeNotionClients) recorded() []notionCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]notionCall(nil), f.calls...)
}

func (f *fakeNotionClients) tokens() []model.NotionAccessToken {
	var out []model.NotionAccessToken
	for _, c := range f.recorded() {
		out = append(out, c.Token)
	}
	return out
}

type fakeNotionClient struct {
	factory *fakeNotionClients
	token   model.NotionAccessToken
}

func (c *fakeNotionClient) list(method string, id model.NotionObjectID, input any) (*model.NotionList, error) {
	if err := c.factory.record(notionCall{Token: c.token, Method: method, ID: id, Input: input}); err != nil {
		return nil, err
	}
	return notionListResult, nil
}

func (c *fakeNotionClient) object(method string, id model.NotionObjectID) (json.RawMessage, error) {
	if err := c.factory.record(notionCall{Token: c.token, Method: method, ID: id}); err != nil {
		return nil, err
	}
	return notionObject, nil
}

func (c *fakeNotionClient) Search(_ context.Context, q model.NotionSearchQuery) (*model.NotionList, error) {
	return c.list("Search", "", q)
}

func (c *fakeNotionClient) GetPage(_ context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	return c.object("GetPage", id)
}

func (c *fakeNotionClient) ListBlockChildren(_ context.Context, id model.NotionObjectID, page model.NotionPagination) (*model.NotionList, error) {
	return c.list("ListBlockChildren", id, page)
}

func (c *fakeNotionClient) GetDatabase(_ context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	return c.object("GetDatabase", id)
}

func (c *fakeNotionClient) GetDataSource(_ context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	return c.object("GetDataSource", id)
}

func (c *fakeNotionClient) QueryDataSource(_ context.Context, id model.NotionObjectID, q model.NotionDataSourceQuery) (*model.NotionList, error) {
	return c.list("QueryDataSource", id, q)
}

type notionFixture struct {
	repo    *memory.Memory
	cipher  *fakeCipher
	oauth   *fakeNotionOAuth
	clients *fakeNotionClients
	access  *usecase.NotionAccess
	uc      *usecase.NotionUseCase
	now     time.Time
}

func newNotionFixture() *notionFixture {
	f := &notionFixture{
		repo:    memory.New(),
		cipher:  &fakeCipher{},
		oauth:   newFakeNotionOAuth(),
		clients: &fakeNotionClients{errs: map[model.NotionAccessToken]error{}},
		now:     time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
	}
	f.access = usecase.NewNotionAccess(f.repo, f.cipher, f.oauth, f.clients)
	f.access.SetNowForTest(func() time.Time { return f.now })
	f.uc = usecase.NewNotionUseCase(f.oauth, f.access, usecase.NotionConfig{
		BaseURL:     "https://ariel.example.com",
		WorkspaceID: notionWorkspace,
	})
	return f
}

func aliceGrant(tokens model.NotionTokens) *usecase.NotionGrant {
	return &usecase.NotionGrant{
		Tokens:        tokens,
		BotID:         "bot-1",
		WorkspaceID:   notionWorkspace,
		WorkspaceName: "Example",
		UserID:        notionAlice,
		UserName:      "Alice Example",
	}
}

// storeTokens stores a credential of testKey for Alice's Notion account.
func (f *notionFixture) storeTokens(t *testing.T, tokens model.NotionTokens) {
	t.Helper()
	gt.NoError(t, f.access.Store(context.Background(), testKey, aliceGrant(tokens))).Required()
}

func (f *notionFixture) credential(t *testing.T) *model.NotionCredential {
	t.Helper()
	cred, err := f.repo.NotionCredential().Get(context.Background(), testKey)
	gt.NoError(t, err).Required()
	return cred
}

// storedTokens decrypts the stored credential of testKey.
func (f *notionFixture) storedTokens(t *testing.T) model.NotionTokens {
	t.Helper()
	cred := f.credential(t)
	plaintext, err := f.cipher.Decrypt(context.Background(), &cred.Tokens, usecase.NotionTokenAADForTest(testKey))
	gt.NoError(t, err).Required()
	var tokens model.NotionTokens
	gt.NoError(t, json.Unmarshal(plaintext, &tokens)).Required()
	return tokens
}

// replaceStored stands for another instance that stored tokens for testKey.
func (f *notionFixture) replaceStored(t *testing.T, tokens model.NotionTokens) {
	t.Helper()
	ctx := context.Background()
	current := f.credential(t)
	encrypted, err := f.cipher.Encrypt(ctx, mustJSON(t, tokens), usecase.NotionTokenAADForTest(testKey))
	gt.NoError(t, err).Required()
	next := *current
	next.Tokens = *encrypted
	updated, err := f.repo.NotionCredential().UpdateIfUnchanged(ctx, testKey, current, &next)
	gt.NoError(t, err).Required()
	gt.Bool(t, updated).True().Required()
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	gt.NoError(t, err).Required()
	return raw
}

var firstTokens = model.NotionTokens{AccessToken: "access-1", RefreshToken: "refresh-1"}

func TestNotionAccess_StoreEncryptsTokensForTheOwner(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)

	gt.Array(t, f.cipher.encryptions).Length(1).Required()
	gt.String(t, string(f.cipher.encryptions[0].AAD)).Equal("ariel:notion-token:v1:T0123ABCD:U0123ABCD")
	gt.String(t, string(f.cipher.encryptions[0].Data)).Equal(`{"access_token":"access-1","refresh_token":"refresh-1"}`)

	cred := f.credential(t)
	gt.Value(t, cred.WorkspaceID).Equal(notionWorkspace)
	gt.String(t, cred.WorkspaceName).Equal("Example")
	gt.String(t, cred.BotID).Equal("bot-1")
	gt.Value(t, cred.NotionUserID).Equal(notionAlice)
	gt.String(t, cred.NotionUserName).Equal("Alice Example")
	gt.Bool(t, cred.NeedsReconnect).False()
	gt.Bool(t, cred.CreatedAt.Equal(f.now)).True()
	gt.Bool(t, cred.UpdatedAt.Equal(f.now)).True()
	gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
}

func TestNotionAccess_CiphertextOfAnotherUserCannotBeDecrypted(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)

	copied := *f.credential(t)
	copied.TeamID, copied.UserID = otherKey.TeamID, otherKey.UserID
	copied.NotionUserID = notionBob
	gt.NoError(t, f.repo.NotionCredential().Create(ctx, otherKey, &copied)).Required()

	_, err := f.access.GetPage(ctx, otherKey, notionPageID)
	gt.Value(t, err).NotNil()
	gt.Array(t, f.clients.recorded()).Length(0)
}

func TestNotionAccess_Connection(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()

	status, err := f.access.Connection(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.NotionStatus{})

	f.storeTokens(t, firstTokens)
	status, err = f.access.Connection(ctx, testKey)
	gt.NoError(t, err).Required()
	gt.Value(t, status).Equal(&usecase.NotionStatus{Connected: true, UserName: "Alice Example", WorkspaceName: "Example"})
	gt.Array(t, f.cipher.decryptions).Length(0)
}

func TestNotionAccess_Replace(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces a credential that needs a reconnection", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		created := f.now
		markNeedsReconnect(t, f)

		f.now = f.now.Add(time.Hour)
		grant := aliceGrant(model.NotionTokens{AccessToken: "access-9", RefreshToken: "refresh-9"})
		grant.UserID = notionBob
		grant.UserName = "Bob Example"
		gt.NoError(t, f.access.Replace(ctx, testKey, grant)).Required()

		cred := f.credential(t)
		gt.Bool(t, cred.NeedsReconnect).False()
		gt.Value(t, cred.NotionUserID).Equal(notionBob)
		gt.String(t, cred.NotionUserName).Equal("Bob Example")
		gt.Bool(t, cred.CreatedAt.Equal(created)).True()
		gt.Bool(t, cred.UpdatedAt.Equal(f.now)).True()
		gt.Value(t, f.storedTokens(t)).Equal(grant.Tokens)

		inUse, err := f.access.AccountInUse(ctx, otherKey, notionAlice)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).False()
		inUse, err = f.access.AccountInUse(ctx, otherKey, notionBob)
		gt.NoError(t, err).Required()
		gt.Bool(t, inUse).True()
	})

	t.Run("a working credential is not replaced", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)

		err := f.access.Replace(ctx, testKey, aliceGrant(model.NotionTokens{AccessToken: "a", RefreshToken: "r"}))
		gt.Error(t, err).Is(usecase.ErrNotionAlreadyConnected)
		gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
	})

	t.Run("a missing credential is not created", func(t *testing.T) {
		f := newNotionFixture()
		err := f.access.Replace(ctx, testKey, aliceGrant(firstTokens))
		gt.Error(t, err).Is(usecase.ErrNotionAlreadyConnected)
		_, err = f.repo.NotionCredential().Get(ctx, testKey)
		gt.Error(t, err).Is(interfaces.ErrNotFound)
	})
}

// markNeedsReconnect makes testKey's credential need a reconnection, as a
// rejected refresh token does.
func markNeedsReconnect(t *testing.T, f *notionFixture) {
	t.Helper()
	f.oauth.refreshErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.Error(t, err).Is(usecase.ErrNotionReconnectRequired)
	gt.Bool(t, f.credential(t).NeedsReconnect).True().Required()
	f.oauth.refreshErr = nil
	delete(f.clients.errs, "access-1")
}

func TestNotionAccess_Reads(t *testing.T) {
	ctx := context.Background()
	search := model.NotionSearchQuery{Query: "roadmap", ObjectType: model.NotionObjectTypePage, Page: model.NotionPagination{StartCursor: "c1", PageSize: 10}}
	page := model.NotionPagination{StartCursor: "c1", PageSize: 50}
	query := model.NotionDataSourceQuery{Filter: json.RawMessage(`{"property":"Status","status":{"equals":"Done"}}`)}

	cases := map[string]struct {
		call func(f *notionFixture) (any, error)
		want notionCall
		res  any
	}{
		"search": {
			call: func(f *notionFixture) (any, error) { return f.access.Search(ctx, testKey, search) },
			want: notionCall{Token: "access-1", Method: "Search", Input: search},
			res:  notionListResult,
		},
		"page": {
			call: func(f *notionFixture) (any, error) { return f.access.GetPage(ctx, testKey, notionPageID) },
			want: notionCall{Token: "access-1", Method: "GetPage", ID: notionPageID},
			res:  notionObject,
		},
		"block children": {
			call: func(f *notionFixture) (any, error) {
				return f.access.ListBlockChildren(ctx, testKey, notionPageID, page)
			},
			want: notionCall{Token: "access-1", Method: "ListBlockChildren", ID: notionPageID, Input: page},
			res:  notionListResult,
		},
		"database": {
			call: func(f *notionFixture) (any, error) { return f.access.GetDatabase(ctx, testKey, notionPageID) },
			want: notionCall{Token: "access-1", Method: "GetDatabase", ID: notionPageID},
			res:  notionObject,
		},
		"data source": {
			call: func(f *notionFixture) (any, error) { return f.access.GetDataSource(ctx, testKey, notionPageID) },
			want: notionCall{Token: "access-1", Method: "GetDataSource", ID: notionPageID},
			res:  notionObject,
		},
		"data source query": {
			call: func(f *notionFixture) (any, error) {
				return f.access.QueryDataSource(ctx, testKey, notionPageID, query)
			},
			want: notionCall{Token: "access-1", Method: "QueryDataSource", ID: notionPageID, Input: query},
			res:  notionListResult,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newNotionFixture()
			f.storeTokens(t, firstTokens)

			res, err := tc.call(f)
			gt.NoError(t, err).Required()
			gt.Value(t, res).Equal(tc.res)
			gt.Value(t, f.clients.recorded()).Equal([]notionCall{tc.want})
			gt.Array(t, f.oauth.refreshes).Length(0)
		})
	}
}

func TestNotionAccess_ReadPreconditions(t *testing.T) {
	ctx := context.Background()

	t.Run("not connected", func(t *testing.T) {
		f := newNotionFixture()
		_, err := f.access.GetPage(ctx, testKey, notionPageID)
		gt.Error(t, err).Is(usecase.ErrNotionNotConnected)
		gt.Array(t, f.clients.recorded()).Length(0)
	})

	t.Run("needs reconnect", func(t *testing.T) {
		f := newNotionFixture()
		f.storeTokens(t, firstTokens)
		markNeedsReconnect(t, f)
		calls := len(f.clients.recorded())

		_, err := f.access.Search(ctx, testKey, model.NotionSearchQuery{})
		gt.Error(t, err).Is(usecase.ErrNotionReconnectRequired)
		gt.Array(t, f.clients.recorded()).Length(calls)
	})

	invalid := map[string]func(f *notionFixture) error{
		"malformed page ID": func(f *notionFixture) error {
			_, err := f.access.GetPage(ctx, testKey, "../users")
			return err
		},
		"malformed database ID": func(f *notionFixture) error {
			_, err := f.access.GetDatabase(ctx, testKey, "x")
			return err
		},
		"malformed data source ID": func(f *notionFixture) error {
			_, err := f.access.GetDataSource(ctx, testKey, "x")
			return err
		},
		"malformed block ID": func(f *notionFixture) error {
			_, err := f.access.ListBlockChildren(ctx, testKey, "x", model.NotionPagination{})
			return err
		},
		"page size out of range": func(f *notionFixture) error {
			_, err := f.access.ListBlockChildren(ctx, testKey, notionPageID, model.NotionPagination{PageSize: 101})
			return err
		},
		"unknown object type": func(f *notionFixture) error {
			_, err := f.access.Search(ctx, testKey, model.NotionSearchQuery{ObjectType: "database"})
			return err
		},
		"filter is not JSON": func(f *notionFixture) error {
			_, err := f.access.QueryDataSource(ctx, testKey, notionPageID, model.NotionDataSourceQuery{Filter: json.RawMessage(`{`)})
			return err
		},
		"malformed query ID": func(f *notionFixture) error {
			_, err := f.access.QueryDataSource(ctx, testKey, "x", model.NotionDataSourceQuery{})
			return err
		},
	}
	for name, call := range invalid {
		t.Run(name, func(t *testing.T) {
			f := newNotionFixture()
			f.storeTokens(t, firstTokens)
			gt.Error(t, call(f)).Is(usecase.ErrNotionInvalidRequest)
			gt.Array(t, f.clients.recorded()).Length(0)
		})
	}
}

func TestNotionAccess_RefreshOnInvalidToken(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.now = f.now.Add(time.Hour)

	res, err := f.access.GetPage(ctx, testKey, notionPageID)
	gt.NoError(t, err).Required()
	gt.Value(t, res).Equal(notionObject)

	gt.Value(t, f.oauth.refreshes).Equal([]model.NotionRefreshToken{"refresh-1"})
	gt.Value(t, f.clients.tokens()).Equal([]model.NotionAccessToken{"access-1", "access-2"})
	gt.Value(t, f.storedTokens(t)).Equal(model.NotionTokens{AccessToken: "access-2", RefreshToken: "refresh-2"})
	gt.Bool(t, f.credential(t).UpdatedAt.Equal(f.now)).True()
}

func TestNotionAccess_RefreshWithoutNewRefreshToken(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshResult = &model.NotionOAuthResult{Tokens: model.NotionTokens{AccessToken: "access-2"}}

	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.NoError(t, err).Required()
	gt.Value(t, f.storedTokens(t)).Equal(model.NotionTokens{AccessToken: "access-2", RefreshToken: "refresh-1"})
}

func TestNotionAccess_RefreshWhileAnotherInstanceStored(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	newer := model.NotionTokens{AccessToken: "access-3", RefreshToken: "refresh-3"}
	f.oauth.onRefresh = func() { f.replaceStored(t, newer) }

	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.NoError(t, err).Required()
	// The refreshed token is used for this request, and the newer credential
	// of the other instance is kept.
	gt.Value(t, f.clients.tokens()).Equal([]model.NotionAccessToken{"access-1", "access-2"})
	gt.Value(t, f.storedTokens(t)).Equal(newer)
}

func TestNotionAccess_RejectedRefreshToken(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")

	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.Error(t, err).Is(usecase.ErrNotionReconnectRequired)
	gt.Bool(t, f.credential(t).NeedsReconnect).True()
	gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
	gt.Value(t, f.clients.tokens()).Equal([]model.NotionAccessToken{"access-1"})

	status, err := f.access.Connection(context.Background(), testKey)
	gt.NoError(t, err).Required()
	gt.Bool(t, status.NeedsReconnect).True()
}

func TestNotionAccess_RejectedRefreshTokenAfterAnotherInstanceRefreshed(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")
	newer := model.NotionTokens{AccessToken: "access-3", RefreshToken: "refresh-3"}
	f.oauth.onRefresh = func() { f.replaceStored(t, newer) }

	res, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.NoError(t, err).Required()
	gt.Value(t, res).Equal(notionObject)
	gt.Value(t, f.clients.tokens()).Equal([]model.NotionAccessToken{"access-1", "access-3"})
	gt.Bool(t, f.credential(t).NeedsReconnect).False()
	gt.Value(t, f.storedTokens(t)).Equal(newer)
}

// hookedRepo runs beforeMark right before a credential is marked for
// reconnection, standing for another instance that stores new tokens between
// the read and the mark.
type hookedRepo struct {
	*memory.Memory
	notion *hookedNotionRepo
}

func (r *hookedRepo) NotionCredential() interfaces.NotionCredentialRepository { return r.notion }

type hookedNotionRepo struct {
	interfaces.NotionCredentialRepository
	beforeMark func()
}

func (r *hookedNotionRepo) UpdateIfUnchanged(ctx context.Context, key model.UserKey, expected, next *model.NotionCredential) (bool, error) {
	if next.NeedsReconnect && r.beforeMark != nil {
		hook := r.beforeMark
		r.beforeMark = nil
		hook()
	}
	return r.NotionCredentialRepository.UpdateIfUnchanged(ctx, key, expected, next)
}

func TestNotionAccess_NewTokensStoredBeforeTheMark(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	newer := model.NotionTokens{AccessToken: "access-3", RefreshToken: "refresh-3"}
	repo := &hookedRepo{Memory: f.repo, notion: &hookedNotionRepo{
		NotionCredentialRepository: f.repo.NotionCredential(),
		beforeMark:                 func() { f.replaceStored(t, newer) },
	}}
	access := usecase.NewNotionAccess(repo, f.cipher, f.oauth, f.clients)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")

	res, err := access.GetPage(context.Background(), testKey, notionPageID)
	gt.NoError(t, err).Required()
	gt.Value(t, res).Equal(notionObject)
	gt.Value(t, f.clients.tokens()).Equal([]model.NotionAccessToken{"access-1", "access-3"})
	gt.Bool(t, f.credential(t).NeedsReconnect).False()
	gt.Value(t, f.storedTokens(t)).Equal(newer)
}

func TestNotionAccess_RejectedRefreshTokenAfterDisconnection(t *testing.T) {
	ctx := context.Background()
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshErr = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "invalid_grant")
	f.oauth.onRefresh = func() {
		deleted, err := f.repo.NotionCredential().DeleteIfUnchanged(ctx, testKey, f.credential(t))
		gt.NoError(t, err).Required()
		gt.Bool(t, deleted).True().Required()
	}

	_, err := f.access.GetPage(ctx, testKey, notionPageID)
	gt.Error(t, err).Is(usecase.ErrNotionNotConnected)
}

func TestNotionAccess_RetryIsRejectedToo(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.clients.errs["access-2"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")

	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.Error(t, err).Is(interfaces.ErrNotionTokenInvalid)
	gt.Array(t, f.oauth.refreshes).Length(1)
	gt.Bool(t, f.credential(t).NeedsReconnect).False()
}

func TestNotionAccess_RefreshFailure(t *testing.T) {
	f := newNotionFixture()
	f.storeTokens(t, firstTokens)
	f.clients.errs["access-1"] = goerr.Wrap(interfaces.ErrNotionTokenInvalid, "unauthorized")
	f.oauth.refreshErr = errors.New("notion unavailable")

	_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
	gt.Value(t, err).NotNil()
	gt.Bool(t, errors.Is(err, usecase.ErrNotionReconnectRequired)).False()
	gt.Bool(t, f.credential(t).NeedsReconnect).False()
	gt.Value(t, f.storedTokens(t)).Equal(firstTokens)
}

func TestNotionAccess_OtherErrorsAreNotRefreshed(t *testing.T) {
	for _, sentinel := range []error{interfaces.ErrNotionNotFound, interfaces.ErrNotionForbidden, interfaces.ErrNotionRateLimited} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			f := newNotionFixture()
			f.storeTokens(t, firstTokens)
			f.clients.errs["access-1"] = goerr.Wrap(sentinel, "notion error")

			_, err := f.access.GetPage(context.Background(), testKey, notionPageID)
			gt.Error(t, err).Is(sentinel)
			gt.Array(t, f.oauth.refreshes).Length(0)
		})
	}
}
