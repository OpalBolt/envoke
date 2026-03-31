package secrets

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// VaultClient wraps the vault CLI for KV v2 fetching.
type VaultClient struct{}

// Resolve runs `vault kv get -field=<field> <path>` and returns the value.
// Requires VAULT_ADDR and VAULT_TOKEN to be set.
func (c *VaultClient) Resolve(ref VaultRef) (string, error) {
	if ref.Path == "" {
		return "", fmt.Errorf("vault:// URI has empty path")
	}
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		return "", fmt.Errorf("VAULT_ADDR not set")
	}
	if _, err := exec.LookPath("vault"); err != nil {
		return "", fmt.Errorf("vault CLI not found in PATH: %w", err)
	}
	cmd := exec.Command("vault", "kv", "get", "-field="+ref.Field, ref.Path)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("vault kv get failed for %q#%s: %w", ref.Path, ref.Field, err)
	}
	val := strings.TrimSpace(string(out))
	if val == "" {
		return "", fmt.Errorf("vault kv get returned empty value for %q#%s", ref.Path, ref.Field)
	}
	return val, nil
}
