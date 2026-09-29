package github_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/adapter/github"
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func TestUserClient_GetUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		f := newFakeGitHub(t, map[string]fakeResponse{
			"GET /user": {status: http.StatusOK, body: `{"id":583231,"login":"octocat","name":"The Octocat"}`},
		})

		identity, err := github.NewUserClientFactoryForTest(f.server.URL).New("ghu_access").GetUser(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, identity).Equal(&model.GitHubIdentity{ID: 583231, Login: "octocat"})

		reqs := f.recorded()
		gt.Array(t, reqs).Length(1).Required()
		gt.String(t, reqs[0].Header.Get("Authorization")).Equal("Bearer ghu_access")
		gt.String(t, reqs[0].Header.Get("Accept")).Equal("application/vnd.github+json")
		gt.String(t, reqs[0].Header.Get("X-GitHub-Api-Version")).Equal("2026-03-10")
	})

	t.Run("rejected token", func(t *testing.T) {
		f := newFakeGitHub(t, map[string]fakeResponse{"GET /user": {status: http.StatusUnauthorized, body: `{}`}})
		_, err := github.NewUserClientFactoryForTest(f.server.URL).New("ghu_access").GetUser(context.Background())
		gt.Error(t, err).Is(interfaces.ErrGitHubTokenInvalid)
	})

	t.Run("server error", func(t *testing.T) {
		f := newFakeGitHub(t, map[string]fakeResponse{"GET /user": {status: http.StatusInternalServerError, body: `{}`}})
		_, err := github.NewUserClientFactoryForTest(f.server.URL).New("ghu_access").GetUser(context.Background())
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, interfaces.ErrGitHubTokenInvalid)).False()
	})
}
