// Package notion wraps the OAuth endpoints and the read-only API of Notion.
package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/utils/safe"
)

// notionVersion is the Notion API version this package is written against.
// Notion requires it on every request.
const notionVersion = "2026-03-11"

// maxResponseBytes bounds a response body read into memory.
const maxResponseBytes = 10 << 20

// newRequest builds a request with the Notion-Version header, and with a JSON
// body when body is not nil.
func newRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to encode notion request")
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to build notion request")
	}
	req.Header.Set("Notion-Version", notionVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do sends req and decodes a successful JSON response into out (skipped when
// out is nil). msg names the operation in the returned error.
func do(ctx context.Context, httpClient *http.Client, req *http.Request, out any, msg string) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return goerr.Wrap(err, msg)
	}
	defer safe.Close(ctx, resp.Body)

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return goerr.Wrap(err, msg, goerr.V("notion_status", resp.StatusCode))
	}
	if len(body) > maxResponseBytes {
		return goerr.New("notion response is too large", goerr.V("notion_status", resp.StatusCode))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return wrapError(resp.StatusCode, body, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return goerr.Wrap(err, "failed to decode notion response", goerr.V("notion_status", resp.StatusCode))
	}
	return nil
}
