package up2k

import (
	"sync"
	"time"
)

// UploadSession represents an active upload session
type UploadSession struct {
	// Session identifier (44-char base64url)
	Wark string `json:"wark"`

	// Deterministic wark (hash-based, if available)
	Dwrk string `json:"dwrk,omitempty"`

	// File metadata
	Filename  string    `json:"name"`
	Size      int64     `json:"size"`
	LastMod   time.Time `json:"lmod"`
	ChunkSize int       `json:"chunk_size"`

	// Hashing
	Hashes []string `json:"hash"` // All chunk hashes (SHA-256)
	Need   []string `json:"need"` // Missing chunks

	// Upload state
	Busy      map[string]bool `json:"busy"`       // Currently writing chunks
	LastPoke  time.Time       `json:"poke"`       // Last activity
	CreatedAt time.Time       `json:"t0"`         // Session start

	// File paths
	TempPath  string `json:"tnam"`  // Temporary .PARTIAL file
	FinalPath string `json:"fpath"` // Final destination

	// Filesystem
	SupportsSparse bool `json:"sprs"` // Sparse file support

	// User context
	Username string `json:"user,omitempty"`
	ClientIP string `json:"addr,omitempty"`
}

// HandshakeRequest is the initial upload request from client
type HandshakeRequest struct {
	Filename string   `json:"name"`
	Size     int64    `json:"size"`
	LastMod  int64    `json:"lmod,omitempty"` // Unix timestamp
	Hashes   []string `json:"hash,omitempty"` // SHA-256 chunk hashes
}

// HandshakeResponse is sent back to client
type HandshakeResponse struct {
	Wark      string   `json:"wark"`            // Session key
	Dwrk      string   `json:"dwrk,omitempty"`  // Deterministic wark
	Filename  string   `json:"name"`
	Size      int64    `json:"size"`
	ChunkSize int      `json:"chunk_size"`
	Hashes    []string `json:"hash"`            // Missing chunks
	Sparse    bool     `json:"sprs"`            // Sparse file support
}

// ChunkUploadRequest represents chunk upload metadata
type ChunkUploadRequest struct {
	Wark   string   // From X-Upload-Wark header
	Hashes []string // From X-Upload-Hash header (comma-separated)
}

// Config holds server configuration
type Config struct {
	// Storage
	UploadDir string // Base directory for uploads
	TempDir   string // Directory for .PARTIAL files (default: same as UploadDir)

	// Session management
	SessionTimeout time.Duration // Abandon uploads after this (default: 24h)
	SnapshotInterval time.Duration // Snapshot frequency (default: 5min)
	SnapshotPath   string        // Where to save snapshots

	// Security
	WarkSalt string // Salt for wark generation (keep secret!)

	// Limits
	MaxConcurrentChunks int   // Max chunks per session (default: 50)
	MaxFileSize         int64 // Max file size (0 = unlimited)

	// Database
	DB Database // Database interface for persistence
}

// Database interface for storage operations
type Database interface {
	// RecordUpload stores completed upload metadata
	RecordUpload(wark, filename string, size int64, uploadedAt time.Time, username, ip string) error

	// FindUploadByWark checks if upload with wark already exists
	FindUploadByWark(wark string) (*UploadRecord, error)

	// DeleteUpload removes upload record
	DeleteUpload(wark string) error
}

// UploadRecord represents a completed upload in the database
type UploadRecord struct {
	Wark       string
	Filename   string
	Size       int64
	UploadedAt time.Time
	Username   string
	ClientIP   string
}

// Registry manages all active upload sessions
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]*UploadSession // wark -> session

	config *Config
	logger Logger
}

// Logger interface for logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// defaultLogger is a simple stdout logger
type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, kv ...interface{})  { logPrint("INFO", msg, kv...) }
func (l *defaultLogger) Error(msg string, kv ...interface{}) { logPrint("ERROR", msg, kv...) }
func (l *defaultLogger) Debug(msg string, kv ...interface{}) { logPrint("DEBUG", msg, kv...) }

func logPrint(level, msg string, kv ...interface{}) {
	print := "[" + level + "] " + msg
	for i := 0; i < len(kv); i += 2 {
		if i+1 < len(kv) {
			print += " " + kv[i].(string) + "=" + toString(kv[i+1])
		}
	}
	println(print)
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int64, int32:
		return string(rune(val.(int)))
	default:
		return ""
	}
}
