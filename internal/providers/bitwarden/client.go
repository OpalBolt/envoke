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

	appcache "github.com/eficode/secure-handling-of-secrets/internal/cache"
	"golang.org/x/term"
)

// ErrInvalidPassword is returned when the Bitwarden master password is rejected.
var ErrInvalidPassword = errors.New("invalid Bitwarden master password")

// BWClient wraps the bw CLI for secret fetching.
// It does NOT use the Bitwarden SDK — subprocess only.
//
// Two-password model:
//   - BWPassword: the Bitwarden master password, used only for `bw unlock`. Never
//     persisted to disk. Only needed on a cache miss or first access to a folder.
//   - LocalPassword: a local session password used to encrypt/decrypt the cache
//     files in /dev/shm. Prompted once per invocation (or sourced from
//     RENV_LOCAL_PASSWORD). Held in process memory only; never written to disk.
//
// Access flow:
//  1. First access to a folder: prompt LocalPassword + BWPassword → fetch from BW
//     → encrypt with LocalPassword → write to /dev/shm cache.
//  2. Subsequent access within cache TTL: prompt LocalPassword only → decrypt
//     cache. No Bitwarden contact.
//  3. Access after cache TTL expires: prompt LocalPassword + BWPassword → re-fetch.
type BWClient struct {
	Cache         *appcache.Cache
	BWPassword    string // cleared after bw unlock; used only for `bw unlock --raw`
	LocalPassword string // used only for cache encryption; never sent to Bitwarden; held in memory only
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

// ensureLocalPassword populates c.LocalPassword from (in order of precedence):
//  1. c.LocalPassword already set
//  2. RENV_LOCAL_PASSWORD env var
//  3. Prompt on /dev/tty; password is stored in c.LocalPassword for the lifetime
//     of this process only — never written to disk.
func (c *BWClient) ensureLocalPassword() error {
	if c.LocalPassword != "" {
		return nil
	}
	if pw := os.Getenv("RENV_LOCAL_PASSWORD"); pw != "" {
		if strings.TrimSpace(pw) == "" {
			return fmt.Errorf("RENV_LOCAL_PASSWORD must not be empty or whitespace-only")
		}
		c.LocalPassword = pw
		return nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening /dev/tty: %w", err)
	}
	defer tty.Close()
	fmt.Fprintf(tty, "Local cache password: ")
	pwBytes, err := term.ReadPassword(int(tty.Fd()))
	if err != nil {
		return fmt.Errorf("reading local cache password from tty: %w", err)
	}
	fmt.Fprintln(tty)
	if strings.TrimSpace(string(pwBytes)) == "" {
		return fmt.Errorf("local cache password must not be empty")
	}
	c.LocalPassword = string(pwBytes)
	return nil
}

// ReadLocalPassword returns the local cache encryption password from, in order:
//  1. RENV_LOCAL_PASSWORD env var
//  2. An interactive prompt on /dev/tty (no echo)
//
// Use this when you need the local password outside of a BWClient (e.g., when
// fetching from Vault and then encrypting to the named kubeconfig store).
// Returns an error if the password is empty or whitespace-only.
func ReadLocalPassword() (string, error) {
	if pw := os.Getenv("RENV_LOCAL_PASSWORD"); pw != "" {
		if strings.TrimSpace(pw) == "" {
			return "", fmt.Errorf("RENV_LOCAL_PASSWORD must not be empty or whitespace-only")
		}
		return pw, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening /dev/tty: %w", err)
	}
	defer tty.Close()
	fmt.Fprintf(tty, "Local cache password: ")
	pwBytes, err := term.ReadPassword(int(tty.Fd()))
	if err != nil {
		return "", fmt.Errorf("reading local cache password from tty: %w", err)
	}
	fmt.Fprintln(tty)
	if strings.TrimSpace(string(pwBytes)) == "" {
		return "", fmt.Errorf("local cache password must not be empty")
	}
	return string(pwBytes), nil
}

// AccountTag returns an 8-char fingerprint of the active BW account.
// It is used as a cache key discriminator to namespace cache entries per account.
// Result is memoised for the lifetime of the client and persisted to disk so
// subsequent runs can skip `bw status` entirely on cache-hit paths.
func (c *BWClient) AccountTag() (string, error) {
	if c.accountTag != "" {
		return c.accountTag, nil
	}
	if stored := c.loadStoredAccountTag(); stored != "" {
		c.accountTag = stored
		return c.accountTag, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw status", "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "bw", "status")
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
	c.saveAccountTag(c.accountTag)
	return c.accountTag, nil
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

// acctTagStorePath returns the path where the BW account tag is persisted between process invocations.
func acctTagStorePath(uid string) string {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	return filepath.Join(dir, "renv-bw-acct-tag-"+uid)
}

// loadStoredAccountTag reads a previously saved BW account tag from disk.
// Returns "" if not found or unreadable.
func (c *BWClient) loadStoredAccountTag() string {
	uid := fmt.Sprintf("%d", os.Getuid())
	data, err := os.ReadFile(acctTagStorePath(uid))
	if err != nil {
		return ""
	}
	tag := strings.TrimSpace(string(data))
	if len(tag) != 8 {
		return ""
	}
	slog.Debug("loaded stored BW account tag", "path", acctTagStorePath(uid))
	return tag
}

// saveAccountTag persists the BW account tag to disk (chmod 600).
func (c *BWClient) saveAccountTag(tag string) {
	uid := fmt.Sprintf("%d", os.Getuid())
	path := acctTagStorePath(uid)
	_ = os.WriteFile(path, []byte(tag), 0600)
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
//
// On cache hit (local file exists and not expired): only LocalPassword is required.
// On cache miss or expiry: LocalPassword + BWPassword (or BW_SESSION) are required.
func (c *BWClient) FolderItems(folder string) ([]map[string]interface{}, error) {
	uid := fmt.Sprintf("%d", os.Getuid())
	acctTag, err := c.AccountTag()
	if err != nil {
		return nil, err
	}

	// Prompt for local cache password (skipped when cache is disabled or env var set).
	if !c.Cache.Disabled {
		if err := c.ensureLocalPassword(); err != nil {
			return nil, err
		}
	}
	pw := c.localPasswordForCache()

	// Check local encrypted cache — no BW contact needed on hit.
	if pw == "" {
		slog.Debug("skipping cache (no local password set)", "folder", folder)
	} else {
		cached, cacheErr := c.Cache.Get(uid, bwCacheKey(uid, acctTag, folder), pw)
		if cacheErr != nil {
			slog.Warn("cache decryption failed; falling back to Bitwarden (wrong local password?)",
				"folder", folder, "error", cacheErr)
		} else if cached != nil {
			var items []map[string]interface{}
			if jsonErr := json.Unmarshal(cached, &items); jsonErr != nil {
				slog.Warn("cached data is not valid JSON; falling back to Bitwarden",
					"folder", folder, "error", jsonErr)
			} else {
				slog.Debug("cache hit", "folder", folder)
				return items, nil
			}
		} else {
			slog.Debug("cache miss", "folder", folder)
		}
	}

	// Cache miss — contact Bitwarden (prompts BWPassword if needed).
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

	if pw != "" {
		_ = c.Cache.Put(uid, bwCacheKey(uid, acctTag, folder), pw, out)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing bw list items output: %w", err)
	}
	return items, nil
}

// CollectionItems fetches all items in the given BW collection.
// URI format: bw://collection:<name>/item[/field]
//
// Follows the same two-password flow as FolderItems.
func (c *BWClient) CollectionItems(collectionName string) ([]map[string]interface{}, error) {
	uid := fmt.Sprintf("%d", os.Getuid())
	acctTag, err := c.AccountTag()
	if err != nil {
		return nil, err
	}

	if !c.Cache.Disabled {
		if err := c.ensureLocalPassword(); err != nil {
			return nil, err
		}
	}
	pw := c.localPasswordForCache()
	cacheKeyStr := "collection:" + collectionName

	if pw == "" {
		slog.Debug("skipping cache (no local password set)", "collection", collectionName)
	} else {
		cached, cacheErr := c.Cache.Get(uid, bwCacheKey(uid, acctTag, cacheKeyStr), pw)
		if cacheErr != nil {
			slog.Warn("cache decryption failed; falling back to Bitwarden (wrong local password?)",
				"collection", collectionName, "error", cacheErr)
		} else if cached != nil {
			var items []map[string]interface{}
			if jsonErr := json.Unmarshal(cached, &items); jsonErr != nil {
				slog.Warn("cached data is not valid JSON; falling back to Bitwarden",
					"collection", collectionName, "error", jsonErr)
			} else {
				slog.Debug("cache hit", "collection", collectionName)
				return items, nil
			}
		} else {
			slog.Debug("cache miss", "collection", collectionName)
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

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	slog.Debug("bw list items (collection)", "collection", collectionName, "timeout", c.timeout())
	cmd := exec.CommandContext(ctx, "bw", "list", "items", "--collectionid", collID)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw list items (collection) failed: %w", err)
	}

	if pw != "" {
		_ = c.Cache.Put(uid, bwCacheKey(uid, acctTag, cacheKeyStr), pw, out)
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

// localPasswordForCache returns the local session password used for cache encryption.
// Returns empty string if not yet set — callers should call ensureLocalPassword first.
func (c *BWClient) localPasswordForCache() string {
	if c.LocalPassword != "" {
		return c.LocalPassword
	}
	if pw := os.Getenv("RENV_LOCAL_PASSWORD"); pw != "" {
		return pw
	}
	return ""
}

// bwSHA256Hex returns the hex-encoded SHA-256 of s.
func bwSHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
