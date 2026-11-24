// Package output renders command results either as indented JSON (for agents
// and scripts, selected with --json) or an aligned text table (for humans).
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"
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
	if len(bytes.TrimSpace(raw)) == 0 {
		// An empty 2xx body (e.g. 204 No Content): pass through as nothing.
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// Truncated or non-JSON: fail rather than hand a script partial
		// output with exit 0.
		return fmt.Errorf("response is not valid JSON: %w", err)
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
		fmt.Fprint(w, truncate(sanitize(c)))
	}
	fmt.Fprintln(w)
}

// sanitize makes a cell safe for the table: tabs and line breaks (which would
// forge columns and rows in the tabwriter), including U+2028/U+2029, become
// spaces; invisible format characters (Unicode Cf: bidi overrides and
// isolates, zero-width spaces, BOM), which can reorder how a row displays, are
// dropped; and any other control character (e.g. ESC starting a terminal
// escape sequence) becomes U+FFFD. ZWNJ and ZWJ are kept: they are ordinary
// text in Persian and Indic names and in emoji, and cannot reorder anything.
// Cell text is partly DJ-entered, so it is untrusted. --json is never
// sanitized.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		if r == '\u200C' || r == '\u200D' {
			return r
		}
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		if unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, s)
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
// place of a table with no rows (default "no results"). A Title, when set, is
// printed as "Title:" on the line above.
type Table struct {
	Title   string
	Headers []string
	Rows    [][]string
	Empty   string
}

// EmitRawTables is EmitRaw for a result shown as several tables (e.g. a
// FIELD/VALUE card followed by a list). In JSON mode it passes raw through
// exactly as EmitRaw does; in table mode it draws each table with its own
// column alignment, separated by a blank line.
func (r Renderer) EmitRawTables(raw []byte, tables ...Table) error {
	blocks := make([]Block, len(tables))
	for i, t := range tables {
		blocks[i] = t
	}
	return r.EmitRawBlocks(raw, blocks...)
}

// Block is one part of a table-mode result drawn by EmitRawBlocks: a Table
// or a Text.
type Block interface {
	write(r Renderer) error
}

// Text is prose printed whole under "Heading:", for DJ-written text too long
// for a table cell (e.g. a review). Its line breaks are kept and every line
// is indented two spaces; like a cell, it is otherwise sanitized.
type Text struct {
	Heading string
	Body    string
}

func (t Text) write(r Renderer) error {
	var b strings.Builder
	b.WriteString(sanitize(t.Heading) + ":\n")
	for _, line := range strings.Split(proseLines(t.Body), "\n") {
		if line = strings.TrimRight(line, " \t"); line != "" {
			b.WriteString("  " + line)
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(r.Out, b.String())
	return err
}

// proseLines normalizes every line break (\r\n, \r, U+2028, U+2029) to \n
// and sanitizes the rest as a cell would, except that tabs are kept: outside
// a tabwriter they can't forge a column.
func proseLines(s string) string {
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u2028", "\n", "\u2029", "\n").Replace(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		parts := strings.Split(l, "\t")
		for j, p := range parts {
			parts[j] = sanitize(p)
		}
		lines[i] = strings.Join(parts, "\t")
	}
	return strings.Join(lines, "\n")
}

func (t Table) write(r Renderer) error {
	if t.Title != "" {
		if _, err := fmt.Fprintln(r.Out, sanitize(t.Title)+":"); err != nil {
			return err
		}
	}
	if len(t.Rows) == 0 && t.Empty != "" {
		_, err := fmt.Fprintln(r.Out, t.Empty)
		return err
	}
	return r.table(t.Headers, t.Rows)
}

// Note is a single line of text, e.g. a page position under a list.
type Note string

func (n Note) write(r Renderer) error {
	_, err := fmt.Fprintln(r.Out, sanitize(string(n)))
	return err
}

// EmitRawBlocks is EmitRawTables for a result that mixes tables and prose.
// In JSON mode it passes raw through exactly as EmitRaw does; in table mode
// it draws each block in turn, separated by a blank line.
func (r Renderer) EmitRawBlocks(raw []byte, blocks ...Block) error {
	if r.JSON {
		return r.EmitRaw(raw, nil, nil)
	}
	for i, b := range blocks {
		if i > 0 {
			if _, err := fmt.Fprintln(r.Out); err != nil {
				return err
			}
		}
		if err := b.write(r); err != nil {
			return err
		}
	}
	return nil
}
