package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Table prints rows in an aligned table format.
//
// Widths are computed with VisibleLen rather than len, and padding is applied
// here rather than by text/tabwriter. tabwriter measures cells in bytes, so a
// single coloured cell would count its escape sequences as visible characters
// and push every later column out of alignment.
func Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = VisibleLen(h)
	}
	for _, row := range rows {
		for i, col := range row {
			if i < len(widths) {
				if n := VisibleLen(col); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	line := func(cells []string) {
		var b strings.Builder
		for i, c := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			// The final column is not padded, so lines carry no trailing
			// whitespace when copied out of a terminal.
			if i == len(cells)-1 {
				b.WriteString(c)
			} else {
				b.WriteString(Pad(c, widths[i]))
			}
		}
		fmt.Fprintln(os.Stdout, strings.TrimRight(b.String(), " "))
	}

	line(headers)
	sep := make([]string, len(headers))
	for i := range headers {
		sep[i] = Dim(strings.Repeat("-", widths[i]))
	}
	line(sep)
	for _, row := range rows {
		line(row)
	}
}

// JSON prints data as formatted JSON to stdout.
func JSON(data interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
