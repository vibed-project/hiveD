package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	goyaml "go.yaml.in/yaml/v2"
	"google.golang.org/protobuf/proto"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func testColonies(names ...string) []proto.Message {
	out := make([]proto.Message, 0, len(names))
	for _, n := range names {
		out = append(out, &v1alpha1.Colony{
			Metadata: &v1alpha1.ObjectMeta{Name: n},
			Spec:     &v1alpha1.ColonySpec{DisplayName: strings.ToUpper(n)},
		})
	}
	return out
}

func TestPrintObjects_JSONIsAParseableArray(t *testing.T) {
	// Each item used to be marshalled and printed individually, so the
	// output was a bare sequence of objects: `json.Unmarshal` fails with
	// "Extra data" and `jq` sees only the first.
	prev := flags.output
	t.Cleanup(func() { flags.output = prev })
	flags.output = "json"

	var buf bytes.Buffer
	if err := printObjectsTo(&buf, testColonies("alpha", "beta", "gamma")); err != nil {
		t.Fatalf("printObjectsTo: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 3 {
		t.Fatalf("decoded %d items, want 3", len(got))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		name := got[i]["metadata"].(map[string]any)["name"]
		if name != want {
			t.Errorf("item %d name = %v, want %s", i, name, want)
		}
	}
}

func TestPrintObjects_YAMLKeepsEveryDocument(t *testing.T) {
	// Without separators the documents concatenate into one stream with
	// duplicate top-level keys, and a YAML parser keeps only the last --
	// silently dropping every other item.
	prev := flags.output
	t.Cleanup(func() { flags.output = prev })
	flags.output = "yaml"

	var buf bytes.Buffer
	if err := printObjectsTo(&buf, testColonies("alpha", "beta", "gamma")); err != nil {
		t.Fatalf("printObjectsTo: %v", err)
	}

	dec := goyaml.NewDecoder(bytes.NewReader(buf.Bytes()))
	var names []string
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc == nil {
			continue
		}
		meta, _ := doc["metadata"].(map[any]any)
		names = append(names, meta["name"].(string))
	}
	if len(names) != 3 {
		t.Fatalf("parsed %d documents, want 3 (got %v)\n%s", len(names), names, buf.String())
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if names[i] != want {
			t.Errorf("document %d name = %s, want %s", i, names[i], want)
		}
	}
}

func TestPrintObjects_SingleItemStillParses(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			prev := flags.output
			t.Cleanup(func() { flags.output = prev })
			flags.output = format

			var buf bytes.Buffer
			if err := printObjectsTo(&buf, testColonies("solo")); err != nil {
				t.Fatalf("printObjectsTo: %v", err)
			}
			if !strings.Contains(buf.String(), "solo") {
				t.Fatalf("output does not contain the item:\n%s", buf.String())
			}
		})
	}
}

func TestPrintObjects_EmptyListIsValid(t *testing.T) {
	prev := flags.output
	t.Cleanup(func() { flags.output = prev })
	flags.output = "json"

	var buf bytes.Buffer
	if err := printObjectsTo(&buf, nil); err != nil {
		t.Fatalf("printObjectsTo: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("empty list is not valid JSON: %v (%q)", err, buf.String())
	}
	if len(got) != 0 {
		t.Fatalf("decoded %d items, want 0", len(got))
	}
}

func TestValidateOutput(t *testing.T) {
	// Validation used to live in printObject, which an empty list never
	// reaches -- so `hived --output bogus get colonies` exited 0 silently.
	prev := flags.output
	t.Cleanup(func() { flags.output = prev })

	for _, ok := range []string{"table", "json", "yaml"} {
		flags.output = ok
		if err := validateOutput(); err != nil {
			t.Errorf("validateOutput(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"bogus", "", "JSON", "tables"} {
		flags.output = bad
		if err := validateOutput(); err == nil {
			t.Errorf("validateOutput(%q) = nil, want an error", bad)
		}
	}
}
