// bitwarden_client.go — Retrieve secrets from Bitwarden via the bw CLI.
//
// Prerequisites:
//   go mod tidy
//   bw login (or bw login --apikey)
//   export BW_SESSION=$(bw unlock --raw)
//
// Usage:
//   go run bitwarden_client.go

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// BitwardenItem represents a Bitwarden vault item (partial).
type BitwardenItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Notes string `json:"notes"`
	Login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"login"`
	Fields []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"fields"`
}

// BitwardenClient wraps the bw CLI.
type BitwardenClient struct {
	session string
}

// NewBitwardenClient creates a client using the BW_SESSION environment variable.
func NewBitwardenClient() (*BitwardenClient, error) {
	session := os.Getenv("BW_SESSION")
	if session == "" {
		return nil, fmt.Errorf("BW_SESSION is not set — run: export BW_SESSION=$(bw unlock --raw)")
	}
	c := &BitwardenClient{session: session}
	if err := c.verifyUnlocked(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *BitwardenClient) run(args ...string) (string, error) {
	args = append(args, "--session", c.session, "--nointeraction")
	cmd := exec.Command("bw", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw %s: %w", strings.Join(args[:len(args)-2], " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *BitwardenClient) verifyUnlocked() error {
	out, err := c.run("status")
	if err != nil || !strings.Contains(out, `"status":"unlocked"`) {
		return fmt.Errorf("Bitwarden vault is locked or session key is invalid")
	}
	return nil
}

// Sync syncs the vault from the server.
func (c *BitwardenClient) Sync() error {
	_, err := c.run("sync")
	return err
}

// GetItem retrieves a full item by name.
func (c *BitwardenClient) GetItem(itemName string) (*BitwardenItem, error) {
	out, err := c.run("get", "item", itemName)
	if err != nil {
		return nil, fmt.Errorf("item %q not found: %w", itemName, err)
	}
	var item BitwardenItem
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return nil, fmt.Errorf("parsing item JSON: %w", err)
	}
	return &item, nil
}

// GetPassword retrieves the password of a login item by name.
func (c *BitwardenClient) GetPassword(itemName string) (string, error) {
	out, err := c.run("get", "password", itemName)
	if err != nil || out == "" {
		return "", fmt.Errorf("password for %q not found: %w", itemName, err)
	}
	return out, nil
}

// GetField retrieves a custom field from a named item.
func (c *BitwardenClient) GetField(itemName, fieldName string) (string, error) {
	item, err := c.GetItem(itemName)
	if err != nil {
		return "", err
	}
	for _, f := range item.Fields {
		if f.Name == fieldName {
			return f.Value, nil
		}
	}
	return "", fmt.Errorf("field %q not found in item %q", fieldName, itemName)
}

// ListItems returns all items matching an optional search term.
func (c *BitwardenClient) ListItems(search string) ([]BitwardenItem, error) {
	args := []string{"list", "items"}
	if search != "" {
		args = append(args, "--search", search)
	}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var items []BitwardenItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, fmt.Errorf("parsing items JSON: %w", err)
	}
	return items, nil
}

func main() {
	client, err := NewBitwardenClient()
	if err != nil {
		log.Fatalf("Failed to create Bitwarden client: %v", err)
	}

	items, err := client.ListItems("github")
	if err != nil {
		log.Fatalf("ListItems error: %v", err)
	}
	fmt.Printf("Found %d item(s) matching 'github'\n", len(items))

	if len(items) > 0 {
		item := items[0]
		fmt.Printf("First match: %s\n", item.Name)

		password, err := client.GetPassword(item.Name)
		if err != nil {
			fmt.Printf("Could not retrieve password: %v\n", err)
		} else {
			fmt.Printf("Password retrieved (length: %d)\n", len(password))
		}
	}
}
