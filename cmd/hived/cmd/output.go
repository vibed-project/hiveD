package cmd

import (
	"fmt"
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
