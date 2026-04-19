package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

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

// WriteTable writes headers + rows tab-aligned.
func WriteTable(w io.Writer, headers []string, rows []Row) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(r, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
