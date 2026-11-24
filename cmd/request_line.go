package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

// requestLineMode is one <mode> argument: the request_mode it sends (nil for
// null) and its help line, using the DJ-facing names.
type requestLineMode struct {
	arg   string
	mode  *string
	label string
}

// requestLineModes lists the arguments in help order; default sends null,
// returning the show to its program's usual setting.
var requestLineModes = []requestLineMode{
	{"requests", ptr("requests"), "Requests: song requests and notes"},
	{"feedback", ptr("feedback"), "Messages only: notes, no song lookup"},
	{"closed", ptr("closed"), "Closed: nothing"},
	{"default", nil, "the show's usual setting"},
}

func ptr(s string) *string { return &s }

func requestLineArgs() string {
	names := make([]string, len(requestLineModes))
	for i, m := range requestLineModes {
		names[i] = m.arg
	}
	return strings.Join(names, ", ")
}

func newFlowsheetRequestLineCmd(app *App) *cobra.Command {
	var help strings.Builder
	for _, m := range requestLineModes {
		fmt.Fprintf(&help, "\n  %-9s %s", m.arg, m.label)
	}
	return &cobra.Command{
		Use:   "request-line <mode>",
		Short: "Choose what the listener request line is for during the open show",
		Long: `Choose what the listener request line is for during the open show.
The choice belongs to this show only; the next show starts at its usual setting.

<mode> is one of:` + help.String() + `

Prints the request line's effective mode and where it came from.`,
		Example: `  wxyc flowsheet request-line feedback --write`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 && modeFor(args[0]) != nil {
				return nil
			}
			return fmt.Errorf("give one mode: %s", requestLineArgs())
		},
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "flowsheet:request-line"},
		RunE: func(cmd *cobra.Command, args []string) error {
			onAir, _, err := app.client.FlowsheetOnAir(cmd.Context())
			if err != nil {
				return onAirError(err)
			}
			if _, err := app.client.ShowSetRequestMode(cmd.Context(), onAir.ShowID, modeFor(args[0]).mode); err != nil {
				return showEditError(err)
			}
			// The PATCH echoes the show row, not the effective mode, so re-read it.
			onAir, raw, err := app.client.FlowsheetOnAir(cmd.Context())
			if err != nil {
				return onAirError(err)
			}
			mode, source := "", ""
			if rl := onAir.RequestLine; rl != nil {
				mode, source = rl.Mode, rl.Source
			}
			return app.render.EmitRaw(raw, []string{"SHOW", "MODE", "SOURCE"},
				[][]string{{strconv.Itoa(onAir.ShowID), mode, source}})
		},
	}
}

func modeFor(arg string) *requestLineMode {
	for i := range requestLineModes {
		if requestLineModes[i].arg == arg {
			return &requestLineModes[i]
		}
	}
	return nil
}

// onAirError words FlowsheetOnAir's sentinels as one sentence.
func onAirError(err error) error {
	switch {
	case errors.Is(err, api.ErrNoOpenShow):
		return &oneLineError{msg: "No show is on the air; start one with `wxyc flowsheet start`.", err: err}
	case errors.Is(err, api.ErrOnAirUnknown):
		return &oneLineError{msg: "The server could not say which show is on the air; try again.", err: err}
	}
	return err
}

// showEditError turns PATCH /flowsheet/shows/{id}'s mayEditShow refusals into
// one line, keeping the *api.StatusError in the chain for mapExit.
func showEditError(err error) error {
	se, body, ok := refusal(err)
	if !ok {
		return err
	}
	switch {
	case se.Code == http.StatusForbidden && body.Code == "closed_show":
		return &oneLineError{msg: "this show has ended; only a music director or station manager can change it", err: err}
	case se.Code == http.StatusBadRequest && strings.Contains(body.Message, "not a member"):
		return &oneLineError{msg: "you are not on the open show; run `wxyc flowsheet start` to join it", err: err}
	case se.Code == http.StatusBadRequest && body.Message != "":
		return &oneLineError{msg: body.Message, err: err}
	}
	return err
}
