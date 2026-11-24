package cmd

import (
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
			fmt.Fprint(app.stderr, "WXYC email or username: ")
			ident, err := readLine(app.stdin)
			if err != nil {
				return fmt.Errorf("reading email or username: %w", err)
			}
			if ident == "" {
				return errors.New("an email or username is required")
			}

			fmt.Fprint(app.stderr, "Password: ")
			pw, err := app.readPassword()
			fmt.Fprintln(app.stderr) // the terminal swallowed the Enter
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
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
	for {
		n, err := r.Read(b)
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
