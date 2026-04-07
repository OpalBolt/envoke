// Package cache provides an encrypted disk cache for resolved secret values.
// It is provider-agnostic: callers supply an opaque string key and a password
// used to derive the AES-256-CBC encryption key via PBKDF2-SHA256.
// Files are stored in /dev/shm (Linux tmpfs) when available, falling back to /tmp.
package cache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	keyLen     = 32
	saltLen    = 32
	ivLen      = 16
	iterations = 100_000
	filePerms  = 0600
)

// Cache stores encrypted secret values on disk.
// Key derivation: PBKDF2-SHA256(password, salt, 100_000 iter, 32 bytes)
// Encryption: AES-256-CBC
// NOTE: AES-CBC has no authentication (malleable). A future upgrade to AES-GCM
// would add integrity checking.
type Cache struct {
	Dir      string
	MaxAge   time.Duration
	Disabled bool // when true, Put and Get are no-ops (--no-cache flag)
}

// New picks /dev/shm if available and writable, else /tmp.
func New() *Cache {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		testFile := filepath.Join("/dev/shm", ".envoke-cache-test")
		if f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(testFile)
			dir = "/dev/shm"
		}
	}
	return &Cache{Dir: dir, MaxAge: 8 * time.Hour}
}

// FilePath returns the cache file path for a given uid and opaque key.
// The uid prefix allows Clear(uid) to scope deletes to a single user.
func (c *Cache) FilePath(uid, key string) string {
	h := sha256.Sum256([]byte(uid + ":" + key))
	return filepath.Join(c.Dir, "renv-"+uid+"-"+hex.EncodeToString(h[:16])+".enc")
}

// Put encrypts data and writes it to the cache file (mode 0600).
// No-op when cache is disabled.
func (c *Cache) Put(uid, key, password string, data []byte) error {
	if c.Disabled {
		return nil
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("cache: generating salt: %w", err)
	}
	iv := make([]byte, ivLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return fmt.Errorf("cache: generating IV: %w", err)
	}

	dk := pbkdf2.Key([]byte(password), salt, iterations, keyLen, sha256.New)
	defer zeroBytes(dk)

	block, err := aes.NewCipher(dk)
	if err != nil {
		return fmt.Errorf("cache: creating cipher: %w", err)
	}
	padded := pkcs7Pad(data, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	path := c.FilePath(uid, key)
	slog.Debug("cache put", "path", path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerms)
	if err != nil {
		return fmt.Errorf("cache: opening file: %w", err)
	}
	defer f.Close()
	for _, chunk := range [][]byte{salt, iv, ciphertext} {
		if _, err := f.Write(chunk); err != nil {
			return fmt.Errorf("cache: writing data: %w", err)
		}
	}
	return nil
}

// Get decrypts and returns cached data, or (nil, nil) on cache miss / expiry / disabled.
func (c *Cache) Get(uid, key, password string) ([]byte, error) {
	if c.Disabled {
		return nil, nil
	}

	path := c.FilePath(uid, key)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		slog.Debug("cache miss (not found)", "path", path)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache: stat: %w", err)
	}
	if time.Since(fi.ModTime()) > c.MaxAge {
		slog.Debug("cache expired", "path", path, "age", time.Since(fi.ModTime()).Round(time.Second))
		os.Remove(path)
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cache: reading file: %w", err)
	}
	if len(raw) < saltLen+ivLen+aes.BlockSize {
		return nil, fmt.Errorf("cache: file too short")
	}

	salt := raw[:saltLen]
	iv := raw[saltLen : saltLen+ivLen]
	ciphertext := raw[saltLen+ivLen:]

	dk := pbkdf2.Key([]byte(password), salt, iterations, keyLen, sha256.New)
	defer zeroBytes(dk)

	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, fmt.Errorf("cache: creating cipher: %w", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("cache: ciphertext not block-aligned")
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)

	unpadded, err := pkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("cache: wrong password or corrupted data: %w", err)
	}
	return unpadded, nil
}

// Clear removes all renv-<uid>-*.enc files owned by uid in the cache dir.
func (c *Cache) Clear(uid string) error {
	matches, err := filepath.Glob(filepath.Join(c.Dir, "renv-"+uid+"-*.enc"))
	if err != nil {
		return fmt.Errorf("cache: glob: %w", err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cache: removing %s: %w", m, err)
		}
	}
	return nil
}

// Status returns the paths and ages of all renv cache files visible in the cache dir.
func (c *Cache) Status() (paths []string, ages []string, err error) {
	matches, err := filepath.Glob(filepath.Join(c.Dir, "renv-*.enc"))
	if err != nil {
		return nil, nil, fmt.Errorf("cache: glob: %w", err)
	}
	for _, m := range matches {
		fi, statErr := os.Stat(m)
		if statErr != nil {
			ages = append(ages, "unknown")
		} else {
			ages = append(ages, time.Since(fi.ModTime()).Round(time.Second).String())
		}
		paths = append(paths, m)
	}
	return paths, ages, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

func pkcs7Unpad(data []byte) ([]byte, error) {
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

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ZeroString overwrites a string's backing memory with zeros (best-effort in Go).
func ZeroString(s *string) {
	b := []byte(*s)
	zeroBytes(b)
	*s = strings.Repeat("\x00", len(*s))
}
