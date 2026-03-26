// vault_client.go — Read and write secrets using the HashiCorp Vault Go SDK.
//
// Prerequisites:
//   go mod tidy
//   export VAULT_ADDR="https://vault.example.com:8200"
//   export VAULT_TOKEN="<YOUR_VAULT_TOKEN>"
//
// Usage:
//   go run vault_client.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

// newClient creates an authenticated Vault client from environment variables.
func newClient(ctx context.Context) (*vault.Client, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}

	client, err := vault.New(
		vault.WithAddress(addr),
		vault.WithRequestTimeout(10),
	)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN is not set")
	}

	if err := client.SetToken(token); err != nil {
		return nil, fmt.Errorf("setting vault token: %w", err)
	}

	return client, nil
}

// writeSecret writes key-value pairs to a KV v2 path.
func writeSecret(ctx context.Context, client *vault.Client, mountPath, secretPath string, data map[string]any) error {
	_, err := client.Secrets.KvV2Write(ctx, secretPath,
		schema.KvV2WriteRequest{Data: data},
		vault.WithMountPath(mountPath),
	)
	if err != nil {
		return fmt.Errorf("writing secret at %s/%s: %w", mountPath, secretPath, err)
	}
	fmt.Printf("✅ Secret written to: %s/%s\n", mountPath, secretPath)
	return nil
}

// readSecret reads a KV v2 secret and returns its data.
func readSecret(ctx context.Context, client *vault.Client, mountPath, secretPath string) (map[string]any, error) {
	resp, err := client.Secrets.KvV2Read(ctx, secretPath, vault.WithMountPath(mountPath))
	if err != nil {
		return nil, fmt.Errorf("reading secret at %s/%s: %w", mountPath, secretPath, err)
	}
	data, ok := resp.Data.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected data type from Vault")
	}
	return data, nil
}

// readField reads a single field from a KV v2 secret.
func readField(ctx context.Context, client *vault.Client, mountPath, secretPath, field string) (string, error) {
	data, err := readSecret(ctx, client, mountPath, secretPath)
	if err != nil {
		return "", err
	}
	val, ok := data[field]
	if !ok {
		return "", fmt.Errorf("field %q not found at %s/%s", field, mountPath, secretPath)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string", field)
	}
	return str, nil
}

// deleteSecret soft-deletes the latest version of a KV v2 secret.
func deleteSecret(ctx context.Context, client *vault.Client, mountPath, secretPath string) error {
	_, err := client.Secrets.KvV2Delete(ctx, secretPath, vault.WithMountPath(mountPath))
	if err != nil {
		return fmt.Errorf("deleting secret at %s/%s: %w", mountPath, secretPath, err)
	}
	fmt.Printf("🗑️  Secret deleted: %s/%s\n", mountPath, secretPath)
	return nil
}

func main() {
	ctx := context.Background()

	client, err := newClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create Vault client: %v", err)
	}

	const mount = "secret"
	const path = "myproject/demo"

	// Write
	if err := writeSecret(ctx, client, mount, path, map[string]any{
		"username": "app_user",
		"password": "<DEMO_PASSWORD>",
	}); err != nil {
		log.Fatalf("Write error: %v", err)
	}

	// Read
	data, err := readSecret(ctx, client, mount, path)
	if err != nil {
		log.Fatalf("Read error: %v", err)
	}
	fmt.Printf("Read secret: username=%s\n", data["username"])

	// Read single field
	password, err := readField(ctx, client, mount, path, "password")
	if err != nil {
		log.Fatalf("ReadField error: %v", err)
	}
	fmt.Printf("Password field retrieved (length: %d)\n", len(password))

	// Clean up
	if err := deleteSecret(ctx, client, mount, path); err != nil {
		log.Fatalf("Delete error: %v", err)
	}
}
