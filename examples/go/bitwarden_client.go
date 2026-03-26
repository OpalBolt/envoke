// bitwarden_client.go — Retrieve secrets from Bitwarden Secrets Manager via the official SDK.
//
// This example uses the Bitwarden Secrets Manager product, designed for
// machine-to-machine access (CI/CD, applications). It uses access tokens —
// NOT the BW_SESSION used by the bw CLI for the personal vault.
//
// Prerequisites:
//   go mod tidy
//
//   # In Bitwarden: Organisation → Secrets Manager → Service Accounts
//   # Create a service account, generate an access token, note the org ID.
//   export BWS_ACCESS_TOKEN="0.your-access-token..."
//   export BWS_ORGANIZATION_ID="your-org-uuid"
//
// Usage:
//   go run bitwarden_client.go
//
// Docs: https://bitwarden.com/help/secrets-manager-overview/
// SDK:  https://github.com/bitwarden/sdk-go

package main

import (
	"fmt"
	"log"
	"os"

	sdk "github.com/bitwarden/sdk-go"
)

// BitwardenSecretsClient wraps the Bitwarden Secrets Manager SDK client.
type BitwardenSecretsClient struct {
	client sdk.BitwardenClientInterface
	orgID  string
}

// NewBitwardenSecretsClient creates and authenticates a Secrets Manager client.
func NewBitwardenSecretsClient() (*BitwardenSecretsClient, error) {
	accessToken := os.Getenv("BWS_ACCESS_TOKEN")
	if accessToken == "" {
		return nil, fmt.Errorf(
			"BWS_ACCESS_TOKEN is not set\n" +
				"Generate one under: Organisation → Secrets Manager → Service Accounts",
		)
	}

	orgID := os.Getenv("BWS_ORGANIZATION_ID")
	if orgID == "" {
		return nil, fmt.Errorf(
			"BWS_ORGANIZATION_ID is not set\n" +
				"Find it under: Organisation Settings in the Bitwarden web app",
		)
	}

	apiURL := "https://api.bitwarden.com"
	identityURL := "https://identity.bitwarden.com"

	client, err := sdk.NewBitwardenClient(&apiURL, &identityURL)
	if err != nil {
		return nil, fmt.Errorf("creating Bitwarden client: %w", err)
	}

	// stateFile persists auth state between calls; nil disables it.
	var stateFile *string
	if sf := os.Getenv("BWS_STATE_FILE"); sf != "" {
		stateFile = &sf
	}

	// v1.0.0 API: AccessTokenLogin is a top-level method on the client.
	if err := client.AccessTokenLogin(accessToken, stateFile); err != nil {
		client.Close()
		return nil, fmt.Errorf("authenticating with access token: %w", err)
	}

	return &BitwardenSecretsClient{client: client, orgID: orgID}, nil
}

// Close releases the underlying SDK client resources.
func (c *BitwardenSecretsClient) Close() {
	c.client.Close()
}

// GetSecretByID retrieves a secret value by its UUID.
func (c *BitwardenSecretsClient) GetSecretByID(secretID string) (string, error) {
	response, err := c.client.Secrets().Get(secretID)
	if err != nil {
		return "", fmt.Errorf("getting secret %q: %w", secretID, err)
	}
	return response.Value, nil
}

// GetSecretByKey retrieves a secret value by its human-readable key name.
// Prefer GetSecretByID when the UUID is known — it avoids an extra API call.
func (c *BitwardenSecretsClient) GetSecretByKey(key string) (string, error) {
	secrets, err := c.client.Secrets().List(c.orgID)
	if err != nil {
		return "", fmt.Errorf("listing secrets: %w", err)
	}
	for _, s := range secrets.Data {
		if s.Key == key {
			return c.GetSecretByID(s.ID)
		}
	}
	return "", fmt.Errorf("secret with key %q not found in organisation", key)
}

// ListSecrets returns all secrets (key + ID only; values are not included).
func (c *BitwardenSecretsClient) ListSecrets() ([]sdk.SecretIdentifierResponse, error) {
	response, err := c.client.Secrets().List(c.orgID)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}
	return response.Data, nil
}

// CreateSecret creates a new secret and returns its UUID.
func (c *BitwardenSecretsClient) CreateSecret(key, value, note string, projectIDs []string) (string, error) {
	response, err := c.client.Secrets().Create(key, value, note, c.orgID, projectIDs)
	if err != nil {
		return "", fmt.Errorf("creating secret %q: %w", key, err)
	}
	return response.ID, nil
}

// UpdateSecret updates an existing secret by UUID.
func (c *BitwardenSecretsClient) UpdateSecret(secretID, key, value, note string, projectIDs []string) error {
	_, err := c.client.Secrets().Update(secretID, key, value, note, c.orgID, projectIDs)
	if err != nil {
		return fmt.Errorf("updating secret %q: %w", secretID, err)
	}
	return nil
}

// DeleteSecrets deletes one or more secrets by UUID.
func (c *BitwardenSecretsClient) DeleteSecrets(secretIDs []string) error {
	_, err := c.client.Secrets().Delete(secretIDs)
	if err != nil {
		return fmt.Errorf("deleting secrets: %w", err)
	}
	return nil
}

func main() {
	bws, err := NewBitwardenSecretsClient()
	if err != nil {
		log.Fatalf("Failed to create Bitwarden Secrets Manager client: %v", err)
	}
	defer bws.Close()

	// List all secrets (key + ID only — values not included in listing)
	secrets, err := bws.ListSecrets()
	if err != nil {
		log.Fatalf("ListSecrets error: %v", err)
	}
	fmt.Printf("Organisation has %d secret(s):\n", len(secrets))
	for _, s := range secrets {
		fmt.Printf("  %s  (%s)\n", s.Key, s.ID)
	}

	if len(secrets) > 0 {
		first := secrets[0]
		value, err := bws.GetSecretByID(first.ID)
		if err != nil {
			fmt.Printf("Could not retrieve secret: %v\n", err)
		} else {
			fmt.Printf("\nFirst secret '%s' value length: %d\n", first.Key, len(value))
		}
	}
}
