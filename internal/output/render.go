// Package output renders command results either as indented JSON (for agents
// and scripts, selected with --json) or an aligned text table (for humans).
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// Renderer emits a result in the configured format. Commands supply both a
// structured value (used for JSON) and a headers/rows projection (used for the
// table), so each view can be shaped independently.
type Renderer struct {
	JSON bool
	Out  io.Writer
}

// Emit writes the result. In JSON mode it encodes jsonValue; otherwise it draws
// a table from headers and rows.
func (r Renderer) Emit(jsonValue any, headers []string, rows [][]string) error {
	if r.JSON {
		enc := json.NewEncoder(r.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonValue)
	}
	return r.table(headers, rows)
}

// EmitRaw is like Emit but, in JSON mode, passes the server's raw JSON through
// verbatim (pretty-printed) so agents receive every field even when the table
// projection shows only a subset. In table mode it behaves exactly like Emit.
func (r Renderer) EmitRaw(raw []byte, headers []string, rows [][]string) error {
	if !r.JSON {
		return r.table(headers, rows)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// Not indentable (unexpected content type) — emit as received.
		_, err := r.Out.Write(raw)
		return err
	}
	buf.WriteByte('\n')
	_, err := r.Out.Write(buf.Bytes())
	return err
}

func (r Renderer) table(headers []string, rows [][]string) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(r.Out, "no results")
		return err
	}
	tw := tabwriter.NewWriter(r.Out, 0, 2, 2, ' ', 0)
	writeRow(tw, headers)
	for _, row := range rows {
		writeRow(tw, row)
	}
	return tw.Flush()
}

func writeRow(w io.Writer, cells []string) {
	for i, c := range cells {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, truncate(c))
	}
	fmt.Fprintln(w)
}

// maxCellWidth caps a table cell's display width. tabwriter pads every cell in
// a column to the widest one, so a single long value (e.g. a run-on artist
// name) would otherwise bloat the whole table with trailing whitespace. Only
// the table view truncates; --json is always full-fidelity.
const maxCellWidth = 48

// truncate shortens s to maxCellWidth runes, marking the cut with an ellipsis.
func truncate(s string) string {
	r := []rune(s)
	if len(r) <= maxCellWidth {
		return s
	}
	return string(r[:maxCellWidth-1]) + "…"
}

// Table is one headers/rows projection for EmitRawTables. Empty is printed in
// place of a table with no rows (default "no results").
type Table struct {
	Headers []string
	Rows    [][]string
	Empty   string
}

// EmitRawTables is EmitRaw for a result shown as several tables (e.g. a
// FIELD/VALUE card followed by a list). In JSON mode it passes raw through
// exactly as EmitRaw does; in table mode it draws each table with its own
// column alignment, separated by a blank line.
func (r Renderer) EmitRawTables(raw []byte, tables ...Table) error {
	if r.JSON {
		return r.EmitRaw(raw, nil, nil)
	}
	for i, t := range tables {
		if i > 0 {
			if _, err := fmt.Fprintln(r.Out); err != nil {
				return err
			}
		}
		if len(t.Rows) == 0 && t.Empty != "" {
			if _, err := fmt.Fprintln(r.Out, t.Empty); err != nil {
				return err
			}
			continue
		}
		if err := r.table(t.Headers, t.Rows); err != nil {
			return err
		}
	}
	return nil
}
