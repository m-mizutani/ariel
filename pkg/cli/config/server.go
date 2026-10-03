package config

import (
	"net/url"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"
)

type Server struct {
	addr       string
	baseURL    string
	sessionTTL time.Duration
}

func (x *Server) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "addr",
			Category:    "Server",
			Usage:       "Address the HTTP server listens on",
			Value:       ":8080",
			Sources:     cli.EnvVars("ROBIN_ADDR"),
			Destination: &x.addr,
		},
		&cli.StringFlag{
			Name:        "base-url",
			Category:    "Server",
			Usage:       "Public URL of this server (scheme://host[:port]), used for the OAuth callback, links in Slack, and the Secure cookie attribute",
			Sources:     cli.EnvVars("ROBIN_BASE_URL"),
			Destination: &x.baseURL,
		},
		&cli.DurationFlag{
			Name:        "session-ttl",
			Category:    "Server",
			Usage:       "Lifetime of a web session",
			Value:       7 * 24 * time.Hour,
			Sources:     cli.EnvVars("ROBIN_SESSION_TTL"),
			Destination: &x.sessionTTL,
		},
	}
}

// Validate checks the settings and removes a trailing slash from the base URL.
func (x *Server) Validate() error {
	if x.baseURL == "" {
		return goerr.New("--base-url is required")
	}
	x.baseURL = strings.TrimSuffix(x.baseURL, "/")
	u, err := url.Parse(x.baseURL)
	if err != nil {
		return goerr.Wrap(err, "invalid --base-url", goerr.V("base_url", x.baseURL))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return goerr.New("--base-url must use http or https", goerr.V("base_url", x.baseURL))
	}
	if u.Host == "" {
		return goerr.New("--base-url has no host", goerr.V("base_url", x.baseURL))
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return goerr.New("--base-url must be scheme://host[:port] without path, query, or credentials",
			goerr.V("base_url", x.baseURL))
	}
	if x.sessionTTL <= 0 {
		return goerr.New("--session-ttl must be positive", goerr.V("session_ttl", x.sessionTTL))
	}
	return nil
}

func (x *Server) Addr() string              { return x.addr }
func (x *Server) BaseURL() string           { return x.baseURL }
func (x *Server) SessionTTL() time.Duration { return x.sessionTTL }
