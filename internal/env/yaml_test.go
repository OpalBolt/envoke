package env

import (
	"testing"
)

func TestResolveYAMLString_NoRefs(t *testing.T) {
	input := `
database:
  host: localhost
  port: 5432
`
	result, err := ResolveYAMLString(input, nil, nil)
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
	doc, err := ResolveYAMLString(input, nil, nil)
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
