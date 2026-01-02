package up2k

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math"
)

// GenerateWark creates a deterministic session ID from file metadata and hashes
func GenerateWark(salt string, fileSize int64, hashes []string) string {
	h := sha256.New()

	// Salt (prevents prediction)
	h.Write([]byte(salt))

	// File size
	h.Write([]byte(fmt.Sprintf("%d", fileSize)))

	// All chunk hashes
	for _, hash := range hashes {
		h.Write([]byte(hash))
	}

	// Take first 33 bytes and encode as base64url
	sum := h.Sum(nil)[:33]
	wark := base64.RawURLEncoding.EncodeToString(sum)

	// Truncate to 44 chars (should already be ~44)
	if len(wark) > 44 {
		wark = wark[:44]
	}

	return wark
}

// GenerateRandomWark creates a random session ID for non-hashable uploads
// (e.g., when client can't compute hashes due to browser limitations)
func GenerateRandomWark(salt string, fileSize int64, lastMod int64, filename string) string {
	h := sha256.New()

	// Salt
	h.Write([]byte(salt))

	// Metadata
	h.Write([]byte(fmt.Sprintf("%d\n%d\n%s", fileSize, lastMod, filename)))

	// Add randomness
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	h.Write(randBytes)

	sum := h.Sum(nil)[:33]
	wark := "#" + base64.RawURLEncoding.EncodeToString(sum)[:43]

	return wark
}

// CalculateChunkSize determines optimal chunk size for a file
// Based on copyparty's algorithm: target 256 chunks, max 32 MB chunks
func CalculateChunkSize(fileSize int64) int {
	const (
		minChunkSize = 1024 * 1024       // 1 MB
		targetChunks = 256
		maxChunkSize = 32 * 1024 * 1024  // 32 MB
		maxChunks    = 4096
		stepSize     = 512 * 1024        // 512 KB
	)

	if fileSize <= minChunkSize {
		return int(fileSize)
	}

	chunkSize := minChunkSize
	step := stepSize
	multiplier := 1

	for {
		numChunks := int(math.Ceil(float64(fileSize) / float64(chunkSize)))

		// Target: 256 chunks, or up to 4096 if chunks >= 32 MB
		if numChunks <= targetChunks {
			return chunkSize
		}

		if chunkSize >= maxChunkSize && numChunks <= maxChunks {
			return chunkSize
		}

		// Increase chunk size
		chunkSize += step

		// Accelerate growth: 512K, 512K, 1M, 2M, 4M, ...
		if multiplier < 8 {
			multiplier++
			if multiplier > 1 {
				step = stepSize * multiplier
			}
		}

		// Safety cap
		if chunkSize > maxChunkSize {
			return maxChunkSize
		}
	}
}

// HashChunk computes SHA-256 hash of a chunk
func HashChunk(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// HashReader computes SHA-256 hash while reading from io.Reader
func HashReader(r io.Reader, size int64) (string, error) {
	h := sha256.New()
	_, err := io.CopyN(h, r, size)
	if err != nil && err != io.EOF {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChunkHash verifies chunk data against expected hash
func VerifyChunkHash(data []byte, expectedHash string) bool {
	actualHash := HashChunk(data)
	return actualHash == expectedHash
}
