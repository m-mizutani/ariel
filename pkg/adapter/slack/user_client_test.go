package slack_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	slackgo "github.com/slack-go/slack"

	"github.com/m-mizutani/robin/pkg/adapter/slack"
	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

func TestUserClient_AuthTest(t *testing.T) {
	fake := newFakeSlack(t, map[string]string{
		"/api/auth.test": `{"ok":true,"team_id":"T0123ABCD","user_id":"U0123ABCD","user":"alice","team":"Example"}`,
	})
	client := slack.NewUserClientFactory(slackgo.OptionAPIURL(fake.apiURL())).New("xoxp-user-token")

	got, err := client.AuthTest(context.Background())
	gt.NoError(t, err).Required()
	gt.Value(t, got.TeamID).Equal(model.SlackTeamID("T0123ABCD"))
	gt.Value(t, got.UserID).Equal(model.SlackUserID("U0123ABCD"))

	reqs := fake.recorded()
	gt.Array(t, reqs).Length(1).Required()
	gt.String(t, reqs[0].Form.Get("token")).Equal("xoxp-user-token")
}

func TestUserClient_AuthTestErrors(t *testing.T) {
	cases := map[string]bool{
		"invalid_auth":     true,
		"not_authed":       true,
		"token_revoked":    true,
		"token_expired":    true,
		"account_inactive": true,
		"ratelimited":      false,
	}
	for code, invalid := range cases {
		t.Run(code, func(t *testing.T) {
			fake := newFakeSlack(t, map[string]string{
				"/api/auth.test": `{"ok":false,"error":"` + code + `"}`,
			})
			client := slack.NewUserClientFactory(slackgo.OptionAPIURL(fake.apiURL())).New("xoxp-user-token")

			_, err := client.AuthTest(context.Background())
			gt.Value(t, err).NotNil().Required()
			gt.Value(t, errors.Is(err, interfaces.ErrSlackTokenInvalid)).Equal(invalid)
		})
	}
}
