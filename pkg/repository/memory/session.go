package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/domain/model/auth"
)

type sessionRepository struct {
	mu       sync.Mutex
	sessions map[auth.SessionID]auth.Session
}

func newSessionRepository() *sessionRepository {
	return &sessionRepository{sessions: make(map[auth.SessionID]auth.Session)}
}

func (r *sessionRepository) Create(_ context.Context, session *auth.Session) error {
	if err := session.Validate(); err != nil {
		return goerr.Wrap(err, "invalid session")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.ID]; ok {
		return goerr.Wrap(interfaces.ErrAlreadyExists, "session already exists", goerr.V("session_id", session.ID))
	}
	s := *session
	s.SecretHash = slices.Clone(s.SecretHash)
	r.sessions[session.ID] = s
	return nil
}

func (r *sessionRepository) Get(_ context.Context, id auth.SessionID) (*auth.Session, error) {
	if err := id.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid session ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrNotFound, "session not found", goerr.V("session_id", id))
	}
	s.SecretHash = slices.Clone(s.SecretHash)
	return &s, nil
}

func (r *sessionRepository) Delete(_ context.Context, id auth.SessionID) error {
	if err := id.Validate(); err != nil {
		return goerr.Wrap(err, "invalid session ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}
