package thumbnail

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Cache manages thumbnail caching using hash-based paths
type Cache struct {
	baseDir   string
	dirHashes map[string]string
	mu        sync.RWMutex
}

// NewCache creates a new cache instance
func NewCache(baseDir string) *Cache {
	return &Cache{
		baseDir:   baseDir,
		dirHashes: make(map[string]string),
	}
}

// hashPath computes SHA512 hash and returns base64-url encoded string
func hashPath(s string) string {
	h := sha512.Sum512([]byte(s))
	encoded := base64.URLEncoding.EncodeToString(h[:])
	// Remove padding and take first 24 characters
	encoded = strings.TrimRight(encoded, "=")
	if len(encoded) > 24 {
		encoded = encoded[:24]
	}
	return encoded
}

// getDirHash returns the cached directory hash or computes it
func (c *Cache) getDirHash(dir string) string {
	c.mu.RLock()
	if hash, ok := c.dirHashes[dir]; ok {
		c.mu.RUnlock()
		return hash
	}
	c.mu.RUnlock()

	// Compute hash
	hash := hashPath(dir)

	// Cache it (with size limit)
	c.mu.Lock()
	if len(c.dirHashes) > 9000 {
		// Clear cache if it gets too large
		c.dirHashes = make(map[string]string)
	}
	c.dirHashes[dir] = hash
	c.mu.Unlock()

	return hash
}

// GetCachePath computes the cache path for a file
// Format: {baseDir}/{d1}/{d2}/{dirHash}{fileHash}.{mtime}.{ext}
func (c *Cache) GetCachePath(filePath string, mtime time.Time, format OutputFormat) string {
	dir := filepath.Dir(filePath)
	filename := filepath.Base(filePath)

	// Hash directory and filename
	dirHash := c.getDirHash(dir)
	fileHash := hashPath(filename)

	// Create directory structure: ab/cd/abcdef...
	d1 := dirHash[:2]
	d2 := dirHash[2:4]
	restDir := dirHash[4:]

	// Create filename: {restDir}{fileHash}.{mtime}.{ext}
	mtimeStr := fmt.Sprintf("%x", mtime.Unix())
	cacheFilename := fmt.Sprintf("%s%s.%s%s", restDir, fileHash, mtimeStr, format.Extension())

	return filepath.Join(c.baseDir, d1, d2, cacheFilename)
}

// Exists checks if a cached thumbnail exists and is valid
func (c *Cache) Exists(cachePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	// Check if it's a valid file (not empty, which marks a failed conversion)
	return info.Size() > 0
}

// EnsureDir creates the cache directory structure
func (c *Cache) EnsureDir(cachePath string) error {
	dir := filepath.Dir(cachePath)
	return os.MkdirAll(dir, 0755)
}

// CleanOldThumbnails removes thumbnails older than maxAge
func (c *Cache) CleanOldThumbnails(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	return filepath.Walk(c.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			// Remove old thumbnail
			os.Remove(path)
		}
		return nil
	})
}

// atomicWrite writes content to a file atomically using a temp file
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	tmpFile = nil // Prevent cleanup
	return nil
}
