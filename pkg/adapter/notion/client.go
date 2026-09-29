package notion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// ClientFactory builds Notion API clients for one user's access token.
type ClientFactory struct {
	baseURL    string
	httpClient *http.Client
}

var _ interfaces.NotionClientFactory = &ClientFactory{}

// NewClientFactory builds the factory. baseURL is the Notion API origin, such
// as https://api.notion.com.
func NewClientFactory(baseURL string, httpClient *http.Client) *ClientFactory {
	return &ClientFactory{baseURL: strings.TrimSuffix(baseURL, "/"), httpClient: httpClient}
}

func (f *ClientFactory) New(token model.NotionAccessToken) interfaces.NotionClient {
	return &client{baseURL: f.baseURL, token: token, httpClient: f.httpClient}
}

type client struct {
	baseURL    string
	token      model.NotionAccessToken
	httpClient *http.Client
}

func (c *client) call(ctx context.Context, method, path string, query url.Values, body any, out any, msg string) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := newRequest(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+string(c.token))
	return do(ctx, c.httpClient, req, out, msg)
}

type listResponse struct {
	Results    []json.RawMessage `json:"results"`
	NextCursor *string           `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

func (r *listResponse) list() *model.NotionList {
	return &model.NotionList{Results: r.Results, NextCursor: deref(r.NextCursor), HasMore: r.HasMore}
}

// paginationBody adds the pagination fields that are set to a POST body.
func paginationBody(body map[string]any, page model.NotionPagination) {
	if page.StartCursor != "" {
		body["start_cursor"] = page.StartCursor
	}
	if page.PageSize > 0 {
		body["page_size"] = page.PageSize
	}
}

func objectPath(prefix string, id model.NotionObjectID, suffix string) string {
	return prefix + url.PathEscape(string(id)) + suffix
}

func (c *client) Search(ctx context.Context, q model.NotionSearchQuery) (*model.NotionList, error) {
	body := map[string]any{}
	if q.Query != "" {
		body["query"] = q.Query
	}
	if q.ObjectType != model.NotionObjectTypeAny {
		body["filter"] = map[string]string{"property": "object", "value": string(q.ObjectType)}
	}
	paginationBody(body, q.Page)

	var resp listResponse
	if err := c.call(ctx, http.MethodPost, "/v1/search", nil, body, &resp, "failed to search notion"); err != nil {
		return nil, err
	}
	return resp.list(), nil
}

func (c *client) GetPage(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	var resp json.RawMessage
	if err := c.call(ctx, http.MethodGet, objectPath("/v1/pages/", id, ""), nil, nil, &resp, "failed to get notion page"); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) ListBlockChildren(ctx context.Context, id model.NotionObjectID, page model.NotionPagination) (*model.NotionList, error) {
	query := url.Values{}
	if page.StartCursor != "" {
		query.Set("start_cursor", page.StartCursor)
	}
	if page.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(page.PageSize))
	}

	var resp listResponse
	if err := c.call(ctx, http.MethodGet, objectPath("/v1/blocks/", id, "/children"), query, nil, &resp, "failed to list notion block children"); err != nil {
		return nil, err
	}
	return resp.list(), nil
}

func (c *client) GetDatabase(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	var resp json.RawMessage
	if err := c.call(ctx, http.MethodGet, objectPath("/v1/databases/", id, ""), nil, nil, &resp, "failed to get notion database"); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) GetDataSource(ctx context.Context, id model.NotionObjectID) (json.RawMessage, error) {
	var resp json.RawMessage
	if err := c.call(ctx, http.MethodGet, objectPath("/v1/data_sources/", id, ""), nil, nil, &resp, "failed to get notion data source"); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *client) QueryDataSource(ctx context.Context, id model.NotionObjectID, q model.NotionDataSourceQuery) (*model.NotionList, error) {
	body := map[string]any{}
	if len(q.Filter) > 0 {
		body["filter"] = q.Filter
	}
	if len(q.Sorts) > 0 {
		body["sorts"] = q.Sorts
	}
	paginationBody(body, q.Page)

	var resp listResponse
	if err := c.call(ctx, http.MethodPost, objectPath("/v1/data_sources/", id, "/query"), nil, body, &resp, "failed to query notion data source"); err != nil {
		return nil, err
	}
	return resp.list(), nil
}
