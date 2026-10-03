package slack

import (
	"errors"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
)

// tokenInvalidErrors are the Slack error codes that mean the token can no
// longer be used and the user has to authorize again.
var tokenInvalidErrors = map[string]struct{}{
	"invalid_auth":     {},
	"not_authed":       {},
	"token_revoked":    {},
	"token_expired":    {},
	"account_inactive": {},
}

// wrapError converts a Slack API error into a goerr error. Token errors wrap
// interfaces.ErrSlackTokenInvalid so callers can detect them with errors.Is.
func wrapError(err error, msg string, opts ...goerr.Option) error {
	var slackErr slack.SlackErrorResponse
	if errors.As(err, &slackErr) {
		opts = append(opts, goerr.V("slack_error", slackErr.Err))
		if _, ok := tokenInvalidErrors[slackErr.Err]; ok {
			return goerr.Wrap(interfaces.ErrSlackTokenInvalid, msg, opts...)
		}
	}
	return goerr.Wrap(err, msg, opts...)
}
