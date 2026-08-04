package cmd

import (
	"context"
	"net/http"
	"time"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/WXYC/wxyc-cli/internal/config"
	"github.com/WXYC/wxyc-cli/internal/output"
	"github.com/WXYC/wxyc-cli/internal/safety"
	"io"
)

// App holds the wired-up dependencies for a single CLI invocation. It is
// populated by build() during the root command's PersistentPreRunE, so every
// subcommand sees the same configured client, gate, and renderer.
type App struct {
	cfg      config.Config
	gate     safety.Gate
	render   output.Renderer
	store    auth.Store
	provider *auth.TokenProvider // nil when a static WXYC_JWT is supplied
	client   *api.Client
	token    func(context.Context) (string, error) // resolves the current JWT
	stdout   io.Writer
	stderr   io.Writer
}

// build assembles the App from resolved flags and the environment.
func (a *App) build(getenv func(string) string, profile string, jsonOut, allowWrite bool) {
	a.cfg = config.Resolve(getenv)
	if profile != "" {
		a.cfg.Profile = profile
	}
	a.gate = safety.Gate{AllowWrite: allowWrite || getenv("WXYC_ALLOW_WRITE") == "1"}
	a.render = output.Renderer{JSON: jsonOut, Out: a.stdout}
	a.store = auth.NewStore()

	// Token source: a static WXYC_JWT (for agents/CI that manage their own
	// tokens) short-circuits the session exchange; otherwise the provider
	// exchanges the stored session token on demand.
	if jwt := getenv("WXYC_JWT"); jwt != "" {
		a.token = func(context.Context) (string, error) { return jwt, nil }
	} else {
		a.provider = &auth.TokenProvider{
			AuthBase: a.cfg.AuthBase,
			HTTP:     http.DefaultClient,
			Session:  func() (string, error) { return a.store.Load(a.cfg.Profile) },
			Skew:     30 * time.Second,
		}
		a.token = a.provider.Token
	}

	refresh := a.token
	if a.provider != nil {
		refresh = a.provider.Refresh
	}
	transport := &auth.Transport{Token: a.token, Refresh: refresh}
	a.client = &api.Client{
		BaseURL: a.cfg.APIBase,
		HTTP:    &http.Client{Transport: transport, Timeout: 35 * time.Second},
	}
}
