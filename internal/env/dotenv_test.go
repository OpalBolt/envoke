package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
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

func TestParseDotEnv_NonExistentFile(t *testing.T) {
	_, err := parseDotEnv("/nonexistent/.env")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestResolveDotEnv_NoRefs(t *testing.T) {
	content := `
KEY1=hello
KEY2=world
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// No secret refs — bwClient and vaultClient are never dereferenced.
	entries, err := ResolveDotEnv(path, &secrets.BWClient{}, &secrets.VaultClient{})
	if err != nil {
		t.Fatalf("ResolveDotEnv: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "KEY1" || entries[0].Value != "hello" {
		t.Errorf("entry[0]: got %q=%q, want KEY1=hello", entries[0].Key, entries[0].Value)
	}
	if entries[0].IsRef {
		t.Error("entry[0].IsRef should be false for plain value")
	}
}

func TestResolveDotEnv_NonExistentFile(t *testing.T) {
	_, err := ResolveDotEnv("/nonexistent/.env", &secrets.BWClient{}, &secrets.VaultClient{})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}
