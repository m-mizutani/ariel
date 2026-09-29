package interfaces

import (
	"context"

	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
)

// Repository is the persistence boundary. Every method that reads or writes
// data owned by a user takes that user's key, and no method returns data of
// more than one user.
type Repository interface {
	User() UserRepository
	SlackCredential() SlackCredentialRepository
	GoogleWorkspaceCredential() GoogleWorkspaceCredentialRepository
	NotionCredential() NotionCredentialRepository
	Session() SessionRepository
	SlackEvent() SlackEventRepository
	Close() error
}

type UserRepository interface {
	Put(ctx context.Context, user *model.User) error
	Get(ctx context.Context, key model.UserKey) (*model.User, error)
}

type SlackCredentialRepository interface {
	Put(ctx context.Context, key model.UserKey, cred *model.SlackCredential) error
	Get(ctx context.Context, key model.UserKey) (*model.SlackCredential, error)
	// DeleteIfUnchanged removes the credential only while it still holds the
	// ciphertext of expected, so a credential replaced by a newer sign-in
	// survives. It reports whether a credential was deleted; a missing or
	// replaced credential is not an error.
	DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.SlackCredential) (bool, error)
}

// GoogleWorkspaceCredentialRepository keeps at most one credential per user
// and connects each Google account (by cred.Subject) to at most one user. The
// owner of a Google account is recorded outside the user's document; it is
// written and deleted together with the credential and never returned.
type GoogleWorkspaceCredentialRepository interface {
	// Create stores the credential and makes key the owner of its Google
	// account, atomically. It fails with ErrAlreadyExists when key already has
	// a credential, and with ErrGoogleAccountInUse when another user owns the
	// Google account.
	Create(ctx context.Context, key model.UserKey, cred *model.GoogleWorkspaceCredential) error
	Get(ctx context.Context, key model.UserKey) (*model.GoogleWorkspaceCredential, error)
	// AccountInUse reports whether a user other than key owns the Google
	// account identified by subject.
	AccountInUse(ctx context.Context, key model.UserKey, subject string) (bool, error)
	// DeleteIfUnchanged removes the credential, and the ownership of its
	// Google account, only while the credential still holds the ciphertext of
	// expected, so a credential stored by a later connection survives. It
	// reports whether a credential was deleted; a missing or replaced
	// credential is not an error.
	DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.GoogleWorkspaceCredential) (bool, error)
}

// NotionCredentialRepository keeps at most one credential per user and
// connects each Notion account (by cred.NotionUserID) to at most one user. The
// owner of a Notion account is recorded outside the user's document; it is
// written and deleted together with the credential and never returned.
type NotionCredentialRepository interface {
	// Create stores the credential and makes key the owner of its Notion
	// account, atomically. It fails with ErrAlreadyExists when key already has
	// a credential, and with ErrNotionAccountInUse when another user owns the
	// Notion account.
	Create(ctx context.Context, key model.UserKey, cred *model.NotionCredential) error
	Get(ctx context.Context, key model.UserKey) (*model.NotionCredential, error)
	// AccountInUse reports whether a user other than key owns the Notion
	// account identified by notionUserID.
	AccountInUse(ctx context.Context, key model.UserKey, notionUserID model.NotionUserID) (bool, error)
	// UpdateIfUnchanged replaces the credential with next only while it still
	// holds the token ciphertext of expected, moving the ownership when next
	// is for another Notion account. It reports whether it replaced the
	// credential; a missing or replaced credential is not an error. It fails
	// with ErrNotionAccountInUse when another user owns next's Notion account.
	UpdateIfUnchanged(ctx context.Context, key model.UserKey, expected, next *model.NotionCredential) (bool, error)
	// DeleteIfUnchanged removes the credential, and the ownership of its
	// Notion account, only while the credential still holds the ciphertext of
	// expected. It reports whether a credential was deleted; a missing or
	// replaced credential is not an error.
	DeleteIfUnchanged(ctx context.Context, key model.UserKey, expected *model.NotionCredential) (bool, error)
}

type SessionRepository interface {
	// Create fails with ErrAlreadyExists when the ID is taken.
	Create(ctx context.Context, session *auth.Session) error
	Get(ctx context.Context, id auth.SessionID) (*auth.Session, error)
	// Delete removes the session. A missing session is not an error.
	Delete(ctx context.Context, id auth.SessionID) error
}

type SlackEventRepository interface {
	// Claim creates the claim if absent. It returns true only for the first
	// caller, across all instances.
	Claim(ctx context.Context, claim *model.SlackEventClaim) (bool, error)
}
