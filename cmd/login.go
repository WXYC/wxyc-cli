package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in and store a session token (password never stored)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ident, err := prompt("WXYC email or username: ")
			if err != nil {
				return err
			}
			password, err := promptSecret("Password: ")
			if err != nil {
				return err
			}

			strategy := auth.PasswordStrategy{
				AuthBase: app.cfg.AuthBase,
				HTTP:     &http.Client{Timeout: 30 * time.Second},
				Ident:    ident,
				Password: password,
			}
			token, err := strategy.Login(cmd.Context())
			if err != nil {
				return err
			}
			if err := app.store.Save(app.cfg.Profile, token); err != nil {
				return fmt.Errorf("saving session: %w", err)
			}
			fmt.Fprintf(app.stdout, "signed in as %s (profile %q)\n", ident, app.cfg.Profile)
			return nil
		},
	}
}

func prompt(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	var s string
	if _, err := fmt.Scanln(&s); err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func promptSecret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
