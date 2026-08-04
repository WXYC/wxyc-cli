package cmd

import (
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newLabelsCmd(app *App) *cobra.Command {
	labels := &cobra.Command{Use: "labels", Short: "Query record labels"}
	labels.AddCommand(newLabelsListCmd(app), newLabelsSearchCmd(app))
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
