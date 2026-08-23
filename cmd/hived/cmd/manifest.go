package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	goyaml "go.yaml.in/yaml/v2"
	"sigs.k8s.io/yaml"
)

// manifest is one parsed apply -f document: apiVersion/kind identify which
// service to route it to; json is the remaining metadata/spec/status body,
// ready for protojson.Unmarshal into the matching object type. Source and
// Index locate the document for error messages.
type manifest struct {
	APIVersion string
	Kind       string
	Name       string
	Colony     string
	JSON       []byte

	Source string // file the document came from, or "-" for stdin
	Index  int    // 0-based position within that file
}

// String identifies a document in diagnostics. It deliberately never includes
// the document body: manifests carry agent instructions and tool config, and
// apply errors end up in CI logs.
func (m manifest) String() string {
	id := m.Kind
	switch {
	case m.Colony != "" && m.Name != "":
		id += " " + m.Colony + "/" + m.Name
	case m.Name != "":
		id += " " + m.Name
	}
	return fmt.Sprintf("%s (%s document %d)", id, m.Source, m.Index+1)
}

// SupportedAPIVersion is the only apiVersion apply accepts. Without this
// check the field was parsed and then never read, so a future v1beta1
// manifest would be silently reinterpreted as v1alpha1.
const SupportedAPIVersion = "hived/v1alpha1"

// readManifests resolves -f FILE|DIR|- into a flat list of manifests,
// supporting multi-document YAML files.
func readManifests(path string) ([]manifest, error) {
	var files []string
	switch path {
	case "-":
		files = []string{"-"}
	default:
		info, err := os.Stat(path)
		if err != nil {
			return nil, err // *os.PathError already reads "stat <path>: ..."
		}
		if info.IsDir() {
			seen := map[string]bool{}
			for _, ext := range []string{"*.yaml", "*.yml", "*.json"} {
				matches, err := filepath.Glob(filepath.Join(path, ext))
				if err != nil {
					return nil, err
				}
				for _, m := range matches {
					seen[m] = true
				}
			}
			for f := range seen {
				files = append(files, f)
			}
			// Glob groups by extension, so without this every .json manifest
			// applied after every .yaml one regardless of filename. Apply in
			// a predictable order instead.
			sort.Strings(files)
		} else {
			files = []string{path}
		}
	}

	var out []manifest
	for _, f := range files {
		docs, err := readDocs(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, docs...)
	}
	return out, nil
}

func readDocs(file string) ([]manifest, error) {
	var raw []byte
	var err error
	if file == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(file) //nolint:gosec // G304: file is the user's own -f argument, not attacker-controlled input
	}
	if err != nil {
		return nil, err
	}

	// Split with a real YAML decoder rather than a regexp. `--- # comment`
	// and `...` are both valid document markers that a `^---\s*$` pattern
	// does not match, so every document after the first was silently dropped
	// and apply still exited 0.
	dec := goyaml.NewDecoder(strings.NewReader(string(raw)))
	var docs []manifest
	for i := 0; ; i++ {
		var doc interface{}
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("document %d: parse yaml: %w", i+1, err)
		}
		// An empty or comment-only document decodes to nil. Skip it the way
		// kubectl does rather than failing the whole apply.
		if doc == nil {
			continue
		}

		norm, err := goyaml.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", i+1, err)
		}
		j, err := yaml.YAMLToJSON(norm)
		if err != nil {
			return nil, fmt.Errorf("document %d: parse yaml: %w", i+1, err)
		}

		var envelope struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name   string `json:"name"`
				Colony string `json:"colony"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(j, &envelope); err != nil {
			// The raw Go type in a json error is noise for a CLI user; the
			// common cause is a top-level list or scalar.
			return nil, fmt.Errorf("document %d: manifest must be a mapping with apiVersion, kind and metadata", i+1)
		}
		if envelope.Kind == "" {
			return nil, fmt.Errorf("document %d: manifest is missing required %q field", i+1, "kind")
		}
		if envelope.APIVersion == "" {
			return nil, fmt.Errorf("document %d: manifest is missing required %q field (want %q)", i+1, "apiVersion", SupportedAPIVersion)
		}
		if envelope.APIVersion != SupportedAPIVersion {
			return nil, fmt.Errorf("document %d: unsupported apiVersion %q (want %q)", i+1, envelope.APIVersion, SupportedAPIVersion)
		}

		// apiVersion and kind route the document; they are not fields on the
		// object itself. Strip them so the body can be unmarshalled strictly
		// -- DiscardUnknown used to hide them, and hid typos with them.
		body, err := stripEnvelopeFields(j)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", i+1, err)
		}

		docs = append(docs, manifest{
			APIVersion: envelope.APIVersion,
			Kind:       envelope.Kind,
			Name:       envelope.Metadata.Name,
			Colony:     envelope.Metadata.Colony,
			JSON:       body,
			Source:     file,
			Index:      len(docs),
		})
	}
	return docs, nil
}

// stripEnvelopeFields removes the routing keys from a manifest body, leaving
// metadata/spec/status for protojson. Everything else is preserved verbatim
// so unknown-field errors still point at the user's real typo.
func stripEnvelopeFields(doc []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return nil, err
	}
	delete(fields, "apiVersion")
	delete(fields, "kind")
	return json.Marshal(fields)
}
