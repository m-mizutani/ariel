package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/cli/config"
	"github.com/m-mizutani/ariel/pkg/repository/memory"
)

func TestRepository_Validate(t *testing.T) {
	unsetEnv(t, "ARIEL_REPOSITORY_BACKEND", "ARIEL_FIRESTORE_PROJECT_ID", "ARIEL_FIRESTORE_DATABASE_ID")

	t.Run("firestore requires a project ID", func(t *testing.T) {
		var r config.Repository
		parse(t, r.Flags())
		err := r.Validate()
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("--firestore-project-id")
	})

	t.Run("firestore with project ID", func(t *testing.T) {
		var r config.Repository
		parse(t, r.Flags(), "--firestore-project-id", "my-project")
		gt.NoError(t, r.Validate())
	})

	t.Run("memory needs no project ID", func(t *testing.T) {
		var r config.Repository
		parse(t, r.Flags(), "--repository-backend", "memory")
		gt.NoError(t, r.Validate()).Required()

		repo, err := r.Configure(context.Background())
		gt.NoError(t, err).Required()
		_, ok := repo.(*memory.Memory)
		gt.Bool(t, ok).True()
	})

	t.Run("unknown backend", func(t *testing.T) {
		var r config.Repository
		parse(t, r.Flags(), "--repository-backend", "postgres")
		gt.Error(t, r.Validate())
	})
}
