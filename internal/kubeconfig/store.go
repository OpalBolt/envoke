package kubeconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	storeKeyLen    = 32
	storeSaltLen   = 32
	storeIVLen     = 16
	storeIter      = 100_000
	storeFilePerms = 0600
	storePrefix    = "kctx-kc-"
)

// validStoreName matches safe kubeconfig names (alphanumeric, dash, underscore, dot).
var validStoreName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// NamedStore stores named kubeconfig data, encrypted with a local password.
// Files are stored as kctx-kc-<uid>-<name>.enc in /dev/shm (preferred) or /tmp.
// Encryption: AES-256-CBC, key derived via PBKDF2-SHA256.
type NamedStore struct {
	Dir    string
	MaxAge time.Duration
}

// NewNamedStore picks /dev/shm if available and writable, else /tmp.
func NewNamedStore() *NamedStore {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		testPath := filepath.Join("/dev/shm", ".kctx-ns-test")
		if f, err := os.OpenFile(testPath, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(testPath)
			dir = "/dev/shm"
		}
	}
	return &NamedStore{Dir: dir, MaxAge: 8 * time.Hour}
}

// ValidateStoreName returns an error if name is empty or contains unsafe characters.
func ValidateStoreName(name string) error {
	if name == "" {
		return fmt.Errorf("kubeconfig name must not be empty")
	}
	if !validStoreName.MatchString(name) {
		return fmt.Errorf("invalid kubeconfig name %q: only [a-zA-Z0-9_.-] are allowed", name)
	}
	return nil
}

// storePath returns the on-disk path for a named kubeconfig.
func (s *NamedStore) storePath(uid, name string) string {
	return filepath.Join(s.Dir, storePrefix+uid+"-"+name+".enc")
}

// Put encrypts data and writes it as the named kubeconfig for uid.
// Existing entries with the same name are overwritten.
func (s *NamedStore) Put(uid, name, password string, data []byte) error {
	if err := ValidateStoreName(name); err != nil {
		return err
	}

	salt := make([]byte, storeSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("store: generating salt: %w", err)
	}
	iv := make([]byte, storeIVLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return fmt.Errorf("store: generating IV: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, storeIter, storeKeyLen, sha256.New)
	defer storeZeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("store: creating cipher: %w", err)
	}
	padded := storePkcs7Pad(data, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	path := s.storePath(uid, name)
	slog.Debug("named store put", "path", path, "name", name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, storeFilePerms)
	if err != nil {
		return fmt.Errorf("store: opening file: %w", err)
	}
	defer f.Close()
	for _, chunk := range [][]byte{salt, iv, ciphertext} {
		if _, err := f.Write(chunk); err != nil {
			return fmt.Errorf("store: writing data: %w", err)
		}
	}
	return nil
}

// Get decrypts and returns the stored kubeconfig for uid/name.
// Returns (nil, nil) on cache miss or expiry.
func (s *NamedStore) Get(uid, name, password string) ([]byte, error) {
	if err := ValidateStoreName(name); err != nil {
		return nil, err
	}
	path := s.storePath(uid, name)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		slog.Debug("named store miss (not found)", "name", name)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: stat: %w", err)
	}
	if s.MaxAge > 0 && time.Since(fi.ModTime()) > s.MaxAge {
		slog.Debug("named store expired", "name", name, "age", time.Since(fi.ModTime()).Round(time.Second))
		os.Remove(path)
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: reading file: %w", err)
	}
	if len(raw) < storeSaltLen+storeIVLen+aes.BlockSize {
		return nil, fmt.Errorf("store: file too short")
	}

	salt := raw[:storeSaltLen]
	iv := raw[storeSaltLen : storeSaltLen+storeIVLen]
	ciphertext := raw[storeSaltLen+storeIVLen:]

	key := pbkdf2.Key([]byte(password), salt, storeIter, storeKeyLen, sha256.New)
	defer storeZeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("store: creating cipher: %w", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("store: ciphertext not block-aligned")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)

	unpadded, err := storePkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("store: wrong password or corrupted data: %w", err)
	}
	return unpadded, nil
}

// List returns the names of all non-expired named kubeconfigs for uid.
func (s *NamedStore) List(uid string) ([]string, error) {
	pattern := filepath.Join(s.Dir, storePrefix+uid+"-*.enc")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("store: glob: %w", err)
	}
	prefix := storePrefix + uid + "-"
	var names []string
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimPrefix(base, prefix)
		name = strings.TrimSuffix(name, ".enc")
		if !validStoreName.MatchString(name) {
			continue
		}
		if s.MaxAge > 0 {
			if fi, err := os.Stat(m); err == nil && time.Since(fi.ModTime()) > s.MaxAge {
				os.Remove(m)
				continue
			}
		}
		names = append(names, name)
	}
	return names, nil
}

// Remove deletes the named kubeconfig for uid. No error if it does not exist.
func (s *NamedStore) Remove(uid, name string) error {
	if err := ValidateStoreName(name); err != nil {
		return err
	}
	path := s.storePath(uid, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: removing %s: %w", path, err)
	}
	return nil
}

// Clear removes all named kubeconfigs for uid.
func (s *NamedStore) Clear(uid string) error {
	pattern := filepath.Join(s.Dir, storePrefix+uid+"-*.enc")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("store: glob: %w", err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: removing %s: %w", m, err)
		}
	}
	return nil
}

// --- crypto helpers (self-contained, matching the secrets/cache.go approach) ---

func storeZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func storePkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

func storePkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > aes.BlockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding byte: %d", padding)
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}
