package notion_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/notion"
)

func TestResponseSizeLimit(t *testing.T) {
	large := `{"object":"page","text":"` + strings.Repeat("a", 10<<20) + `"}`
	fake := newFakeNotion(t, map[string]fakeResponse{"GET /v1/pages/" + string(pageID): {200, large}})
	c := notion.NewClientFactory(fake.server.URL, http.DefaultClient).New("access-1")

	_, err := c.GetPage(context.Background(), pageID)
	gt.Value(t, err).NotNil()
}

func TestRequestFailure(t *testing.T) {
	fake := newFakeNotion(t, nil)
	url := fake.server.URL
	fake.server.Close()
	c := notion.NewClientFactory(url, http.DefaultClient).New("access-1")

	_, err := c.GetPage(context.Background(), pageID)
	gt.Value(t, err).NotNil()
	gt.String(t, err.Error()).NotContains("access-1")
}
