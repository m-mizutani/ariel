package config

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/m-mizutani/robin/pkg/domain/interfaces"
	"github.com/m-mizutani/robin/pkg/repository/firestore"
	"github.com/m-mizutani/robin/pkg/repository/memory"
	"github.com/m-mizutani/robin/pkg/utils/logging"
)

const (
	backendFirestore = "firestore"
	backendMemory    = "memory"
)

type Repository struct {
	backend    string
	projectID  string
	databaseID string
}

func (x *Repository) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "repository-backend",
			Category:    "Repository",
			Usage:       "Repository backend [firestore|memory]. memory keeps data in this process only.",
			Value:       backendFirestore,
			Sources:     cli.EnvVars("ROBIN_REPOSITORY_BACKEND"),
			Destination: &x.backend,
		},
		&cli.StringFlag{
			Name:        "firestore-project-id",
			Category:    "Repository",
			Usage:       "Google Cloud project ID of Firestore (required for the firestore backend)",
			Sources:     cli.EnvVars("ROBIN_FIRESTORE_PROJECT_ID"),
			Destination: &x.projectID,
		},
		&cli.StringFlag{
			Name:        "firestore-database-id",
			Category:    "Repository",
			Usage:       "Firestore database ID (empty selects the (default) database)",
			Sources:     cli.EnvVars("ROBIN_FIRESTORE_DATABASE_ID"),
			Destination: &x.databaseID,
		},
	}
}

func (x *Repository) IsMemory() bool {
	return x.backend == backendMemory
}

func (x *Repository) Validate() error {
	switch x.backend {
	case backendFirestore:
		if x.projectID == "" {
			return goerr.New("--firestore-project-id is required for the firestore backend")
		}
	case backendMemory:
	default:
		return goerr.New("invalid --repository-backend", goerr.V("repository_backend", x.backend))
	}
	return nil
}

func (x *Repository) Configure(ctx context.Context) (interfaces.Repository, error) {
	if err := x.Validate(); err != nil {
		return nil, err
	}
	if x.backend == backendMemory {
		logging.Default().Warn("using the in-memory repository; data is lost when the process exits")
		return memory.New(), nil
	}
	repo, err := firestore.New(ctx, x.projectID, x.databaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to initialize firestore repository")
	}
	return repo, nil
}
