package bitwarden

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
	cacheKeyLen    = 32
	cacheSaltLen   = 32
	cacheIVLen     = 16
	cacheIter      = 100000
	cacheFilePerms = 0600
)

// Cache stores encrypted BW folder item JSON on disk (RAM-backed when /dev/shm available).
// Key derivation: PBKDF2-SHA256(masterPassword, salt, 100000 iter, 32 bytes)
// Encryption: AES-256-CBC
type Cache struct {
	Dir      string
	MaxAge   time.Duration
	Disabled bool // when true, Put and Get are no-ops (--no-cache flag)
}

// NewCache picks /dev/shm if available and writable, else /tmp.
func NewCache() *Cache {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		// Test writability
		testFile := filepath.Join("/dev/shm", ".renv-test")
		if f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(testFile)
			dir = "/dev/shm"
		}
	}
	return &Cache{Dir: dir, MaxAge: 8 * time.Hour}
}

// cacheKey returns the hex-encoded first 16 bytes of SHA-256(uid:acctTag:folder).
func cacheKey(uid, acctTag, folder string) string {
	h := sha256.Sum256([]byte(uid + ":" + acctTag + ":" + folder))
	return hex.EncodeToString(h[:16])
}

// CacheFile returns the path for a given uid, acctTag, and folder.
// The filename includes a uid prefix so Clear(uid) can scope deletes.
func (c *Cache) CacheFile(uid, acctTag, folder string) string {
	return filepath.Join(c.Dir, "renv-"+uid+"-"+cacheKey(uid, acctTag, folder)+".enc")
}

// Put encrypts items JSON and writes to cache file (chmod 600).
// Returns immediately (no-op) when cache is disabled.
func (c *Cache) Put(uid, acctTag, folder, masterPassword string, items []byte) error {
	if c.Disabled {
		return nil
	}
	// Generate random salt and IV
	salt := make([]byte, cacheSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("cache: generating salt: %w", err)
	}
	iv := make([]byte, cacheIVLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return fmt.Errorf("cache: generating IV: %w", err)
	}

	// Derive key
	key := pbkdf2.Key([]byte(masterPassword), salt, cacheIter, cacheKeyLen, sha256.New)
	defer zeroBytes(key)

	// Encrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("cache: creating cipher: %w", err)
	}
	padded := pkcs7Pad(items, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	// Write: [32-byte salt][16-byte IV][ciphertext]
	path := c.CacheFile(uid, acctTag, folder)
	slog.Debug("cache put", "path", path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, cacheFilePerms)
	if err != nil {
		return fmt.Errorf("cache: opening file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(salt); err != nil {
		return fmt.Errorf("cache: writing salt: %w", err)
	}
	if _, err := f.Write(iv); err != nil {
		return fmt.Errorf("cache: writing IV: %w", err)
	}
	if _, err := f.Write(ciphertext); err != nil {
		return fmt.Errorf("cache: writing ciphertext: %w", err)
	}
	return nil
}

// Get decrypts and returns items JSON, or (nil, nil) if cache miss/expired/disabled.
func (c *Cache) Get(uid, acctTag, folder, masterPassword string) ([]byte, error) {
	if c.Disabled {
		return nil, nil // cache disabled — treat as miss
	}
	path := c.CacheFile(uid, acctTag, folder)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		slog.Debug("cache miss (not found)", "path", path)
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("cache: stat: %w", err)
	}
	if time.Since(fi.ModTime()) > c.MaxAge {
		slog.Debug("cache expired", "path", path, "age", time.Since(fi.ModTime()).Round(time.Second))
		os.Remove(path)
		return nil, nil // expired
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cache: reading file: %w", err)
	}
	if len(data) < cacheSaltLen+cacheIVLen+aes.BlockSize {
		return nil, fmt.Errorf("cache: file too short")
	}

	salt := data[:cacheSaltLen]
	iv := data[cacheSaltLen : cacheSaltLen+cacheIVLen]
	ciphertext := data[cacheSaltLen+cacheIVLen:]

	// Derive key
	key := pbkdf2.Key([]byte(masterPassword), salt, cacheIter, cacheKeyLen, sha256.New)
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
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
	pattern := filepath.Join(c.Dir, "renv-"+uid+"-*.enc")
	matches, err := filepath.Glob(pattern)
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

// CacheStatus returns the paths and ages of all renv cache files for any uid.
func CacheStatus(c *Cache) ([]string, []string, error) {
	pattern := filepath.Join(c.Dir, "renv-*.enc")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("cache: glob: %w", err)
	}
	var ages []string
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			ages = append(ages, "unknown")
			continue
		}
		ages = append(ages, time.Since(fi.ModTime()).Round(time.Second).String())
	}
	return matches, ages, nil
}

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

// pkcs7Unpad removes PKCS#7 padding.
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

// zeroBytes overwrites a byte slice with zeros.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// zeroString overwrites a string's backing memory with zeros.
// NOTE: This is best-effort in Go — strings are immutable values.
func zeroString(s *string) {
	b := []byte(*s)
	zeroBytes(b)
	*s = strings.Repeat("\x00", len(*s))
}
