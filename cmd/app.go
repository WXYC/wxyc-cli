package cmd

import (
	"context"
	"net/http"
	"time"

	"io"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/WXYC/wxyc-cli/internal/config"
	"github.com/WXYC/wxyc-cli/internal/output"
	"github.com/WXYC/wxyc-cli/internal/safety"
)

// App holds the wired-up dependencies for a single CLI invocation. It is
// populated by build() during the root command's PersistentPreRunE, so every
// subcommand sees the same configured client, gate, and renderer.
type App struct {
	// getenv resolves environment variables. Execute passes os.Getenv; tests
	// pass a hermetic lookup so the developer's shell can't leak in.
	getenv func(string) string
	// newStore constructs the session store. It is called lazily (see
	// sessionStore) so that commands that never touch the store — anything
	// run with WXYC_JWT — don't probe the OS keychain.
	newStore func() auth.Store
	// authTimeout bounds each token exchange; zero means defaultAuthTimeout.
	authTimeout time.Duration

	cfg      config.Config
	gate     safety.Gate
	render   output.Renderer
	store    auth.Store          // built on first use by sessionStore
	provider *auth.TokenProvider // nil when a static WXYC_JWT is supplied
	client   *api.Client
	token    func(context.Context) (string, error) // resolves the current JWT
	stdout   io.Writer
	stderr   io.Writer
}

// defaultAuthTimeout bounds a token exchange. Commands like whoami fetch a
// token directly, outside the API client's own timeout, so the provider's
// client needs its own.
const defaultAuthTimeout = 30 * time.Second

// defaultRequestTimeout bounds each HTTP attempt against the API (the request
// and its 401 retry each get their own), excluding the token exchange, which
// defaultAuthTimeout bounds separately. A whole-request client timeout would
// let a slow token exchange eat the request's budget.
const defaultRequestTimeout = 30 * time.Second

// build assembles the App from resolved flags and the environment.
func (a *App) build(profile string, jsonOut, allowWrite bool) {
	getenv := a.getenv
	a.cfg = config.Resolve(getenv)
	if profile != "" {
		a.cfg.Profile = profile
	}
	a.gate = safety.Gate{AllowWrite: allowWrite || getenv("WXYC_ALLOW_WRITE") == "1"}
	a.render = output.Renderer{JSON: jsonOut, Out: a.stdout}

	// Token source: a static WXYC_JWT (for agents/CI that manage their own
	// tokens) short-circuits the session exchange; otherwise the provider
	// exchanges the stored session token on demand.
	if jwt := getenv("WXYC_JWT"); jwt != "" {
		a.token = func(context.Context) (string, error) { return jwt, nil }
	} else {
		authTimeout := a.authTimeout
		if authTimeout == 0 {
			authTimeout = defaultAuthTimeout
		}
		a.provider = &auth.TokenProvider{
			AuthBase: a.cfg.AuthBase,
			HTTP:     &http.Client{Timeout: authTimeout},
			Session:  func() (string, error) { return a.sessionStore().Load(a.cfg.Profile) },
			Skew:     30 * time.Second,
		}
		a.token = a.provider.Token
	}

	// A static JWT has nothing to refresh to; a nil Refresh makes the
	// transport return a 401 instead of retrying with the same token.
	var refresh func(context.Context) (string, error)
	if a.provider != nil {
		refresh = a.provider.Refresh
	}
	transport := &auth.Transport{Token: a.token, Refresh: refresh, AttemptTimeout: defaultRequestTimeout}
	a.client = &api.Client{
		BaseURL: a.cfg.APIBase,
		HTTP:    &http.Client{Transport: transport},
	}
}

// sessionStore returns the session store, constructing it on first use. Only
// login and the token provider need it, and neither runs when WXYC_JWT is set,
// so a static-JWT invocation never probes the OS keychain.
func (a *App) sessionStore() auth.Store {
	if a.store == nil {
		a.store = a.newStore()
	}
	return a.store
}
