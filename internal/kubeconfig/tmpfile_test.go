package kubeconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opalbolt/envoke/internal/tmpdir"
)

func TestUnloadRequestFile(t *testing.T) {
	path := UnloadRequestFile("9999")
	if path == "" {
		t.Fatal("UnloadRequestFile returned empty string")
	}
	dir := filepath.Dir(path)
	validDirs := tmpdir.Dirs()
	found := false
	for _, d := range validDirs {
		if dir == d {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("unexpected base dir in path %q (expected one of %v)", path, validDirs)
	}
}

func TestRequestUnload(t *testing.T) {
	uid := "test-kctx-unload-request"
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
	uid := "test-kctx-unload-symlink"
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

func TestClearManaged(t *testing.T) {
	// Create a few fake kctx-*.tmp files in os.TempDir().
	// ClearManaged searches all known tmp locations.
	tmpDir := os.TempDir()
	paths := []string{
		filepath.Join(tmpDir, "kctx-test-clear-managed-1.tmp"),
		filepath.Join(tmpDir, "kctx-test-clear-managed-2.tmp"),
	}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("test"), 0600); err != nil {
			t.Fatalf("creating test file %s: %v", p, err)
		}
	}
	// Ensure cleanup even if ClearManaged doesn't get them.
	t.Cleanup(func() {
		for _, p := range paths {
			os.Remove(p)
		}
	})

	// Verify the files exist before clearing.
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file %s to exist before ClearManaged", p)
		}
	}

	ClearManaged()

	// Verify the files are removed after clearing.
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be removed by ClearManaged", p)
		}
	}
}

func TestClearManaged_NonKctxFilesUntouched(t *testing.T) {
	// Create a non-kctx file that should NOT be removed.
	notKctx := filepath.Join(os.TempDir(), "renv-test-not-kctx.tmp")
	if err := os.WriteFile(notKctx, []byte("keep"), 0600); err != nil {
		t.Fatalf("creating file: %v", err)
	}
	t.Cleanup(func() { os.Remove(notKctx) })

	ClearManaged()

	// The non-kctx file should still exist.
	if _, err := os.Stat(notKctx); err != nil {
		t.Errorf("non-kctx file was incorrectly removed by ClearManaged")
	}
}
