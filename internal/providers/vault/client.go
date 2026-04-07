package vault

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// VaultClient wraps the vault CLI for KV v2 fetching.
type VaultClient struct {
	// Timeout caps each vault subprocess call. Zero uses the 30 s default.
	Timeout time.Duration
}

func (c *VaultClient) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

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

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()

	slog.Debug("vault kv get", "path", ref.Path, "field", ref.Field, "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "vault", "kv", "get", "-field="+ref.Field, ref.Path)
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
