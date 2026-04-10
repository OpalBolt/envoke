//go:build e2e

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// envokeBin holds the path to the compiled envoke binary built once in TestMain.
var envokeBin string

// TestMain compiles the envoke binary before running e2e tests and removes it
// afterwards. This avoids rebuilding per test and ensures tests run against the
// exact binary that would be shipped.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "envoke-e2e-*")
	if err != nil {
		panic("creating temp dir: " + err.Error())
	}

	envokeBin = filepath.Join(dir, "envoke")
	// Build from the full import path so this works regardless of working directory.
	out, buildErr := exec.Command(
		"go", "build", "-o", envokeBin,
		"github.com/eficode/secure-handling-of-secrets/cmd/envoke",
	).CombinedOutput()
	if buildErr != nil {
		os.RemoveAll(dir)
		panic("building envoke: " + buildErr.Error() + "\n" + string(out))
	}

	// os.Exit skips deferred calls — capture the code, clean up, then exit.
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// TestEnvokeVersion checks that the binary runs and prints a version line in
// the expected format ("envoke <version>").
func TestEnvokeVersion(t *testing.T) {
	out, err := exec.Command(envokeBin, "--version").Output()
	if err != nil {
		t.Fatalf("envoke --version: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, "envoke ") {
		t.Errorf("unexpected version output: %q (want prefix \"envoke \")", got)
	}
}

// TestResolveNoSecrets verifies that a .env file with plain KEY=value pairs
// produces the correct export lines without contacting any secret backend.
func TestResolveNoSecrets(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("FOO=bar\nBAZ=hello world\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(envokeBin, "renv", "resolve", "--no-cache", envFile)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve: %v\nstderr: %s", err, stderrFromErr(err))
	}
	got := string(out)

	if !strings.Contains(got, "export FOO='bar'") {
		t.Errorf("missing FOO export in output:\n%s", got)
	}
	if !strings.Contains(got, "export BAZ='hello world'") {
		t.Errorf("missing BAZ export in output:\n%s", got)
	}
}

// TestResolveBWRef verifies that a bw:// secret reference in a .env file is
// resolved via the bw CLI and the secret value is emitted as an export.
//
// A mock bw binary serves fixture JSON so no real Bitwarden account is needed.
// BW_SESSION is pre-set so the unlock step is skipped entirely.
// --no-cache disables the encrypted cache so no local-password prompt occurs.
func TestResolveBWRef(t *testing.T) {
	dir := t.TempDir()

	// mockBWScript dispatches on subcommand and serves minimal fixture data.
	const mockBWScript = `#!/bin/sh
case "$1" in
  status)
    printf '{"userEmail":"test@example.com","serverUrl":"https://bitwarden.example.com"}\n';;
  list)
    case "$2" in
      folders)
        printf '[{"id":"f-prod","name":"prod"}]\n';;
      items)
        printf '[{"name":"mydb","login":{"password":"s3cr3t","username":"admin"}}]\n';;
      *)
        printf 'mock-bw: unknown list subcommand: %s\n' "$2" >&2; exit 1;;
    esac;;
  *)
    printf 'mock-bw: unknown command: %s\n' "$1" >&2; exit 1;;
esac
`
	bwPath := filepath.Join(dir, "bw")
	if err := os.WriteFile(bwPath, []byte(mockBWScript), 0755); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("MY_SECRET=bw://prod/mydb\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Prepend the mock directory to PATH so our bw stub is found first.
	// BW_SESSION causes BWClient.Session() to return immediately without unlock.
	cmd := exec.Command(envokeBin, "renv", "resolve", "--no-cache", envFile)
	cmd.Env = append(os.Environ(),
		"PATH="+dir+":"+os.Getenv("PATH"),
		"BW_SESSION=test-tok",
	)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve bw ref: %v\nstderr: %s", err, stderrFromErr(err))
	}
	got := string(out)
	if !strings.Contains(got, "export MY_SECRET='s3cr3t'") {
		t.Errorf("expected resolved secret in output; got:\n%s", got)
	}
}

// stderrFromErr extracts stderr bytes from an *exec.ExitError, or returns an
// empty string if the error is nil or of a different type.
func stderrFromErr(err error) string {
	if err == nil {
		return ""
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(ee.Stderr)
	}
	return err.Error()
}
