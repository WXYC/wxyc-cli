package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
)

func newLoginCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in and store a session token (password never stored)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Refuse a profile the store can't hold before prompting, so
			// login never sends a password it then can't save.
			if err := app.sessionStore().CheckProfile(app.cfg.Profile); err != nil {
				return err
			}
			fmt.Fprint(app.stderr, "WXYC email or username: ")
			// Ctrl-C cancels the context; awaitCtx lets login return at once
			// rather than wait for a line that may never come.
			ident, err := awaitCtx(cmd.Context(), nil, func() (string, error) { return readLine(app.stdin) })
			if errors.Is(err, context.Canceled) {
				return err
			}
			if err != nil {
				return fmt.Errorf("reading email or username: %w", err)
			}
			if ident == "" {
				return errors.New("an email or username is required")
			}

			fmt.Fprint(app.stderr, "Password: ")
			pw, err := app.readPassword(cmd.Context())
			fmt.Fprintln(app.stderr) // the terminal swallowed the Enter
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			if len(pw) == 0 {
				return errors.New("a password is required")
			}

			strategy := auth.PasswordStrategy{
				AuthBase: app.cfg.AuthBase,
				HTTP:     &http.Client{Timeout: 30 * time.Second},
				Ident:    ident,
				Password: string(pw),
			}
			token, err := strategy.Login(cmd.Context())
			if err != nil {
				return err
			}
			if err := app.sessionStore().Save(app.cfg.Profile, token); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}
			fmt.Fprintf(app.stdout, "signed in as %s (profile %q)\n", ident, app.cfg.Profile)
			return nil
		},
	}
}

// readLine reads one line from r and returns it trimmed of surrounding
// whitespace, so identifiers may contain inner spaces. It reads a byte at a
// time rather than through a bufio.Reader, so nothing past the newline is
// consumed from a stdin the password prompt reads next. A final line without
// a newline is accepted; EOF before any input is an error.
func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	b := make([]byte, 1)
	empty := 0
	for {
		n, err := r.Read(b)
		if n == 0 && err == nil {
			// io.Reader permits (0, nil); give up like bufio does rather
			// than spin on a reader that never makes progress.
			if empty++; empty >= maxEmptyReads {
				return "", io.ErrNoProgress
			}
			continue
		}
		empty = 0
		if n == 1 {
			if b[0] == '\n' {
				break
			}
			sb.WriteByte(b[0])
		}
		if err == io.EOF && sb.Len() > 0 {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// maxEmptyReads is how many consecutive (0, nil) reads readLine tolerates,
// matching bufio's limit.
const maxEmptyReads = 100

// awaitCtx runs fn and returns its result, or returns ctx.Err() as soon as
// ctx is cancelled, after calling onCancel (if non-nil). fn keeps running in
// the background: that is acceptable only for a blocking terminal read the
// process is about to exit from anyway.
func awaitCtx[T any](ctx context.Context, onCancel func(), fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		if onCancel != nil {
			onCancel()
		}
		var zero T
		return zero, ctx.Err()
	}
}
