package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestReadDocs_MultiDocumentSeparators(t *testing.T) {
	// The regexp splitter this replaced only matched a line of exactly
	// "---", so every one of these separators silently dropped all
	// documents after the first while apply still exited 0.
	tests := []struct {
		name      string
		separator string
	}{
		{"bare", "---"},
		{"trailing comment", "--- # second doc"},
		{"trailing spaces", "---   "},
		{"tab before comment", "---\t# second doc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: alpha\nspec: {}\n" +
				tc.separator + "\n" +
				"apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: beta\nspec: {}\n" +
				tc.separator + "\n" +
				"apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: gamma\nspec: {}\n"
			path := writeFile(t, dir, "m.yaml", body)

			docs, err := readDocs(path)
			if err != nil {
				t.Fatalf("readDocs: %v", err)
			}
			if len(docs) != 3 {
				t.Fatalf("parsed %d documents, want 3", len(docs))
			}
			for i, want := range []string{"alpha", "beta", "gamma"} {
				if docs[i].Name != want {
					t.Errorf("document %d name = %q, want %q", i, docs[i].Name, want)
				}
			}
		})
	}
}

func TestReadDocs_InvalidYAMLFailsLoudly(t *testing.T) {
	// "---#comment" is not a document separator (the directives-end marker
	// must be followed by whitespace), so this is malformed YAML. The point
	// is that it errors rather than silently parsing as something else.
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml",
		"apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: alpha\nspec: {}\n"+
			"---# not a separator\n"+
			"apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: beta\nspec: {}\n")

	if _, err := readDocs(path); err == nil {
		t.Fatal("readDocs silently accepted malformed YAML")
	}
}

func TestReadDocs_SkipsEmptyAndCommentOnlyDocuments(t *testing.T) {
	// A trailing "--- # TODO" used to abort the entire apply with
	// "missing required kind field" and apply nothing at all.
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml", `---
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: alpha
spec: {}
---
# TODO: add the agent here
---

---
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: beta
spec: {}
`)

	docs, err := readDocs(path)
	if err != nil {
		t.Fatalf("readDocs: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("parsed %d documents, want 2 (empty and comment-only skipped)", len(docs))
	}
	if docs[0].Name != "alpha" || docs[1].Name != "beta" {
		t.Fatalf("got %q, %q; want alpha, beta", docs[0].Name, docs[1].Name)
	}
}

func TestReadDocs_DocumentEndMarker(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml", `apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: alpha
spec: {}
...
---
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: beta
spec: {}
`)
	docs, err := readDocs(path)
	if err != nil {
		t.Fatalf("readDocs: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("parsed %d documents, want 2", len(docs))
	}
}

func TestReadDocs_APIVersionValidated(t *testing.T) {
	// apiVersion was parsed into the manifest struct and then never read, so
	// a future v1beta1 document was silently decoded as v1alpha1.
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{"unsupported", "hived/v9beta1", "unsupported apiVersion"},
		{"foreign", "apps/v1", "unsupported apiVersion"},
		{"missing", "", "missing required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "kind: Colony\nmetadata:\n  name: alpha\nspec: {}\n"
			if tc.version != "" {
				body = "apiVersion: " + tc.version + "\n" + body
			}
			path := writeFile(t, dir, "m.yaml", body)

			_, err := readDocs(path)
			if err == nil {
				t.Fatalf("readDocs accepted apiVersion %q", tc.version)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestReadDocs_MissingKind(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml", "apiVersion: hived/v1alpha1\nmetadata:\n  name: alpha\n")
	if _, err := readDocs(path); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("error = %v, want it to mention the missing kind", err)
	}
}

func TestReadDocs_RejectsNonMapping(t *testing.T) {
	// A JSON array of manifests is a common mistake; the old message leaked
	// the raw anonymous Go struct type at the user.
	dir := t.TempDir()
	for _, body := range []string{"[1, 2, 3]\n", "just a scalar\n"} {
		path := writeFile(t, dir, "m.yaml", body)
		_, err := readDocs(path)
		if err == nil {
			t.Fatalf("readDocs accepted %q", body)
		}
		if strings.Contains(err.Error(), "struct {") {
			t.Fatalf("error leaks a Go type: %v", err)
		}
	}
}

func TestManifestString_DoesNotLeakBody(t *testing.T) {
	// apply errors were formatted with the whole document JSON, which puts
	// agent instructions and tool config into CI logs.
	m := manifest{
		Kind: "AgentVersion", Name: "greeter-v1", Colony: "acme",
		JSON:   []byte(`{"spec":{"instructions":"SUPER SECRET"}}`),
		Source: "agents.yaml", Index: 2,
	}
	got := m.String()
	if strings.Contains(got, "SUPER SECRET") {
		t.Fatalf("manifest.String() leaked the body: %s", got)
	}
	for _, want := range []string{"AgentVersion", "acme/greeter-v1", "agents.yaml", "document 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest.String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestReadManifests_DirectoryOrderIsFilenameSorted(t *testing.T) {
	// Glob ran per extension, so every .json applied after every .yaml
	// regardless of filename -- surprising when ordering matters.
	dir := t.TempDir()
	writeFile(t, dir, "10-colony.yaml", "apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: c10\nspec: {}\n")
	writeFile(t, dir, "20-agent.json", `{"apiVersion":"hived/v1alpha1","kind":"Colony","metadata":{"name":"c20"},"spec":{}}`)
	writeFile(t, dir, "30-tool.yml", "apiVersion: hived/v1alpha1\nkind: Colony\nmetadata:\n  name: c30\nspec: {}\n")

	docs, err := readManifests(dir)
	if err != nil {
		t.Fatalf("readManifests: %v", err)
	}
	var got []string
	for _, d := range docs {
		got = append(got, d.Name)
	}
	want := []string{"c10", "c20", "c30"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("apply order = %v, want %v", got, want)
	}
}

func TestReadDocs_StripsEnvelopeSoStrictUnmarshalWorks(t *testing.T) {
	// apply now unmarshals strictly, so apiVersion/kind must not reach the
	// proto message -- they are routing keys, not object fields.
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml", `apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: alpha
spec:
  displayName: Alpha
`)
	docs, err := readDocs(path)
	if err != nil {
		t.Fatalf("readDocs: %v", err)
	}
	for _, key := range []string{"apiVersion", "kind"} {
		if strings.Contains(string(docs[0].JSON), key) {
			t.Errorf("body still contains routing key %q: %s", key, docs[0].JSON)
		}
	}

	obj := &v1alpha1.Colony{}
	if err := unmarshalOpts.Unmarshal(docs[0].JSON, obj); err != nil {
		t.Fatalf("strict unmarshal of a valid manifest failed: %v", err)
	}
	if obj.GetSpec().GetDisplayName() != "Alpha" {
		t.Fatalf("displayName = %q, want Alpha", obj.GetSpec().GetDisplayName())
	}
}

func TestApply_RejectsTypoedField(t *testing.T) {
	// `dispalyName` used to be silently discarded, creating a Colony with an
	// empty spec and exiting 0.
	dir := t.TempDir()
	path := writeFile(t, dir, "m.yaml", `apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: alpha
spec:
  dispalyName: Alpha
`)
	docs, err := readDocs(path)
	if err != nil {
		t.Fatalf("readDocs: %v", err)
	}
	err = unmarshalOpts.Unmarshal(docs[0].JSON, &v1alpha1.Colony{})
	if err == nil {
		t.Fatal("typo'd field was silently accepted")
	}
	if !strings.Contains(err.Error(), "dispalyName") {
		t.Fatalf("error = %v, want it to name the offending field", err)
	}
}
