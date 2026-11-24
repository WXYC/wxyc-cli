package cmd

import (
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newLabelsCmd(app *App) *cobra.Command {
	labels := &cobra.Command{Use: "labels", Short: "Query record labels"}
	labels.AddCommand(newLabelsListCmd(app), newLabelsSearchCmd(app), newLabelsShowCmd(app))
	return labels
}

func newLabelsListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all record labels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			labels, raw, err := app.client.Labels(cmd.Context())
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"ID", "LABEL"}, labelRows(labels))
		},
	}
}

func newLabelsSearchCmd(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search record labels by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, raw, err := app.client.LabelSearch(cmd.Context(), args[0], limit)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"ID", "LABEL"}, labelRows(labels))
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "max results")
	return cmd
}

func labelRows(labels []api.Label) [][]string {
	rows := make([][]string, 0, len(labels))
	for _, l := range labels {
		rows = append(rows, []string{strconv.Itoa(l.ID), l.LabelName})
	}
	return rows
}

func newLabelsShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a label with its parent and reference counts",
		Long: `Show one record label: its name, parent label id, how many labels name it
as their parent, and how many rows reference it in the library, rotation,
flowsheet and intake. The id is the ID column of "wxyc labels list" or
"wxyc labels search".

--json prints the server's GET /labels/{id} body verbatim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("label id must be an integer, got %q", args[0])
			}
			label, raw, err := app.client.Label(cmd.Context(), id)
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"FIELD", "VALUE"}, labelShowRows(label))
		},
	}
}

// labelShowRows is the FIELD/VALUE table. A label with no parent gets no
// parent row.
func labelShowRows(l api.LabelWithReferences) [][]string {
	rows := [][]string{
		{"id", strconv.Itoa(l.ID)},
		{"label", l.LabelName},
	}
	if l.ParentLabelID != nil {
		rows = append(rows, []string{"parent id", strconv.Itoa(*l.ParentLabelID)})
	}
	return append(rows,
		[]string{"child labels", strconv.Itoa(l.ChildLabels)},
		[]string{"references (library)", strconv.Itoa(l.References.Library)},
		[]string{"references (rotation)", strconv.Itoa(l.References.Rotation)},
		[]string{"references (flowsheet)", strconv.Itoa(l.References.Flowsheet)},
		[]string{"references (intake)", strconv.Itoa(l.References.IntakeItems)},
	)
}
