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

	// ErrGoogleScopeNotGranted means the user did not allow every required
	// Google scope on the consent screen.
	ErrGoogleScopeNotGranted = errors.New("required google scope was not granted")

	// ErrGoogleConnectRejected means Google completed the authorization but
	// the result cannot be stored (no refresh token, no account identity).
	ErrGoogleConnectRejected = errors.New("google connection rejected")

	// ErrGoogleWorkspaceNotConnected means no Google refresh token is stored
	// for the user.
	ErrGoogleWorkspaceNotConnected = errors.New("google workspace is not connected")

	// ErrGoogleAccountInUse means the Google account the user authorized is
	// already connected to another user.
	ErrGoogleAccountInUse = errors.New("google account is connected to another user")

	// ErrGoogleWorkspaceAlreadyConnected means the user already has a
	// connected Google account; a new connection is ignored.
	ErrGoogleWorkspaceAlreadyConnected = errors.New("google workspace is already connected")
)
