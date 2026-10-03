package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model/auth"
)

type sessionRepository struct {
	client *firestore.Client
}

func (r *sessionRepository) doc(id auth.SessionID) *firestore.DocumentRef {
	return r.client.Collection(sessionsCollection).Doc(string(id))
}

func (r *sessionRepository) Create(ctx context.Context, session *auth.Session) error {
	if err := session.Validate(); err != nil {
		return goerr.Wrap(err, "invalid session")
	}
	if _, err := r.doc(session.ID).Create(ctx, session); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return goerr.Wrap(interfaces.ErrAlreadyExists, "session already exists",
				goerr.V("session_id", session.ID))
		}
		return goerr.Wrap(err, "failed to create session", goerr.V("session_id", session.ID))
	}
	return nil
}

func (r *sessionRepository) Get(ctx context.Context, id auth.SessionID) (*auth.Session, error) {
	if err := id.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid session ID")
	}
	snap, err := r.doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrNotFound, "session not found", goerr.V("session_id", id))
		}
		return nil, goerr.Wrap(err, "failed to get session", goerr.V("session_id", id))
	}

	var session auth.Session
	if err := snap.DataTo(&session); err != nil {
		return nil, goerr.Wrap(err, "failed to decode session")
	}
	return &session, nil
}

func (r *sessionRepository) Delete(ctx context.Context, id auth.SessionID) error {
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid session ID")
	}
	if _, err := r.doc(id).Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete session", goerr.V("session_id", id))
	}
	return nil
}
