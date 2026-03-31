package env

import (
	"strings"
	"testing"
)

func TestEmitExports(t *testing.T) {
	entries := []EnvEntry{
		{Key: "FOO", Value: "bar"},
		{Key: "BAZ", Value: "it's a value"},
	}
	var sb strings.Builder
	if err := EmitExports(&sb, entries); err != nil {
		t.Fatalf("EmitExports: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "export FOO='bar'") {
		t.Errorf("missing FOO export, got:\n%s", out)
	}
	// Single quote with apostrophe should be escaped
	if !strings.Contains(out, "BAZ=") {
		t.Errorf("missing BAZ export")
	}
}

func TestEmitUnload(t *testing.T) {
	entries := []EnvEntry{
		{Key: "FOO", Value: "bar"},
		{Key: "BAZ", Value: "qux"},
	}
	var sb strings.Builder
	if err := EmitUnload(&sb, entries); err != nil {
		t.Fatalf("EmitUnload: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "unset FOO") {
		t.Errorf("missing unset FOO")
	}
	if !strings.Contains(out, "unset BAZ") {
		t.Errorf("missing unset BAZ")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"it's", "'it'\"'\"'s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
