package goldentest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/japer-technology/flat-email/internal/archive"
	"github.com/japer-technology/flat-email/internal/storage"
)

// TestProducedJSONValidatesAgainstSchemas asserts that every JSON file the
// producer emits validates against the machine-readable schemas in schemas/
// (SPEC §15). This guards the on-disk contract independently of the byte-exact
// golden comparison.
func TestProducedJSONValidatesAgainstSchemas(t *testing.T) {
	repo := repoRoot(t)
	out := t.TempDir()
	if err := archive.Produce(storage.NewFS(out), goldenInput(t, filepath.Join(repo, "tests", "golden"))); err != nil {
		t.Fatalf("Produce: %v", err)
	}

	schemaDir := filepath.Join(repo, "schemas")
	schemas := map[string]*jsonschema.Schema{
		"metadata":    compileSchema(t, filepath.Join(schemaDir, "metadata.schema.json")),
		"attachments": compileSchema(t, filepath.Join(schemaDir, "attachments.schema.json")),
		"labels":      compileSchema(t, filepath.Join(schemaDir, "labels.schema.json")),
		"catalog":     compileSchema(t, filepath.Join(schemaDir, "catalog.schema.json")),
		"manifest":    compileSchema(t, filepath.Join(schemaDir, "manifest.schema.json")),
	}

	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".json") {
			return err
		}
		schema := schemaFor(filepath.ToSlash(p))
		if schema == "" {
			return nil
		}
		validateFile(t, schemas[schema], p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func schemaFor(path string) string {
	switch {
	case strings.HasSuffix(path, "/metadata.json"):
		return "metadata"
	case strings.HasSuffix(path, "/attachments.json"):
		return "attachments"
	case strings.HasSuffix(path, "/labels.json"):
		return "labels"
	case strings.HasSuffix(path, "/catalog.json"):
		return "catalog"
	case strings.HasSuffix(path, "/flat-email.json"):
		return "manifest"
	default:
		// label membership arrays and thread arrays have no dedicated schema.
		return ""
	}
}

func compileSchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	s, err := c.Compile(path)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return s
}

func validateFile(t *testing.T, schema *jsonschema.Schema, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var v any
	if err := jsonUnmarshalReader(f, &v); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if err := schema.Validate(v); err != nil {
		t.Errorf("schema validation failed for %s: %v", path, err)
	}
}
