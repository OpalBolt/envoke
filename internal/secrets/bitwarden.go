package secrets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// BWClient wraps the bw CLI for secret fetching.
// It does NOT use the Bitwarden SDK — subprocess only.
type BWClient struct {
	Cache          *Cache
	MasterPassword string // cleared after first use

	accountTag     string // cached account tag
	session        string // ephemeral session token
	sessionFromEnv bool   // true if session was sourced from BW_SESSION env var
}

// AccountTag returns an 8-char fingerprint of the active BW account.
func (c *BWClient) AccountTag() (string, error) {
	if c.accountTag != "" {
		return c.accountTag, nil
	}
	cmd := exec.Command("bw", "status")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw status failed: %w", err)
	}
	var status struct {
		UserEmail string `json:"userEmail"`
		ServerURL string `json:"serverUrl"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return "", fmt.Errorf("parsing bw status: %w", err)
	}
	h := bwSHA256Hex(status.UserEmail + status.ServerURL)
	c.accountTag = h[:8]
	return c.accountTag, nil
}

// Session returns an active BW session token.
// Precedence: BW_SESSION env var → RENV_BW_PASSWORD env var → prompt on /dev/tty
func (c *BWClient) Session() (string, error) {
	if c.session != "" {
		return c.session, nil
	}
	// 1. BW_SESSION env var
	if s := os.Getenv("BW_SESSION"); s != "" {
		c.session = s
		c.sessionFromEnv = true
		return c.session, nil
	}

	// 2. RENV_BW_PASSWORD → unlock
	pw := os.Getenv("RENV_BW_PASSWORD")
	if pw == "" {
		pw = c.MasterPassword
	}
	if pw == "" {
		// 3. Prompt on /dev/tty — no echo using x/term
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return "", fmt.Errorf("opening /dev/tty: %w", err)
		}
		defer tty.Close()
		fmt.Fprintf(tty, "Bitwarden master password: ")
		pwBytes, err := term.ReadPassword(int(tty.Fd()))
		if err != nil {
			return "", fmt.Errorf("reading password from tty: %w", err)
		}
		fmt.Fprintln(tty) // newline after hidden input
		defer zeroBytes(pwBytes) // zero the raw bytes before they go out of scope
		pw = string(pwBytes)
		// Store for cache key derivation — cleared after unlock below
		c.MasterPassword = pw
	}

	// bw unlock --raw — pass password via stdin, NOT via argv (argv is world-readable via ps/proc)
	cmd := exec.Command("bw", "unlock", "--raw")
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewBufferString(pw + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw unlock failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("bw unlock returned empty session token")
	}
	c.session = token
	c.sessionFromEnv = false
	// Clear master password
	zeroString(&c.MasterPassword)
	pwCopy := pw
	zeroString(&pwCopy)
	return c.session, nil
}

// FolderItems fetches all items in the given BW folder.
// It ensures cache key material is available before any cache operation.
func (c *BWClient) FolderItems(folder string) ([]map[string]interface{}, error) {
	uid := fmt.Sprintf("%d", os.Getuid())
	acctTag, err := c.AccountTag()
	if err != nil {
		return nil, err
	}

	// Ensure we have a real password/key before cache operations.
	// If only an interactive prompt would provide it, pre-call Session() so
	// c.MasterPassword is populated before masterPasswordForCache() runs.
	if c.MasterPassword == "" && os.Getenv("RENV_BW_PASSWORD") == "" && os.Getenv("BW_SESSION") == "" {
		if _, err := c.Session(); err != nil {
			return nil, err
		}
	}
	pw := c.masterPasswordForCache()
	if pw == "" {
		// No material to encrypt cache — skip cache and fetch directly.
		return c.fetchFolderItemsDirect(folder)
	}

	// Check cache
	if cached, err := c.Cache.Get(uid, acctTag, folder, pw); err == nil && cached != nil {
		var items []map[string]interface{}
		if err := json.Unmarshal(cached, &items); err == nil {
			return items, nil
		}
	}

	// Fetch from BW
	session, err := c.Session()
	if err != nil {
		return nil, err
	}

	// Find folder ID
	folderID, err := c.findFolderID(folder, session)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("bw", "list", "items", "--folderid", folderID)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items failed: %w", err)
	}

	// Cache result
	_ = c.Cache.Put(uid, acctTag, folder, pw, out)

	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing bw list items output: %w", err)
	}
	return items, nil
}

// fetchFolderItemsDirect fetches folder items without cache (used when no encryption key available).
func (c *BWClient) fetchFolderItemsDirect(folder string) ([]map[string]interface{}, error) {
	session, err := c.Session()
	if err != nil {
		return nil, err
	}
	folderID, err := c.findFolderID(folder, session)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("bw", "list", "items", "--folderid", folderID)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items failed: %w", err)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing bw list items output: %w", err)
	}
	return items, nil
}

// CollectionItems fetches all items in the given BW collection.
// URI format: bw://collection:<name>/item[/field]
func (c *BWClient) CollectionItems(collectionName string) ([]map[string]interface{}, error) {
	uid := fmt.Sprintf("%d", os.Getuid())
	acctTag, err := c.AccountTag()
	if err != nil {
		return nil, err
	}

	if c.MasterPassword == "" && os.Getenv("RENV_BW_PASSWORD") == "" && os.Getenv("BW_SESSION") == "" {
		if _, err := c.Session(); err != nil {
			return nil, err
		}
	}
	pw := c.masterPasswordForCache()
	cacheKeyStr := "collection:" + collectionName

	if pw != "" {
		if cached, err := c.Cache.Get(uid, acctTag, cacheKeyStr, pw); err == nil && cached != nil {
			var items []map[string]interface{}
			if err := json.Unmarshal(cached, &items); err == nil {
				return items, nil
			}
		}
	}

	session, err := c.Session()
	if err != nil {
		return nil, err
	}

	collID, err := c.findCollectionID(collectionName, session)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("bw", "list", "items", "--collectionid", collID)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items (collection) failed: %w", err)
	}

	if pw != "" {
		_ = c.Cache.Put(uid, acctTag, cacheKeyStr, pw, out)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing bw list items output: %w", err)
	}
	return items, nil
}

// Close zeros the session token. Call this once when done with all BW operations.
// It only unsets BW_SESSION if we did not read it from the environment.
func (c *BWClient) Close() {
	if c.session != "" {
		zeroString(&c.session)
		c.session = "" // explicitly empty after zeroing NUL-bytes
	}
	if !c.sessionFromEnv {
		os.Unsetenv("BW_SESSION")
	}
}

// findFolderID returns the Bitwarden folder ID for the given folder name.
func (c *BWClient) findFolderID(folder, session string) (string, error) {
	cmd := exec.Command("bw", "list", "folders")
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw list folders failed: %w", err)
	}
	var folders []map[string]interface{}
	if err := json.Unmarshal(out, &folders); err != nil {
		return "", fmt.Errorf("parsing bw list folders: %w", err)
	}
	for _, f := range folders {
		name, _ := f["name"].(string)
		id, _ := f["id"].(string)
		if name == folder {
			return id, nil
		}
	}
	return "", fmt.Errorf("bitwarden folder %q not found", folder)
}

// findCollectionID returns the Bitwarden collection ID for the given collection name.
func (c *BWClient) findCollectionID(name, session string) (string, error) {
	cmd := exec.Command("bw", "list", "collections")
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw list collections failed: %w", err)
	}
	var collections []map[string]interface{}
	if err := json.Unmarshal(out, &collections); err != nil {
		return "", fmt.Errorf("parsing bw list collections: %w", err)
	}
	for _, col := range collections {
		n, _ := col["name"].(string)
		id, _ := col["id"].(string)
		if n == name {
			return id, nil
		}
	}
	return "", fmt.Errorf("bitwarden collection %q not found", name)
}

// Resolve resolves a BWRef to a secret value.
func (c *BWClient) Resolve(ref BWRef) (string, error) {
	var items []map[string]interface{}
	var err error
	if ref.IsCollection {
		items, err = c.CollectionItems(ref.Folder)
	} else {
		items, err = c.FolderItems(ref.Folder)
	}
	if err != nil {
		return "", err
	}
	for _, item := range items {
		name, _ := item["name"].(string)
		if name != ref.Item {
			continue
		}
		val, err := extractField(item, ref.FieldSpec)
		if err != nil {
			return "", fmt.Errorf("bw item %q: %w", ref.Item, err)
		}
		if val == "" {
			return "", fmt.Errorf("bw item %q field %q is empty", ref.Item, ref.FieldSpec)
		}
		return val, nil
	}
	return "", fmt.Errorf("bitwarden item %q not found in folder %q", ref.Item, ref.Folder)
}

// extractField extracts a field value from a BW item map.
func extractField(item map[string]interface{}, fieldSpec string) (string, error) {
	switch fieldSpec {
	case "password":
		login, _ := item["login"].(map[string]interface{})
		if login == nil {
			return "", fmt.Errorf("item has no login")
		}
		val, _ := login["password"].(string)
		return val, nil
	case "username":
		login, _ := item["login"].(map[string]interface{})
		if login == nil {
			return "", fmt.Errorf("item has no login")
		}
		val, _ := login["username"].(string)
		return val, nil
	case "note", "notes":
		val, _ := item["notes"].(string)
		return val, nil
	case "totp":
		login, _ := item["login"].(map[string]interface{})
		if login == nil {
			return "", fmt.Errorf("item has no login")
		}
		val, _ := login["totp"].(string)
		return val, nil
	default:
		if strings.HasPrefix(fieldSpec, "field:") {
			fieldName := strings.TrimPrefix(fieldSpec, "field:")
			fields, _ := item["fields"].([]interface{})
			for _, f := range fields {
				fm, _ := f.(map[string]interface{})
				if fm == nil {
					continue
				}
				if fm["name"] == fieldName {
					val, _ := fm["value"].(string)
					return val, nil
				}
			}
			return "", fmt.Errorf("custom field %q not found", fieldName)
		}
		return "", fmt.Errorf("unknown field spec: %q", fieldSpec)
	}
}

// masterPasswordForCache returns the password to use for cache key derivation.
// Returns empty string if no source is available — callers MUST skip cache in that case.
func (c *BWClient) masterPasswordForCache() string {
	if c.MasterPassword != "" {
		return c.MasterPassword
	}
	if pw := os.Getenv("RENV_BW_PASSWORD"); pw != "" {
		return pw
	}
	if s := os.Getenv("BW_SESSION"); s != "" {
		return s
	}
	// Use the already-obtained session token (only if it's non-trivial length — not NUL-zeroed)
	if len(c.session) > 0 && c.session[0] != 0 {
		return c.session
	}
	// No material available — caller must skip cache to avoid encrypting with a hardcoded key.
	return ""
}

// bwSHA256Hex returns the hex-encoded SHA-256 of s.
func bwSHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
