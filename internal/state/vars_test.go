package state

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/opalbolt/envoke/internal/securedir"
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
	if !strings.HasPrefix(path, securedir.Dir()) {
		t.Errorf("unexpected base dir in path %q (want prefix %q)", path, securedir.Dir())
	}
	const uid = "1234"
	if !strings.Contains(path, uid) {
		t.Errorf("path %q does not contain uid %q", path, uid)
	}
}

func TestUnloadRequestFile(t *testing.T) {
	path := UnloadRequestFile("9999")
	if path == "" {
		t.Fatal("UnloadRequestFile returned empty string")
	}
	if !strings.HasPrefix(path, securedir.Dir()) {
		t.Errorf("unexpected base dir in path %q (want prefix %q)", path, securedir.Dir())
	}
	const uid = "9999"
	if !strings.Contains(path, uid) {
		t.Errorf("path %q does not contain uid %q", path, uid)
	}
}

func TestRequestUnload(t *testing.T) {
	uid := "test-unload-request"
	path := UnloadRequestFile(uid)
	t.Cleanup(func() { os.Remove(path) })

	if err := RequestUnload(uid); err != nil {
		t.Fatalf("RequestUnload: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("sentinel file not created: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", fi.Mode().Perm())
	}
}

func TestRequestUnload_RejectsSymlink(t *testing.T) {
	uid := "test-unload-symlink"
	path := UnloadRequestFile(uid)

	// Create a symlink at the sentinel path pointing to a temp file.
	target := path + "-target"
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatalf("creating symlink target: %v", err)
	}
	t.Cleanup(func() { os.Remove(target); os.Remove(path) })

	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	err := RequestUnload(uid)
	if err == nil {
		t.Fatal("expected error when path is a symlink, got nil")
	}

	// The symlink target should be unchanged — we must not have written through it.
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Errorf("symlink target was modified (symlink followed): got %q", data)
	}
}
