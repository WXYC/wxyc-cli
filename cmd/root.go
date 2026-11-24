package cmd

import (
	"fmt"
	"os"

	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
)

// annotation keys carried on mutating commands.
const (
	annMutates = "mutates" // "true" if the command changes server state
	annOp      = "op"      // short op id used in the write-gate error, e.g. "bin:add"
)

// newRoot builds the command tree. app is populated in PersistentPreRunE and
// shared by reference with every subcommand.
func newRoot(app *App) *cobra.Command {
	var (
		profile    string
		jsonOut    bool
		allowWrite bool
	)

	root := &cobra.Command{
		Use:           "wxyc",
		Short:         "WXYC backend CLI (read-only by default)",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			app.build(profile, jsonOut, allowWrite)
			// Enforce the client-side write gate before any command runs.
			mutates := cmd.Annotations[annMutates] == "true"
			return app.gate.Authorize(cmd.Annotations[annOp], mutates)
		},
	}

	root.PersistentFlags().StringVar(&profile, "profile", "", "credential profile (default \"default\")")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "emit JSON instead of a table")
	root.PersistentFlags().BoolVar(&allowWrite, "write", false, "permit mutating commands (also WXYC_ALLOW_WRITE=1)")

	root.AddCommand(
		newLoginCmd(app),
		newWhoamiCmd(app),
		newLibraryCmd(app),
		newFlowsheetCmd(app),
		newBinCmd(app),
		newDJsCmd(app),
		newLabelsCmd(app),
		newScheduleCmd(app),
	)
	return root
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	app := &App{
		getenv:   os.Getenv,
		newStore: auth.NewStore,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
	}
	root := newRoot(app)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return mapExit(err)
	}
	return ExitOK
}
