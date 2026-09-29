package usecase

import "errors"

var (
	// ErrUnauthenticated means the web session is missing, unknown, expired,
	// or its secret does not match.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrLoginRejected means Slack completed the authorization but the result
	// does not satisfy the login conditions (workspace, token type, scopes,
	// identity).
	ErrLoginRejected = errors.New("login rejected")

	// ErrSlackNotConnected means no Slack user token is stored for the user.
	ErrSlackNotConnected = errors.New("slack account is not connected")
)
