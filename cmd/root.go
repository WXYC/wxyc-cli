package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/WXYC/wxyc-cli/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

// Execute runs the CLI against the real process (args, environment,
// terminal, keychain) and returns a process exit code.
func Execute() int {
	app := &App{
		getenv:       os.Getenv,
		newStore:     auth.NewStore,
		stdout:       os.Stdout,
		stderr:       os.Stderr,
		stdin:        os.Stdin,
		readPassword: terminalPassword,
	}
	ctx, stop := interruptContext(context.Background())
	defer stop()
	return execute(ctx, app, os.Args[1:])
}

// terminalPassword reads a password from the terminal without echo. On
// Ctrl-C it restores the terminal state itself and returns ctx.Err(), so the
// shell is never left with echo off: term.ReadPassword would only restore it
// on return, which a blocked read never reaches before the process exits.
func terminalPassword(ctx context.Context) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	return awaitCtx(ctx, func() { term.Restore(fd, state) }, func() ([]byte, error) {
		return term.ReadPassword(fd)
	})
}

// execute runs args on app, reports any error on app.stderr, and returns the
// exit code. It is Execute minus the process wiring, so tests can drive the
// stderr line and exit-code contract end to end.
func execute(ctx context.Context, app *App, args []string) int {
	return report(app.stderr, run(ctx, app, args))
}

// interruptContext returns a context cancelled by the first Ctrl-C, which
// aborts any in-flight request and login's prompts. The handler is removed as
// soon as that signal arrives, so a second Ctrl-C gets the default behaviour
// and kills the process — the way out of anything that ignores the context.
func interruptContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		select {
		case <-sigs:
		case <-ctx.Done():
		}
		// Restore the default handler before cancelling, so by the time
		// anything observes the cancellation a second Ctrl-C already kills.
		signal.Stop(sigs)
		cancel()
	}()
	return ctx, cancel
}

// report prints err (if any) to w and returns the process exit code. A Ctrl-C
// prints "interrupted" rather than a "context canceled" error.
func report(w io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, "interrupted")
	} else {
		fmt.Fprintln(w, "error:", err)
	}
	return mapExit(err)
}

// run executes the command tree for args on app.
func run(ctx context.Context, app *App, args []string) error {
	root := newRoot(app)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}
