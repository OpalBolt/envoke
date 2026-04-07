package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
)

func TestResolveYAMLString_NoRefs(t *testing.T) {
	input := `
database:
  host: localhost
  port: 5432
`
	result, err := ResolveYAMLString(input, secrets.NewRegistry())
	if err != nil {
		t.Fatalf("ResolveYAMLString: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	db, _ := m["database"].(map[string]interface{})
	if db["host"] != "localhost" {
		t.Errorf("host: got %v", db["host"])
	}
}

func TestYAMLLookup(t *testing.T) {
	input := `
database:
  host: localhost
  port: 5432
list:
  - item0
  - item1
`
	doc, err := ResolveYAMLString(input, secrets.NewRegistry())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"database.host", "localhost"},
		{"list.1", "item1"},
	}
	for _, tt := range tests {
		got, err := YAMLLookup(doc, tt.key)
		if err != nil {
			t.Errorf("YAMLLookup(%q): %v", tt.key, err)
			continue
		}
		if got != tt.want {
			t.Errorf("YAMLLookup(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestYAMLLookup_Errors(t *testing.T) {
	input := `
map:
  key: value
list:
  - a
  - b
`
	doc, err := ResolveYAMLString(input, secrets.NewRegistry())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	errCases := []struct {
		name string
		key  string
	}{
		{"missing map key", "map.missing"},
		{"list non-integer index", "list.notanint"},
		{"list index out of range", "list.99"},
		{"traverse scalar", "map.key.deeper"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := YAMLLookup(doc, tc.key)
			if err == nil {
				t.Errorf("YAMLLookup(%q): expected error, got nil", tc.key)
			}
		})
	}
}

func TestResolveYAML_File(t *testing.T) {
	content := `
app:
  name: testapp
  debug: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing YAML file: %v", err)
	}

	result, err := ResolveYAML(path, secrets.NewRegistry())
	if err != nil {
		t.Fatalf("ResolveYAML: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	app, _ := m["app"].(map[string]interface{})
	if app["name"] != "testapp" {
		t.Errorf("app.name: got %v, want testapp", app["name"])
	}
}

func TestResolveYAML_NonExistentFile(t *testing.T) {
	_, err := ResolveYAML("/nonexistent/config.yaml", secrets.NewRegistry())
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestMarshalYAML(t *testing.T) {
	input := map[string]interface{}{
		"key": "value",
	}
	out, err := MarshalYAML(input)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if len(out) == 0 {
		t.Error("MarshalYAML returned empty output")
	}
	// Round-trip: unmarshal and verify.
	doc, err := ResolveYAMLString(string(out), secrets.NewRegistry())
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	got, err := YAMLLookup(doc, "key")
	if err != nil {
		t.Fatalf("YAMLLookup after round-trip: %v", err)
	}
	if got != "value" {
		t.Errorf("round-trip: got %q, want value", got)
	}
}

func TestResolveYAMLString_InvalidYAML(t *testing.T) {
	_, err := ResolveYAMLString(":\tinvalid: [", secrets.NewRegistry())
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
