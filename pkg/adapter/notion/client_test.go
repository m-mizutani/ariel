package notion_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/adapter/notion"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

const (
	pageID       = model.NotionObjectID("0f4a2b1c3d4e4f508a6b7c8d9e0f1a2b")
	dataSourceID = model.NotionObjectID("1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d")
)

const listBody = `{"object":"list","results":[{"object":"page","id":"p1"},{"object":"data_source","id":"d1"}],"next_cursor":"cursor-2","has_more":true,"type":"page_or_data_source"}`

func assertBearer(t *testing.T, req recordedRequest) {
	t.Helper()
	gt.String(t, req.Header.Get("Authorization")).Equal("Bearer access-1")
	gt.String(t, req.Header.Get("Notion-Version")).Equal("2026-03-11")
}

func wantList() *model.NotionList {
	return &model.NotionList{
		Results:    []json.RawMessage{json.RawMessage(`{"object":"page","id":"p1"}`), json.RawMessage(`{"object":"data_source","id":"d1"}`)},
		NextCursor: "cursor-2",
		HasMore:    true,
	}
}

func TestClient_Search(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/search": {200, listBody}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		got, err := c.Search(context.Background(), model.NotionSearchQuery{
			Query:      "roadmap",
			ObjectType: model.NotionObjectTypeDataSource,
			Page:       model.NotionPagination{StartCursor: "cursor-1", PageSize: 50},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(wantList())

		reqs := fake.recorded()
		gt.Array(t, reqs).Length(1).Required()
		assertBearer(t, reqs[0])
		gt.Value(t, reqs[0].Body).Equal(map[string]any{
			"query":        "roadmap",
			"filter":       map[string]any{"property": "object", "value": "data_source"},
			"start_cursor": "cursor-1",
			"page_size":    float64(50),
		})
	})

	t.Run("empty fields are not sent", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/search": {200, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		got, err := c.Search(context.Background(), model.NotionSearchQuery{})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(&model.NotionList{Results: []json.RawMessage{}})
		gt.Value(t, fake.recorded()[0].Body).Equal(map[string]any{})
	})
}

func TestClient_GetPage(t *testing.T) {
	fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/pages/" + string(pageID): {200, `{"object":"page","id":"p1"}`}})
	c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

	got, err := c.GetPage(context.Background(), pageID)
	gt.NoError(t, err).Required()
	gt.String(t, string(got)).Equal(`{"object":"page","id":"p1"}`)
	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	assertBearer(t, reqs[0])
	gt.String(t, reqs[0].Method).Equal(http.MethodGet)
	gt.Array(t, reqs[0].RawBody).Length(0)
}

func TestClient_ListBlockChildren(t *testing.T) {
	t.Run("with pagination", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/blocks/" + string(pageID) + "/children": {200, listBody}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		got, err := c.ListBlockChildren(context.Background(), pageID, model.NotionPagination{StartCursor: "cursor-1", PageSize: 20})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(wantList())
		reqs := fake.recorded()
		assertBearer(t, reqs[0])
		gt.Value(t, reqs[0].Query).Equal(url.Values{"start_cursor": {"cursor-1"}, "page_size": {"20"}})
	})

	t.Run("without pagination", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/blocks/" + string(pageID) + "/children": {200, listBody}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		_, err := c.ListBlockChildren(context.Background(), pageID, model.NotionPagination{})
		gt.NoError(t, err).Required()
		gt.Value(t, fake.recorded()[0].Query).Equal(url.Values{})
	})
}

func TestClient_GetDatabase(t *testing.T) {
	body := `{"object":"database","id":"db1","data_sources":[{"id":"d1","name":"Tasks"}]}`
	fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/databases/" + string(pageID): {200, body}})
	c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

	got, err := c.GetDatabase(context.Background(), pageID)
	gt.NoError(t, err).Required()
	gt.String(t, string(got)).Equal(body)
	assertBearer(t, fake.recorded()[0])
}

func TestClient_GetDataSource(t *testing.T) {
	body := `{"object":"data_source","id":"d1","properties":{}}`
	fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/data_sources/" + string(dataSourceID): {200, body}})
	c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

	got, err := c.GetDataSource(context.Background(), dataSourceID)
	gt.NoError(t, err).Required()
	gt.String(t, string(got)).Equal(body)
	assertBearer(t, fake.recorded()[0])
}

func TestClient_QueryDataSource(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/data_sources/" + string(dataSourceID) + "/query": {200, listBody}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		got, err := c.QueryDataSource(context.Background(), dataSourceID, model.NotionDataSourceQuery{
			Filter: json.RawMessage(`{"property":"Status","status":{"equals":"Done"}}`),
			Sorts:  json.RawMessage(`[{"timestamp":"last_edited_time","direction":"descending"}]`),
			Page:   model.NotionPagination{StartCursor: "cursor-1", PageSize: 10},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(wantList())
		reqs := fake.recorded()
		assertBearer(t, reqs[0])
		gt.Value(t, reqs[0].Body).Equal(map[string]any{
			"filter":       map[string]any{"property": "Status", "status": map[string]any{"equals": "Done"}},
			"sorts":        []any{map[string]any{"timestamp": "last_edited_time", "direction": "descending"}},
			"start_cursor": "cursor-1",
			"page_size":    float64(10),
		})
	})

	t.Run("empty fields are not sent", func(t *testing.T) {
		fake := newFakeNotion(t, map[string]fakeResponse{"POST /v1/data_sources/" + string(dataSourceID) + "/query": {200, listBody}})
		c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

		_, err := c.QueryDataSource(context.Background(), dataSourceID, model.NotionDataSourceQuery{})
		gt.NoError(t, err).Required()
		gt.Value(t, fake.recorded()[0].Body).Equal(map[string]any{})
	})
}
