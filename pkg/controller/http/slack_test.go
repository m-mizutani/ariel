package http_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/slack-go/slack/slackevents"

	httpctrl "github.com/m-mizutani/ariel/pkg/controller/http"
	"github.com/m-mizutani/ariel/pkg/utils/async"
)

func sign(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func signedRequest(body string, ts time.Time) *http.Request {
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	r := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Slack-Request-Timestamp", timestamp)
	r.Header.Set("X-Slack-Signature", sign(testSigningSecret, timestamp, body))
	return r
}

func TestVerifySlackSignature(t *testing.T) {
	now := time.Unix(1700000000, 0)
	body := []byte(`{"type":"event_callback"}`)
	ts := strconv.FormatInt(now.Unix(), 10)
	valid := sign(testSigningSecret, ts, string(body))

	gt.NoError(t, httpctrl.VerifySlackSignatureForTest(testSigningSecret, ts, valid, body, now))

	cases := map[string]struct {
		timestamp string
		signature string
		now       time.Time
	}{
		"missing timestamp":  {timestamp: "", signature: valid, now: now},
		"missing signature":  {timestamp: ts, signature: "", now: now},
		"non numeric ts":     {timestamp: "abc", signature: valid, now: now},
		"too old":            {timestamp: ts, signature: valid, now: now.Add(5*time.Minute + time.Second)},
		"too far in future":  {timestamp: ts, signature: valid, now: now.Add(-5*time.Minute - time.Second)},
		"signature mismatch": {timestamp: ts, signature: sign("other-secret", ts, string(body)), now: now},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, httpctrl.VerifySlackSignatureForTest(testSigningSecret, tc.timestamp, tc.signature, body, tc.now))
		})
	}
}

func TestSlackEvent_Rejected(t *testing.T) {
	body := `{"type":"event_callback","team_id":"T0123ABCD","event_id":"Ev001","event":{"type":"app_mention","user":"U0123ABCD","text":"hi","ts":"1.1","channel":"C1","event_ts":"1.1"}}`

	cases := map[string]func() *http.Request{
		"no signature": func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/hooks/slack/event", strings.NewReader(body))
		},
		"wrong signature": func() *http.Request {
			r := signedRequest(body, time.Now())
			r.Header.Set("X-Slack-Signature", "v0=deadbeef")
			return r
		},
		"old timestamp": func() *http.Request {
			return signedRequest(body, time.Now().Add(-10*time.Minute))
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			slackUC := &fakeSlackEventUseCase{}
			srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), slackUC)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, build())
			async.Wait()

			gt.Number(t, w.Code).Equal(http.StatusUnauthorized)
			gt.Array(t, slackUC.handled()).Length(0)
		})
	}
}

func TestSlackEvent_URLVerification(t *testing.T) {
	slackUC := &fakeSlackEventUseCase{}
	srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), slackUC)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, signedRequest(`{"type":"url_verification","token":"x","challenge":"challenge-value"}`, time.Now()))

	gt.Number(t, w.Code).Equal(http.StatusOK)
	gt.String(t, w.Body.String()).Equal("challenge-value")
	gt.String(t, w.Header().Get("Content-Type")).Equal("text/plain")
	gt.Array(t, slackUC.handled()).Length(0)
}

func TestSlackEvent_CallbackIsDispatched(t *testing.T) {
	slackUC := &fakeSlackEventUseCase{}
	srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), slackUC)
	body := `{"type":"event_callback","team_id":"T0123ABCD","event_id":"Ev001","event":{"type":"app_mention","user":"U0123ABCD","text":"hi","ts":"1700000000.000100","channel":"C0123ABCD","event_ts":"1700000000.000100"}}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, signedRequest(body, time.Now()))
	async.Wait()

	gt.Number(t, w.Code).Equal(http.StatusOK)
	events := slackUC.handled()
	gt.Array(t, events).Length(1).Required()
	gt.String(t, events[0].TeamID).Equal("T0123ABCD")
	mention, ok := events[0].InnerEvent.Data.(*slackevents.AppMentionEvent)
	gt.Bool(t, ok).True().Required()
	gt.String(t, mention.User).Equal("U0123ABCD")
	gt.String(t, mention.Channel).Equal("C0123ABCD")
	callback, ok := events[0].Data.(*slackevents.EventsAPICallbackEvent)
	gt.Bool(t, ok).True().Required()
	gt.String(t, callback.EventID).Equal("Ev001")
}

func TestSlackEvent_InvalidJSON(t *testing.T) {
	slackUC := &fakeSlackEventUseCase{}
	srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), slackUC)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, signedRequest(`{not json`, time.Now()))
	async.Wait()

	gt.Number(t, w.Code).Equal(http.StatusBadRequest)
	gt.Array(t, slackUC.handled()).Length(0)
}

func TestSlackEvent_BodyTooLarge(t *testing.T) {
	slackUC := &fakeSlackEventUseCase{}
	srv := newTestServer(t, "https://ariel.example.com", newFakeAuthUseCase(), slackUC)

	body := `{"type":"event_callback","padding":"` + strings.Repeat("a", 1<<20) + `"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, signedRequest(body, time.Now()))
	async.Wait()

	gt.Number(t, w.Code).Equal(http.StatusRequestEntityTooLarge)
	gt.Array(t, slackUC.handled()).Length(0)
}
