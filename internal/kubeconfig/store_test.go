package kubeconfig

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*NamedStore, string) {
	t.Helper()
	dir := t.TempDir()
	store := &NamedStore{Dir: dir, MaxAge: 8 * time.Hour}
	uid := "testuid"
	return store, uid
}

func TestNamedStore_PutGet_RoundTrip(t *testing.T) {
	store, uid := newTestStore(t)
	data := []byte("apiVersion: v1\nclusters: []\n")

	if err := store.Put(uid, "prod", data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get(uid, "prod")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Get returned %q, want %q", got, data)
	}
}

func TestNamedStore_Get_Miss(t *testing.T) {
	store, uid := newTestStore(t)

	got, err := store.Get(uid, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error on miss: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on miss, got %q", got)
	}
}

func TestNamedStore_Get_Expired(t *testing.T) {
	store, uid := newTestStore(t)

	data := []byte("apiVersion: v1\n")
	if err := store.Put(uid, "prod", data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Backdate the file mtime so the entry is already expired.
	path := store.storePath(uid, "prod")
	expiredAt := time.Now().Add(-2 * store.MaxAge)
	if err := os.Chtimes(path, expiredAt, expiredAt); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	got, err := store.Get(uid, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for expired entry, got data")
	}

	// File should be removed.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected expired file to be deleted")
	}
}

func TestNamedStore_Put_Overwrite(t *testing.T) {
	store, uid := newTestStore(t)

	if err := store.Put(uid, "prod", []byte("v1")); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := store.Put(uid, "prod", []byte("v2")); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, err := store.Get(uid, "prod")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("expected v2, got %q", got)
	}
}

func TestNamedStore_List(t *testing.T) {
	store, uid := newTestStore(t)

	for _, name := range []string{"prod", "staging", "dev"} {
		if err := store.Put(uid, name, []byte("data")); err != nil {
			t.Fatalf("Put %q: %v", name, err)
		}
	}

	names, err := store.List(uid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sort.Strings(names)
	want := []string{"dev", "prod", "staging"}
	if len(names) != len(want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestNamedStore_List_ExcludesExpired(t *testing.T) {
	dir := t.TempDir()
	store := &NamedStore{Dir: dir, MaxAge: time.Hour}
	uid := "testuid"

	if err := store.Put(uid, "old", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Backdate all store files so the entry is already expired.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	expiredAt := time.Now().Add(-2 * store.MaxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Chtimes(filepath.Join(dir, entry.Name()), expiredAt, expiredAt); err != nil {
			t.Fatalf("Chtimes %q: %v", entry.Name(), err)
		}
	}

	names, err := store.List(uid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names after expiry, got %v", names)
	}
}

func TestNamedStore_Remove(t *testing.T) {
	store, uid := newTestStore(t)

	if err := store.Put(uid, "prod", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Remove(uid, "prod"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got, err := store.Get(uid, "prod")
	if err != nil {
		t.Fatalf("Get after Remove: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after Remove, got data")
	}
}

func TestNamedStore_Remove_NotExist(t *testing.T) {
	store, uid := newTestStore(t)
	// Should not error if the file doesn't exist.
	if err := store.Remove(uid, "nonexistent"); err != nil {
		t.Fatalf("Remove non-existent: %v", err)
	}
}

func TestNamedStore_Clear(t *testing.T) {
	store, uid := newTestStore(t)

	for _, name := range []string{"prod", "staging"} {
		if err := store.Put(uid, name, []byte("data")); err != nil {
			t.Fatalf("Put %q: %v", name, err)
		}
	}
	// A file belonging to a different uid — must NOT be removed.
	otherFile := filepath.Join(store.Dir, storePrefix+"otheruid-prod.yaml")
	if err := os.WriteFile(otherFile, []byte("x"), 0600); err != nil {
		t.Fatalf("creating other uid file: %v", err)
	}

	if err := store.Clear(uid); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	names, _ := store.List(uid)
	if len(names) != 0 {
		t.Errorf("expected 0 names after Clear, got %v", names)
	}

	// Other uid file must still exist.
	if _, err := os.Stat(otherFile); err != nil {
		t.Errorf("other uid file was incorrectly removed: %v", err)
	}
}

func TestValidateStoreName(t *testing.T) {
	valid := []string{"prod", "staging", "my-cluster", "cluster_1", "a.b"}
	for _, n := range valid {
		if err := ValidateStoreName(n); err != nil {
			t.Errorf("ValidateStoreName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []string{"", "has space", "has/slash", "has:colon", "../escape"}
	for _, n := range invalid {
		if err := ValidateStoreName(n); err == nil {
			t.Errorf("ValidateStoreName(%q) = nil, want error", n)
		}
	}
}

func TestNamedStore_FileMode(t *testing.T) {
	store, uid := newTestStore(t)

	if err := store.Put(uid, "prod", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	path := store.storePath(uid, "prod")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", fi.Mode().Perm())
	}
}
