package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// manifest is one parsed apply -f document: apiVersion/kind identify which
// service to route it to; json is the remaining metadata/spec/status body,
// ready for protojson.Unmarshal into the matching object type.
type manifest struct {
	APIVersion string
	Kind       string
	JSON       []byte
}

var yamlDocSeparator = regexp.MustCompile(`(?m)^---\s*$`)

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
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			for _, ext := range []string{"*.yaml", "*.yml", "*.json"} {
				matches, err := filepath.Glob(filepath.Join(path, ext))
				if err != nil {
					return nil, err
				}
				files = append(files, matches...)
			}
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

	var docs []manifest
	for _, chunk := range yamlDocSeparator.Split(string(raw), -1) {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		j, err := yaml.YAMLToJSON([]byte(chunk))
		if err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
		var envelope struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
		}
		if err := json.Unmarshal(j, &envelope); err != nil {
			return nil, fmt.Errorf("parse envelope: %w", err)
		}
		if envelope.Kind == "" {
			return nil, fmt.Errorf("manifest is missing required \"kind\" field")
		}
		docs = append(docs, manifest{APIVersion: envelope.APIVersion, Kind: envelope.Kind, JSON: j})
	}
	return docs, nil
}
