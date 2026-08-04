package cmd

import (
	"fmt"
	"strconv"

	"github.com/WXYC/wxyc-cli/internal/api"
	"github.com/spf13/cobra"
)

func newBinCmd(app *App) *cobra.Command {
	bin := &cobra.Command{Use: "bin", Short: "The DJ mail bin"}
	bin.AddCommand(newBinListCmd(app), newBinAddCmd(app))
	return bin
}

func newBinListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List albums in your bin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, raw, err := app.client.Bin(cmd.Context())
			if err != nil {
				return err
			}
			return app.render.EmitRaw(raw, []string{"ALBUM_ID", "ARTIST", "ALBUM"}, binRows(items))
		},
	}
}

func newBinAddCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "add <album_id>",
		Short: "Add an album to your bin",
		Args:  cobra.ExactArgs(1),
		// Mutating: the root PersistentPreRunE blocks this unless writes are unlocked.
		Annotations: map[string]string{annMutates: "true", annOp: "bin:add"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("album_id must be an integer: %q", args[0])
			}
			if err := app.client.BinAdd(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(app.stdout, "added album %d to bin\n", id)
			return nil
		},
	}
}

func binRows(items []api.BinItem) [][]string {
	rows := make([][]string, 0, len(items))
	for _, i := range items {
		rows = append(rows, []string{strconv.Itoa(i.AlbumID), i.ArtistName, i.AlbumTitle})
	}
	return rows
}
