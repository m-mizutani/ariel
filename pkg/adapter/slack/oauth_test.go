package slack_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	slackgo "github.com/slack-go/slack"

	"github.com/m-mizutani/robin/pkg/adapter/slack"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

func TestOAuth_ExchangeCode(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/oauth.v2.access": `{
			"ok": true,
			"app_id": "A0123",
			"team": {"id": "T0123ABCD", "name": "Example"},
			"authed_user": {
				"id": "U0123ABCD",
				"scope": "search:read,users:read",
				"access_token": "xoxp-user-token",
				"token_type": "user"
			}
		}`,
	})
	oauth := slack.NewOAuth("client-id", "client-secret", slackgo.OAuthOptionAPIURL(fake.apiURL()))

	got, err := oauth.ExchangeCode(context.Background(), "auth-code", "https://robin.example.com/api/auth/callback")
	gt.NoError(t, err).Required()
	gt.Value(t, got.TeamID).Equal(model.SlackTeamID("T0123ABCD"))
	gt.Value(t, got.UserID).Equal(model.SlackUserID("U0123ABCD"))
	gt.Value(t, got.AccessToken).Equal(model.SlackUserToken("xoxp-user-token"))
	gt.String(t, got.TokenType).Equal("user")
	gt.Value(t, got.Scopes).Equal([]string{"search:read", "users:read"})

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	gt.String(t, reqs[0].Form.Get("client_id")).Equal("client-id")
	gt.String(t, reqs[0].Form.Get("client_secret")).Equal("client-secret")
	gt.String(t, reqs[0].Form.Get("code")).Equal("auth-code")
	gt.String(t, reqs[0].Form.Get("redirect_uri")).Equal("https://robin.example.com/api/auth/callback")
}

func TestOAuth_ExchangeCodeError(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/oauth.v2.access": `{"ok":false,"error":"invalid_code"}`,
	})
	oauth := slack.NewOAuth("client-id", "client-secret", slackgo.OAuthOptionAPIURL(fake.apiURL()))

	_, err := oauth.ExchangeCode(context.Background(), "bad-code", "https://robin.example.com/api/auth/callback")
	gt.Value(t, err).NotNil()
}
