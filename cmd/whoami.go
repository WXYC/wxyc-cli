package cmd

import (
	"time"

	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
)

func newWhoamiCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the identity and role of the current token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jwt, err := app.token(cmd.Context())
			if err != nil {
				return err
			}
			c, err := auth.ParseClaims(jwt)
			if err != nil {
				return err
			}
			expires := time.Unix(c.Exp, 0).UTC().Format(time.RFC3339)
			return app.render.Emit(
				map[string]any{"sub": c.Sub, "email": c.Email, "role": c.Role, "expires": expires},
				[]string{"FIELD", "VALUE"},
				[][]string{
					{"sub", c.Sub},
					{"email", c.Email},
					{"role", c.Role},
					{"expires", expires},
				},
			)
		},
	}
}
