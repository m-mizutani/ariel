package http

import (
	"context"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack/slackevents"

	"github.com/m-mizutani/ariel/frontend"
	"github.com/m-mizutani/ariel/pkg/domain/model"
	"github.com/m-mizutani/ariel/pkg/domain/model/auth"
	"github.com/m-mizutani/ariel/pkg/usecase"
	"github.com/m-mizutani/ariel/pkg/utils/safe"
)

type authUseCase interface {
	AuthorizeURL(state string) string
	HandleCallback(ctx context.Context, code string) (*auth.Session, auth.SessionSecret, error)
	Authenticate(ctx context.Context, id auth.SessionID, secret auth.SessionSecret) (*auth.Session, error)
	Logout(ctx context.Context, id auth.SessionID, secret auth.SessionSecret) error
	Me(ctx context.Context, key model.UserKey) (*usecase.Me, error)
}

type slackEventUseCase interface {
	HandleEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error
}

type Config struct {
	// BaseURL decides the Secure cookie attribute. A TLS-terminating proxy
	// hides TLS from the request, so the scheme of the public URL is used.
	BaseURL string
	// Static is the SPA file system. nil serves the embedded frontend build.
	Static fs.FS
}

type Server struct {
	router       *chi.Mux
	authUC       authUseCase
	slackUC      slackEventUseCase
	secureCookie bool
}

type Option func(*options)

type options struct {
	slackUC            slackEventUseCase
	slackSigningSecret string
}

// WithSlackEvents mounts POST /hooks/slack/event. Without it the endpoint does
// not exist, which is how a no-auth development server without Slack runs.
func WithSlackEvents(uc slackEventUseCase, signingSecret string) Option {
	return func(o *options) {
		o.slackUC = uc
		o.slackSigningSecret = signingSecret
	}
}

func New(authUC authUseCase, cfg Config, opts ...Option) (*Server, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.slackUC != nil && o.slackSigningSecret == "" {
		return nil, goerr.New("slack events need a signing secret")
	}

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid base URL", goerr.V("base_url", cfg.BaseURL))
	}

	static := cfg.Static
	if static == nil {
		static, err = fs.Sub(frontend.StaticFiles, "dist")
		if err != nil {
			return nil, goerr.Wrap(err, "failed to open embedded frontend")
		}
	}

	s := &Server{
		router:       chi.NewRouter(),
		authUC:       authUC,
		slackUC:      o.slackUC,
		secureCookie: base.Scheme == "https",
	}

	r := s.router
	r.Use(middleware.RequestID)
	r.Use(accessLogger)
	r.Use(middleware.Recoverer)

	apiNotFound := func(w http.ResponseWriter, r *http.Request) {
		writeError(r.Context(), w, http.StatusNotFound, errCodeNotFound)
	}
	r.Route("/api/auth", func(r chi.Router) {
		r.Get("/login", s.authLoginHandler)
		r.Get("/callback", s.authCallbackHandler)
		r.Post("/logout", s.authLogoutHandler)
		r.With(requireSession(authUC)).Get("/me", s.authMeHandler)
		r.NotFound(apiNotFound)
	})
	r.HandleFunc("/api/*", apiNotFound)

	if o.slackUC != nil {
		r.With(slackSignatureMiddleware(o.slackSigningSecret)).Post("/hooks/slack/event", s.slackEventHandler)
	}

	r.Get("/*", spaHandler(static))

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// spaHandler serves a file when it exists and index.html otherwise, so the
// client-side router handles paths such as /login.
func spaHandler(static fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(static))
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := static.Open(path); err == nil {
				safe.Close(r.Context(), f)
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := static.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer safe.Close(r.Context(), index)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		safe.Copy(r.Context(), w, index)
	}
}
