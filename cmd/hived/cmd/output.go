package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

// printObject renders a single proto message per --output (table, json, or
// yaml). Table output for a single object is a narrow key/value dump;
// printTable below is used instead for List results, which is what --output
// table is really for.
func printObject(m proto.Message) error {
	switch flags.output {
	case "json":
		b, err := protojson.MarshalOptions{Indent: "  "}.Marshal(m)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	case "yaml", "table":
		b, err := protojson.Marshal(m)
		if err != nil {
			return err
		}
		y, err := yaml.JSONToYAML(b)
		if err != nil {
			return err
		}
		fmt.Print(string(y))
		return nil
	default:
		return fmt.Errorf("unknown --output %q (want table, json, or yaml)", flags.output)
	}
}

// printObjects renders a list of messages per --output. Each item used to be
// passed to printObject individually, which produced neither valid JSON (a
// bare sequence of objects) nor valid YAML (documents concatenated with no
// separator, so a parser sees duplicate top-level keys and keeps only the
// last). Either way every item but one was lost on the way to `yq`/`jq`.
func printObjects(items []proto.Message) error {
	return printObjectsTo(os.Stdout, items)
}

func printObjectsTo(w io.Writer, items []proto.Message) error {
	switch flags.output {
	case "json":
		parts := make([]json.RawMessage, 0, len(items))
		for _, m := range items {
			b, err := protojson.Marshal(m)
			if err != nil {
				return err
			}
			parts = append(parts, b)
		}
		b, err := json.MarshalIndent(parts, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	case "yaml":
		for _, m := range items {
			b, err := protojson.Marshal(m)
			if err != nil {
				return err
			}
			y, err := yaml.JSONToYAML(b)
			if err != nil {
				return err
			}
			// A leading --- on every document is valid YAML and keeps the
			// stream unambiguous even for a single item.
			if _, err := fmt.Fprintf(w, "---\n%s", string(y)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown --output %q (want table, json, or yaml)", flags.output)
	}
}

// validOutputs is checked up front by the root command. Validation used to
// live in printObject, which an empty list never reaches, so a bogus
// --output silently produced no output and exit 0.
var validOutputs = []string{"table", "json", "yaml"}

func validateOutput() error {
	for _, v := range validOutputs {
		if flags.output == v {
			return nil
		}
	}
	return fmt.Errorf("unknown --output %q (want %s)", flags.output, strings.Join(validOutputs, ", "))
}

// printTable writes a simple tab-aligned table. It's used by `get` (list
// form) and `watch`; single-object `get`/`apply` output goes through
// printObject instead.
func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
}
