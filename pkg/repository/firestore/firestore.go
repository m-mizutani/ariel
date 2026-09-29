package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

const (
	teamsCollection                = "teams"
	usersCollection                = "users"
	credentialsCollection          = "credentials"
	slackCredentialDocID           = "slack"
	googleWorkspaceCredentialDocID = "google_workspace"
	sessionsCollection             = "sessions"
	slackEventsCollection          = "slackEvents"
)

type Firestore struct {
	client                    *firestore.Client
	user                      *userRepository
	slackCredential           *slackCredentialRepository
	googleWorkspaceCredential *googleWorkspaceCredentialRepository
	session                   *sessionRepository
	slackEvent                *slackEventRepository
}

var _ interfaces.Repository = &Firestore{}

// New connects to Firestore. An empty databaseID selects the default database.
func New(ctx context.Context, projectID, databaseID string) (*Firestore, error) {
	var client *firestore.Client
	var err error
	if databaseID != "" {
		client, err = firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	} else {
		client, err = firestore.NewClient(ctx, projectID)
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create firestore client",
			goerr.V("project_id", projectID),
			goerr.V("database_id", databaseID),
		)
	}

	return &Firestore{
		client:                    client,
		user:                      &userRepository{client: client},
		slackCredential:           &slackCredentialRepository{client: client},
		googleWorkspaceCredential: &googleWorkspaceCredentialRepository{client: client},
		session:                   &sessionRepository{client: client},
		slackEvent:                &slackEventRepository{client: client},
	}, nil
}

func (f *Firestore) User() interfaces.UserRepository                       { return f.user }
func (f *Firestore) SlackCredential() interfaces.SlackCredentialRepository { return f.slackCredential }
func (f *Firestore) GoogleWorkspaceCredential() interfaces.GoogleWorkspaceCredentialRepository {
	return f.googleWorkspaceCredential
}
func (f *Firestore) Session() interfaces.SessionRepository       { return f.session }
func (f *Firestore) SlackEvent() interfaces.SlackEventRepository { return f.slackEvent }

func (f *Firestore) Close() error {
	return f.client.Close()
}

// userDoc is the only place that builds a user document path. All user-owned
// documents hang below it, so a path for another user can only be produced
// from another user's key.
func userDoc(client *firestore.Client, key model.UserKey) *firestore.DocumentRef {
	return client.Collection(teamsCollection).Doc(string(key.TeamID)).
		Collection(usersCollection).Doc(string(key.UserID))
}
