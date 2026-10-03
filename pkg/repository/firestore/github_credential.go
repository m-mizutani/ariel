package firestore

import (
	"context"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model"
)

type githubCredentialRepository struct {
	client *firestore.Client
}

func (r *githubCredentialRepository) doc(key model.UserKey) *firestore.DocumentRef {
	return userDoc(r.client, key).Collection(credentialsCollection).Doc(githubCredentialDocID)
}

// accountDoc holds the owner of one GitHub account. It lives outside the
// user's document because it has to be found by the GitHub account alone.
func (r *githubCredentialRepository) accountDoc(id model.GitHubUserID) *firestore.DocumentRef {
	return r.client.Collection(githubAccountsCollection).Doc(strconv.FormatInt(int64(id), 10))
}

func checkGitHubCredential(key model.UserKey, cred *model.GitHubCredential) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	if err := cred.Validate(); err != nil {
		return goerr.Wrap(err, "invalid github credential")
	}
	if cred.Key() != key {
		return goerr.Wrap(interfaces.ErrKeyMismatch, "github credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

// current reads the stored credential inside tx. It returns nil when none is
// stored.
func (r *githubCredentialRepository) current(tx *firestore.Transaction, key model.UserKey) (*model.GitHubCredential, error) {
	snap, err := tx.Get(r.doc(key))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get github credential")
	}
	var cred model.GitHubCredential
	if err := snap.DataTo(&cred); err != nil {
		return nil, goerr.Wrap(err, "failed to decode github credential")
	}
	if cred.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored github credential belongs to another key")
	}
	return &cred, nil
}

// accountOwner reads the owner of the GitHub account inside tx. It returns
// nil when no user owns the account.
func (r *githubCredentialRepository) accountOwner(tx *firestore.Transaction, id model.GitHubUserID) (*model.UserKey, error) {
	snap, err := tx.Get(r.accountDoc(id))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get github account")
	}
	var account model.GitHubAccount
	if err := snap.DataTo(&account); err != nil {
		return nil, goerr.Wrap(err, "failed to decode github account")
	}
	owner := account.Key()
	return &owner, nil
}

func (r *githubCredentialRepository) Create(ctx context.Context, key model.UserKey, cred *model.GitHubCredential) error {
	if err := checkGitHubCredential(key, cred); err != nil {
		return err
	}

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		existing, err := r.current(tx, key)
		if err != nil {
			return err
		}
		if existing != nil {
			return goerr.Wrap(interfaces.ErrAlreadyExists, "user already has a github credential")
		}
		owner, err := r.accountOwner(tx, cred.GitHubUserID)
		if err != nil {
			return err
		}
		if owner != nil && *owner != key {
			return goerr.Wrap(interfaces.ErrGitHubAccountInUse, "github account is connected to another user")
		}

		account := &model.GitHubAccount{
			GitHubUserID: cred.GitHubUserID,
			TeamID:       key.TeamID,
			UserID:       key.UserID,
			CreatedAt:    cred.CreatedAt,
		}
		if err := tx.Set(r.accountDoc(cred.GitHubUserID), account); err != nil {
			return goerr.Wrap(err, "failed to put github account")
		}
		if err := tx.Create(r.doc(key), cred); err != nil {
			return goerr.Wrap(err, "failed to create github credential")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "failed to create github credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.V("github_user_id", cred.GitHubUserID))
	}
	return nil
}

func (r *githubCredentialRepository) Get(ctx context.Context, key model.UserKey) (*model.GitHubCredential, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid user key")
	}
	snap, err := r.doc(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "github credential not found",
				goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
		}
		return nil, goerr.Wrap(err, "failed to get github credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}

	var cred model.GitHubCredential
	if err := snap.DataTo(&cred); err != nil {
		return nil, goerr.Wrap(err, "failed to decode github credential")
	}
	if cred.Key() != key {
		return nil, goerr.Wrap(interfaces.ErrKeyMismatch, "stored github credential belongs to another key",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return &cred, nil
}

func (r *githubCredentialRepository) AccountInUse(ctx context.Context, key model.UserKey, id model.GitHubUserID) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}
	if id <= 0 {
		return false, goerr.New("invalid github user ID", goerr.V("github_user_id", id))
	}
	snap, err := r.accountDoc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, goerr.Wrap(err, "failed to get github account",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.V("github_user_id", id))
	}
	var account model.GitHubAccount
	if err := snap.DataTo(&account); err != nil {
		return false, goerr.Wrap(err, "failed to decode github account")
	}
	return account.Key() != key, nil
}

func (r *githubCredentialRepository) AcquireRefreshLease(ctx context.Context, key model.UserKey, leaseID string, now, expiresAt time.Time) (*model.GitHubCredential, bool, error) {
	if err := key.Validate(); err != nil {
		return nil, false, goerr.Wrap(err, "invalid user key")
	}
	if leaseID == "" || !now.Before(expiresAt) {
		return nil, false, goerr.New("invalid refresh lease", goerr.V("lease_id", leaseID))
	}

	var (
		stored   *model.GitHubCredential
		acquired bool
	)
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		stored, acquired = nil, false
		cred, err := r.current(tx, key)
		if err != nil {
			return err
		}
		if cred == nil {
			return goerr.Wrap(interfaces.ErrNotFound, "github credential not found")
		}
		stored = cred
		if cred.LeaseHeld(now) {
			return nil
		}
		cred.RefreshLeaseID = leaseID
		cred.RefreshLeaseExpiresAt = expiresAt
		if err := tx.Set(r.doc(key), cred); err != nil {
			return goerr.Wrap(err, "failed to set github refresh lease")
		}
		acquired = true
		return nil
	})
	if err != nil {
		return nil, false, goerr.Wrap(err, "failed to acquire github refresh lease",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return stored, acquired, nil
}

func (r *githubCredentialRepository) ReplaceIfLeaseHeld(ctx context.Context, key model.UserKey, leaseID string, cred *model.GitHubCredential) (bool, error) {
	if err := checkGitHubCredential(key, cred); err != nil {
		return false, err
	}
	if cred.RefreshLeaseID != "" {
		return false, goerr.New("replacement github credential carries a lease")
	}

	var replaced bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		replaced = false
		current, err := r.current(tx, key)
		if err != nil {
			return err
		}
		if current == nil || current.ConnectionID != cred.ConnectionID || current.RefreshLeaseID != leaseID {
			return nil
		}
		if err := tx.Set(r.doc(key), cred); err != nil {
			return goerr.Wrap(err, "failed to replace github credential")
		}
		replaced = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to replace github credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return replaced, nil
}

func (r *githubCredentialRepository) ReleaseRefreshLease(ctx context.Context, key model.UserKey, leaseID string) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid user key")
	}
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		current, err := r.current(tx, key)
		if err != nil {
			return err
		}
		if current == nil || current.RefreshLeaseID != leaseID {
			return nil
		}
		current.RefreshLeaseID = ""
		current.RefreshLeaseExpiresAt = time.Time{}
		if err := tx.Set(r.doc(key), current); err != nil {
			return goerr.Wrap(err, "failed to release github refresh lease")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "failed to release github refresh lease",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID))
	}
	return nil
}

func (r *githubCredentialRepository) DeleteIfConnection(ctx context.Context, key model.UserKey, connectionID string) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid user key")
	}

	var deleted bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		deleted = false
		current, err := r.current(tx, key)
		if err != nil {
			return err
		}
		if current == nil || current.ConnectionID != connectionID {
			return nil
		}
		owner, err := r.accountOwner(tx, current.GitHubUserID)
		if err != nil {
			return err
		}

		if err := tx.Delete(r.doc(key)); err != nil {
			return goerr.Wrap(err, "failed to delete github credential")
		}
		if owner != nil && *owner == key {
			if err := tx.Delete(r.accountDoc(current.GitHubUserID)); err != nil {
				return goerr.Wrap(err, "failed to delete github account")
			}
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "failed to delete github credential",
			goerr.V("team_id", key.TeamID), goerr.V("user_id", key.UserID), goerr.V("connection_id", connectionID))
	}
	return deleted, nil
}
