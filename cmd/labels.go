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

func newLabelsShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a label with its reference counts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("id must be an integer: %q", args[0])
			}
			label, raw, err := app.client.Label(cmd.Context(), id)
			if err != nil {
				return err
			}
			parent := "-"
			if label.ParentLabelID != nil {
				parent = strconv.Itoa(*label.ParentLabelID)
			}
			row := []string{
				strconv.Itoa(label.ID),
				label.LabelName,
				parent,
				strconv.Itoa(label.References.Library),
				strconv.Itoa(label.References.Rotation),
				strconv.Itoa(label.References.Flowsheet),
				strconv.Itoa(label.References.IntakeItems),
				strconv.Itoa(label.ChildLabels),
			}
			return app.render.EmitRaw(raw,
				[]string{"ID", "LABEL", "PARENT", "LIBRARY", "ROTATION", "FLOWSHEET", "INTAKE", "CHILDREN"},
				[][]string{row})
		},
	}
}

func labelRows(labels []api.Label) [][]string {
	rows := make([][]string, 0, len(labels))
	for _, l := range labels {
		rows = append(rows, []string{strconv.Itoa(l.ID), l.LabelName})
	}
	return rows
}
