package cmd

import (
	"errors"
	"net/http"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/WXYC/wxyc-cli/internal/safety"
)

// Exit codes form the CLI's machine-readable contract. An agent can branch on
// these without parsing stderr.
const (
	ExitOK        = 0
	ExitError     = 1 // unclassified failure
	ExitBlocked   = 2 // write attempted while read-only (BlockedError)
	ExitAuth      = 3 // missing session or 401
	ExitForbidden = 4 // 403: authenticated but role lacks permission
	ExitNotFound  = 5 // 404
)

// mapExit classifies an error into a process exit code.
func mapExit(err error) int {
	if err == nil {
		return ExitOK
	}
	var blocked *safety.BlockedError
	if errors.As(err, &blocked) {
		return ExitBlocked
	}
	if errors.Is(err, auth.ErrNoSession) || errors.Is(err, auth.ErrSessionExpired) {
		return ExitAuth
	}
	var se *api.StatusError
	if errors.As(err, &se) {
		switch se.Code {
		case http.StatusUnauthorized:
			return ExitAuth
		case http.StatusForbidden:
			return ExitForbidden
		case http.StatusNotFound:
			return ExitNotFound
		}
	}
	return ExitError
}
