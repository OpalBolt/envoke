package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	content := `
# This is a comment
KEY1=value1
KEY2="quoted value"
KEY3='single quoted'
export KEY4=exported
KEY5=bw://folder/item
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing test .env: %v", err)
	}

	entries, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("parseDotEnv: %v", err)
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	tests := []struct{ key, val string }{
		{"KEY1", "value1"},
		{"KEY2", "quoted value"},
		{"KEY3", "single quoted"},
		{"KEY4", "exported"},
		{"KEY5", "bw://folder/item"},
	}
	for i, tt := range tests {
		if entries[i].Key != tt.key {
			t.Errorf("[%d] key: got %q, want %q", i, entries[i].Key, tt.key)
		}
		if entries[i].Value != tt.val {
			t.Errorf("[%d] value: got %q, want %q", i, entries[i].Value, tt.val)
		}
	}
}
