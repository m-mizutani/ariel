package slack_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/m-mizutani/gt"
	slackgo "github.com/slack-go/slack"

	"github.com/m-mizutani/robin/pkg/adapter/slack"
)

type recordedRequest struct {
	Path string
	Form url.Values
}

// fakeSlack is an httptest server that records each API call and answers with
// the JSON registered for its path.
type fakeSlack struct {
	mu        sync.Mutex
	requests  []recordedRequest
	responses map[string]string
	server    *httptest.Server
}

func newFakeSlack(t *testing.T, responses map[string]string) *fakeSlack {
	t.Helper()
	f := &fakeSlack{responses: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gt.NoError(t, r.ParseForm())
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{Path: r.URL.Path, Form: r.PostForm})
		body, ok := f.responses[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			body = `{"ok":false,"error":"unknown_method"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeSlack) apiURL() string { return f.server.URL + "/api/" }

func (f *fakeSlack) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func TestBot_PostThreadReply(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/chat.postMessage": `{"ok":true,"channel":"C0123","ts":"1700000000.000200"}`,
	})
	bot := slack.NewBot("xoxb-test", slackgo.OptionAPIURL(fake.apiURL()))

	gt.NoError(t, bot.PostThreadReply(context.Background(), "C0123", "1700000000.000100", "hello")).Required()

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	gt.String(t, reqs[0].Path).Equal("/api/chat.postMessage")
	gt.String(t, reqs[0].Form.Get("channel")).Equal("C0123")
	gt.String(t, reqs[0].Form.Get("thread_ts")).Equal("1700000000.000100")
	gt.String(t, reqs[0].Form.Get("text")).Equal("hello")
}

func TestBot_PostEphemeral(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/chat.postEphemeral": `{"ok":true,"message_ts":"1700000000.000300"}`,
	})
	bot := slack.NewBot("xoxb-test", slackgo.OptionAPIURL(fake.apiURL()))

	gt.NoError(t, bot.PostEphemeral(context.Background(), "C0123", "U0123ABCD", "1700000000.000100", "please sign in")).Required()

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	gt.String(t, reqs[0].Path).Equal("/api/chat.postEphemeral")
	gt.String(t, reqs[0].Form.Get("channel")).Equal("C0123")
	gt.String(t, reqs[0].Form.Get("user")).Equal("U0123ABCD")
	gt.String(t, reqs[0].Form.Get("thread_ts")).Equal("1700000000.000100")
	gt.String(t, reqs[0].Form.Get("text")).Equal("please sign in")
}

func TestBot_PostThreadReplyError(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/chat.postMessage": `{"ok":false,"error":"channel_not_found"}`,
	})
	bot := slack.NewBot("xoxb-test", slackgo.OptionAPIURL(fake.apiURL()))

	gt.Value(t, bot.PostThreadReply(context.Background(), "C0123", "1.1", "hello")).NotNil()
}

func TestBot_GetUserName(t *testing.T) {
	cases := map[string]struct {
		body    string
		want    string
		wantErr bool
	}{
		"real name":          {body: `{"ok":true,"user":{"id":"U0123ABCD","name":"alice","real_name":"Alice Example"}}`, want: "Alice Example"},
		"falls back to name": {body: `{"ok":true,"user":{"id":"U0123ABCD","name":"alice","real_name":""}}`, want: "alice"},
		"no name at all":     {body: `{"ok":true,"user":{"id":"U0123ABCD","name":"","real_name":""}}`, wantErr: true},
		"api error":          {body: `{"ok":false,"error":"user_not_found"}`, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeSlack(t, map[string]string{"/api/users.info": tc.body})
			bot := slack.NewBot("xoxb-test", slackgo.OptionAPIURL(fake.apiURL()))

			got, err := bot.GetUserName(context.Background(), "U0123ABCD")
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err).Required()
			gt.String(t, got).Equal(tc.want)

			reqs := fake.recorded()
			gt.Array(t, reqs).Length(1).Required()
			gt.String(t, reqs[0].Form.Get("user")).Equal("U0123ABCD")
		})
	}
}
