package interfaces

import "errors"

var (
	// ErrNotFound is returned by a repository when the requested entity does
	// not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned by a repository create operation when an
	// entity with the same ID already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrKeyMismatch is returned when an entity's user key differs from the
	// key the caller passed. It guards the per-user document path.
	ErrKeyMismatch = errors.New("entity key does not match the requested user key")

	// ErrSlackTokenInvalid is returned when Slack reports that a token can no
	// longer be used (revoked, expired, account deactivated).
	ErrSlackTokenInvalid = errors.New("slack token is invalid")

	// ErrGoogleTokenInvalid is returned when Google rejects a token as invalid
	// (already revoked, expired, or unknown).
	ErrGoogleTokenInvalid = errors.New("google token is invalid")

	// ErrGoogleAccountInUse is returned when a Google account is already
	// connected to another user.
	ErrGoogleAccountInUse = errors.New("google account is connected to another user")
)
