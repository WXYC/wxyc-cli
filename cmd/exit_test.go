package cmd

import (
	"bytes"
	"context"
	"errors"
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

// A Ctrl-C reaches mapExit as context.Canceled; scripts need to tell a user
// abort from a failure, so it gets the conventional 128+SIGINT code.
func TestMapExit_InterruptIs130(t *testing.T) {
	err := fmt.Errorf(`Get "https://api.wxyc.org/flowsheet": %w`, context.Canceled)
	if got := mapExit(err); got != ExitInterrupted || ExitInterrupted != 130 {
		t.Errorf("mapExit(%v) = %d, want 130", err, got)
	}
}

func TestReport_InterruptSaysInterrupted(t *testing.T) {
	var buf bytes.Buffer
	code := report(&buf, fmt.Errorf("whoami: %w", context.Canceled))
	if code != 130 || buf.String() != "interrupted\n" {
		t.Errorf("report() = %d, %q; want 130, %q", code, buf.String(), "interrupted\n")
	}
}

func TestReport_ErrorIsPrefixed(t *testing.T) {
	var buf bytes.Buffer
	code := report(&buf, errors.New("boom"))
	if code != ExitError || buf.String() != "error: boom\n" {
		t.Errorf("report() = %d, %q; want %d, %q", code, buf.String(), ExitError, "error: boom\n")
	}
}
