package bitwarden_test

import (
	"os"
	"testing"
	"time"

	"github.com/eficode/envoke/internal/providers/bitwarden"
)

func TestCacheRoundtrip(t *testing.T) {
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = 8 * time.Hour

	data := []byte(`[{"id":"abc","name":"myitem","login":{"password":"s3cr3t"}}]`)
	if err := c.Put("1000", "acct1:myfolder", "masterpass", data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := c.Get("1000", "acct1:myfolder", "masterpass")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, data)
	}
}

func TestCacheWrongPassword(t *testing.T) {
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = 8 * time.Hour

	data := []byte(`[{"id":"abc"}]`)
	if err := c.Put("1000", "acct1:myfolder", "correctpass", data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	_, err := c.Get("1000", "acct1:myfolder", "wrongpass")
	if err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

func TestCacheExpired(t *testing.T) {
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = -1 * time.Second // always expired

	data := []byte(`[{"id":"abc"}]`)
	if err := c.Put("1000", "acct1:myfolder", "pass", data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := c.Get("1000", "acct1:myfolder", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for expired cache, got data")
	}
}

func TestCacheMiss(t *testing.T) {
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = 8 * time.Hour

	got, err := c.Get("1000", "acct1:nonexistent", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for cache miss, got data")
	}
}

func TestCacheClear(t *testing.T) {
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = 8 * time.Hour

	data := []byte(`[{"id":"abc"}]`)
	c.Put("1000", "acct1:folder1", "pass", data)
	c.Put("1000", "acct1:folder2", "pass", data)

	if err := c.Clear("1000"); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	f1 := c.FilePath("1000", "acct1:folder1")
	f2 := c.FilePath("1000", "acct1:folder2")
	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Error("expected f1 to be removed")
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Error("expected f2 to be removed")
	}
}

func TestCacheFilePerms(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skip file perm check in CI")
	}
	c := bitwarden.NewCache()
	c.Dir = t.TempDir()
	c.MaxAge = 8 * time.Hour

	data := []byte(`[{"id":"abc"}]`)
	if err := c.Put("1000", "acct1:myfolder", "pass", data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	fi, err := os.Stat(c.FilePath("1000", "acct1:myfolder"))
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", fi.Mode().Perm())
	}
}
