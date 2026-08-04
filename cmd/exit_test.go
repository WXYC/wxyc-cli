package cmd

import (
	"fmt"
	"testing"

	"github.com/WXYC/wxyc-cli/internal/auth"
)

// A dead or empty session surfaced from the token provider must map to ExitAuth
// (3), the CLI's machine-readable "you need to (re-)login" signal, not the
// catch-all ExitError (1).
func TestMapExit_SessionFailuresAreAuth(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"no session", auth.ErrNoSession},
		{"wrapped no session", fmt.Errorf("reading session token: %w", auth.ErrNoSession)},
		{"expired session", auth.ErrSessionExpired},
		{"wrapped expired session", fmt.Errorf("exchanging session: %w", auth.ErrSessionExpired)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapExit(tc.err); got != ExitAuth {
				t.Errorf("mapExit(%v) = %d, want ExitAuth(%d)", tc.err, got, ExitAuth)
			}
		})
	}
}
