package bitwarden

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

// ErrInvalidPassword is returned when the Bitwarden master password is rejected.
var ErrInvalidPassword = errors.New("invalid Bitwarden master password")

// BWClient wraps the bw CLI for secret fetching.
// It does NOT use the Bitwarden SDK — subprocess only.
//
// The BWPassword is used only for `bw unlock`. It is never persisted to disk
// and held in process memory only.
//
// Access flow:
//  1. On Bitwarden access: prompt BWPassword (or use BW_SESSION env var)
//     → fetch from BW.
type BWClient struct {
	BWPassword string // cleared after bw unlock; used only for `bw unlock --raw`
	// Timeout caps each bw subprocess call. Zero uses the 30 s default.
	Timeout time.Duration

	accountTag     string // cached account tag
	session        string // ephemeral in-process session token; never written to disk
	sessionFromEnv bool   // true if session was sourced from BW_SESSION env var
}

func (c *BWClient) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

// Session returns an active BW session token.
// Precedence: BW_SESSION env var → RENV_BW_PASSWORD env var → BWPassword field → prompt on /dev/tty
//
// The session token is held in process memory only and is never written to disk.
// Each invocation that requires Bitwarden access will prompt for BWPassword unless
// BW_SESSION or RENV_BW_PASSWORD is set.
func (c *BWClient) Session() (string, error) {
	if c.session != "" {
		return c.session, nil
	}
	// 1. BW_SESSION env var
	if s := os.Getenv("BW_SESSION"); s != "" {
		slog.Debug("using BW session from environment")
		c.session = s
		c.sessionFromEnv = true
		return c.session, nil
	}

	// 2. RENV_BW_PASSWORD → unlock
	pw := os.Getenv("RENV_BW_PASSWORD")
	if pw == "" {
		pw = c.BWPassword
	}
	if pw == "" {
		// 3. Prompt on /dev/tty — no echo using x/term
		slog.Debug("prompting for BW master password on /dev/tty")
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
		fmt.Fprintln(tty)        // newline after hidden input
		defer zeroBytes(pwBytes) // zero the raw bytes before they go out of scope
		pw = string(pwBytes)
		// Store for the duration of this unlock call — cleared below
		c.BWPassword = pw
	}

	// bw unlock --raw — pass password via stdin, NOT via argv (argv is world-readable via ps/proc)
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw unlock", "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "bw", "unlock", "--raw")
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewBufferString(pw + "\n")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if strings.Contains(stderrStr, "Invalid master password") {
			return "", ErrInvalidPassword
		}
		if stderrStr != "" {
			fmt.Fprintln(os.Stderr, stderrStr)
		}
		return "", fmt.Errorf("bw unlock failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("bw unlock returned empty session token")
	}
	slog.Debug("BW unlock successful; session held in memory only")
	c.session = token
	c.sessionFromEnv = false
	// Clear BW password immediately — it is no longer needed
	zeroString(&c.BWPassword)
	pwCopy := pw
	zeroString(&pwCopy)
	return c.session, nil
}

// sessionStorePath returns the path where a legacy plaintext BW session may exist.
// Used only by ClearStoredSession to clean up files left by older versions.
func sessionStorePath(uid string) string {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	return filepath.Join(dir, "renv-bw-session-"+uid)
}

// localKeyStorePath returns the legacy path where the shared LocalPassword was
// stored by older versions of renv. LocalPassword is no longer written to disk;
// this helper exists only so ClearStoredLocalPassword can remove any leftover files.
func localKeyStorePath(uid string) string {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	return filepath.Join(dir, "renv-local-key-"+uid)
}

// ClearStoredLocalPassword removes any local password files left by previous
// versions of renv. The password is no longer written to disk, but legacy files
// may still exist on systems that ran an older version.
func ClearStoredLocalPassword(uid string) error {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	// Remove shared store.
	sharedPath := localKeyStorePath(uid)
	slog.Debug("clearing stored local password", "path", sharedPath)
	if err := os.Remove(sharedPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing local password: %w", err)
	}
	// Remove all per-terminal session stores (renv-local-key-<uid>-<ppid>).
	pattern := filepath.Join(dir, "renv-local-key-"+uid+"-*")
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		_ = os.Remove(m)
	}
	return nil
}

// ClearStoredSession removes any legacy plaintext BW session file left by older versions.
func ClearStoredSession(uid string) error {
	path := sessionStorePath(uid)
	slog.Debug("clearing stored BW session", "path", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing session: %w", err)
	}
	return nil
}

// FolderItems fetches all items in the given BW folder.
// Requires BWPassword (or BW_SESSION) to be available.
func (c *BWClient) FolderItems(folder string) ([]map[string]interface{}, error) {
	// Contact Bitwarden directly.
	session, err := c.Session()
	if err != nil {
		return nil, err
	}

	folderID, err := c.findFolderID(folder, session)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw list items", "folder", folder, "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "bw", "list", "items", "--folderid", folderID)
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
// Requires BWPassword (or BW_SESSION) to be available.
func (c *BWClient) CollectionItems(collectionName string) ([]map[string]interface{}, error) {
	session, err := c.Session()
	if err != nil {
		return nil, err
	}

	collID, err := c.findCollectionID(collectionName, session)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw list items (collection)", "collection", collectionName, "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "bw", "list", "items", "--collectionid", collID)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items (collection) failed: %w", err)
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
		slog.Debug("closing BW client, zeroing session token")
		zeroString(&c.session)
		c.session = "" // explicitly empty after zeroing NUL-bytes
	}
	if !c.sessionFromEnv {
		os.Unsetenv("BW_SESSION")
	}
}

// findFolderID returns the Bitwarden folder ID for the given folder name.
func (c *BWClient) findFolderID(folder, session string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw list folders", "folder", folder)
	cmd := exec.CommandContext(ctx, "bw", "list", "folders")
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
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw list collections", "collection", name)
	cmd := exec.CommandContext(ctx, "bw", "list", "collections")
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

// bwSHA256Hex returns the hex-encoded SHA-256 of s.
func bwSHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// zeroBytes overwrites a byte slice with zeros.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// zeroString clears the caller-visible string reference.
// In Go, strings are immutable, so this does not overwrite the original
// backing memory; it only drops this reference to the secret value.
func zeroString(s *string) {
	*s = ""
}
