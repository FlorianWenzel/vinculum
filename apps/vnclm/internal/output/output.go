package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// Format is one of: table, wide, json, yaml.
type Format string

const (
	Table Format = "table"
	Wide  Format = "wide"
	JSON  Format = "json"
	YAML  Format = "yaml"
)

func Parse(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table":
		return Table, nil
	case "wide":
		return Wide, nil
	case "json":
		return JSON, nil
	case "yaml", "yml":
		return YAML, nil
	}
	return "", fmt.Errorf("unknown output format %q (want table|wide|json|yaml)", s)
}

// WriteJSON marshals v to w.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WriteYAML marshals v to w.
func WriteYAML(w io.Writer, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// Row is a table row (cells).
type Row []string

// Styler stylizes cell (row=-1 for headers) and returns ANSI-wrapped text.
// Return "" to emit cell unchanged.
type Styler func(row, col int, cell string) string

// WriteTable writes headers + rows aligned using lipgloss width (ANSI-aware).
// Pass nil styler for uncolored output.
func WriteTable(w io.Writer, headers []string, rows []Row, styler Styler) error {
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i := 0; i < cols && i < len(r); i++ {
			if wv := lipgloss.Width(r[i]); wv > widths[i] {
				widths[i] = wv
			}
		}
	}

	// Header row.
	for i, h := range headers {
		cell := h
		if styler != nil {
			if out := styler(-1, i, h); out != "" {
				cell = out
			}
		}
		if _, err := fmt.Fprint(w, pad(cell, widths[i])); err != nil {
			return err
		}
		if i < cols-1 {
			if _, err := fmt.Fprint(w, "  "); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Body rows.
	for ri, r := range rows {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			if styler != nil {
				if out := styler(ri, i, cell); out != "" {
					cell = out
				}
			}
			if _, err := fmt.Fprint(w, pad(cell, widths[i])); err != nil {
				return err
			}
			if i < cols-1 {
				if _, err := fmt.Fprint(w, "  "); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// pad right-pads s to visible width n.
func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
