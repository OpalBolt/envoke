package secrets

import (
	"os"
	"reflect"
	"testing"
)

func TestSaveAndLoadVarNames(t *testing.T) {
	uid := "test-vars-roundtrip"
	names := []string{"FOO", "BAR", "BAZ"}

	if err := SaveVarNames(uid, names); err != nil {
		t.Fatalf("SaveVarNames: %v", err)
	}
	t.Cleanup(func() { _ = ClearVarNames(uid) })

	got, err := LoadVarNames(uid)
	if err != nil {
		t.Fatalf("LoadVarNames: %v", err)
	}
	if !reflect.DeepEqual(got, names) {
		t.Errorf("LoadVarNames: got %v, want %v", got, names)
	}
}

func TestSaveVarNames_Empty(t *testing.T) {
	uid := "test-vars-empty"
	if err := SaveVarNames(uid, nil); err != nil {
		t.Fatalf("SaveVarNames(nil): %v", err)
	}
	t.Cleanup(func() { _ = ClearVarNames(uid) })

	got, err := LoadVarNames(uid)
	if err != nil {
		t.Fatalf("LoadVarNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no names, got %v", got)
	}
}

func TestLoadVarNames_NoFile(t *testing.T) {
	uid := "test-vars-nonexistent-uid-xyz"
	// Ensure there's no leftover file from a previous run.
	_ = ClearVarNames(uid)

	got, err := LoadVarNames(uid)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice, got %v", got)
	}
}

func TestClearVarNames(t *testing.T) {
	uid := "test-vars-clear"
	if err := SaveVarNames(uid, []string{"X", "Y"}); err != nil {
		t.Fatalf("SaveVarNames: %v", err)
	}
	if err := ClearVarNames(uid); err != nil {
		t.Fatalf("ClearVarNames: %v", err)
	}
	// After clearing, the file should not exist.
	path := varsFilePath(uid)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be removed after ClearVarNames, stat err: %v", err)
	}
}

func TestClearVarNames_NoFile(t *testing.T) {
	uid := "test-vars-clear-missing"
	// Clearing a non-existent file should not return an error.
	if err := ClearVarNames(uid); err != nil {
		t.Fatalf("ClearVarNames on missing file: %v", err)
	}
}

func TestVarsFilePath(t *testing.T) {
	path := varsFilePath("1234")
	if path == "" {
		t.Fatal("varsFilePath returned empty string")
	}
	// Should reside in /dev/shm or /tmp.
	if path[:8] != "/dev/shm" && path[:4] != "/tmp" {
		t.Errorf("unexpected base dir in path %q", path)
	}
	// Should contain the uid.
	const uid = "1234"
	if len(path) < len(uid) {
		t.Errorf("path %q does not contain uid %q", path, uid)
	}
}
