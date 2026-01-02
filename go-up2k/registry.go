package up2k

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrSessionNotFound   = errors.New("upload session not found")
	ErrChunkAlreadyBusy  = errors.New("chunk is already being written")
	ErrChunkNotNeeded    = errors.New("chunk not needed (already uploaded)")
	ErrInvalidChunkHash  = errors.New("invalid chunk hash")
	ErrFileSizeMismatch  = errors.New("file size mismatch")
)

// NewRegistry creates a new upload registry
func NewRegistry(config *Config) (*Registry, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	// Set defaults
	if config.SessionTimeout == 0 {
		config.SessionTimeout = 24 * time.Hour
	}
	if config.SnapshotInterval == 0 {
		config.SnapshotInterval = 5 * time.Minute
	}
	if config.MaxConcurrentChunks == 0 {
		config.MaxConcurrentChunks = 50
	}
	if config.WarkSalt == "" {
		return nil, errors.New("wark salt is required for security")
	}
	if config.UploadDir == "" {
		return nil, errors.New("upload directory is required")
	}
	if config.TempDir == "" {
		config.TempDir = config.UploadDir
	}

	// Create directories
	if err := os.MkdirAll(config.UploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload dir: %w", err)
	}
	if err := os.MkdirAll(config.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	r := &Registry{
		sessions: make(map[string]*UploadSession),
		config:   config,
		logger:   &defaultLogger{},
	}

	// Load snapshots if they exist
	if config.SnapshotPath != "" {
		if err := r.LoadSnapshot(); err != nil && !os.IsNotExist(err) {
			r.logger.Error("failed to load snapshot", "error", err)
		}
	}

	// Start background tasks
	go r.cleanupLoop()
	if config.SnapshotPath != "" {
		go r.snapshotLoop()
	}

	return r, nil
}

// SetLogger sets a custom logger
func (r *Registry) SetLogger(logger Logger) {
	r.logger = logger
}

// Handshake initializes or resumes an upload session
func (r *Registry) Handshake(req *HandshakeRequest, username, clientIP string) (*HandshakeResponse, error) {
	if req.Size > r.config.MaxFileSize && r.config.MaxFileSize > 0 {
		return nil, fmt.Errorf("file too large: %d > %d", req.Size, r.config.MaxFileSize)
	}

	// Calculate chunk size
	chunkSize := CalculateChunkSize(req.Size)

	// Generate wark
	var wark, dwrk string
	if len(req.Hashes) > 0 {
		// Hash-based (deterministic)
		wark = GenerateWark(r.config.WarkSalt, req.Size, req.Hashes)
		dwrk = wark
	} else {
		// Metadata-based (random)
		wark = GenerateRandomWark(r.config.WarkSalt, req.Size, req.LastMod, req.Filename)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if session already exists
	if session, exists := r.sessions[wark]; exists {
		// Resume existing session
		session.LastPoke = time.Now()
		return r.buildHandshakeResponse(session), nil
	}

	// Check if file was already uploaded (deduplication)
	if r.config.DB != nil && dwrk != "" {
		if record, err := r.config.DB.FindUploadByWark(dwrk); err == nil {
			r.logger.Info("file already uploaded (deduplication)", "wark", dwrk, "filename", record.Filename)
			// File exists, return empty need list
			return &HandshakeResponse{
				Wark:      wark,
				Dwrk:      dwrk,
				Filename:  req.Filename,
				Size:      req.Size,
				ChunkSize: chunkSize,
				Hashes:    []string{}, // Empty = already complete
				Sparse:    checkSparseSupport(r.config.TempDir),
			}, nil
		}
	}

	// Create new session
	tempPath := filepath.Join(r.config.TempDir, "."+sanitizeFilename(req.Filename)+".PARTIAL")
	finalPath := filepath.Join(r.config.UploadDir, sanitizeFilename(req.Filename))

	session := &UploadSession{
		Wark:           wark,
		Dwrk:           dwrk,
		Filename:       req.Filename,
		Size:           req.Size,
		LastMod:        time.Unix(req.LastMod, 0),
		ChunkSize:      chunkSize,
		Hashes:         req.Hashes,
		Need:           make([]string, len(req.Hashes)),
		Busy:           make(map[string]bool),
		LastPoke:       time.Now(),
		CreatedAt:      time.Now(),
		TempPath:       tempPath,
		FinalPath:      finalPath,
		SupportsSparse: checkSparseSupport(r.config.TempDir),
		Username:       username,
		ClientIP:       clientIP,
	}

	// Initially, all chunks are needed
	copy(session.Need, session.Hashes)

	// Pre-allocate temp file
	if err := r.preallocateFile(session); err != nil {
		return nil, fmt.Errorf("failed to preallocate file: %w", err)
	}

	r.sessions[wark] = session

	r.logger.Info("new upload session", "wark", wark, "filename", req.Filename, "size", req.Size)

	return r.buildHandshakeResponse(session), nil
}

// ReserveChunks marks chunks as busy before upload
func (r *Registry) ReserveChunks(wark string, chunkHashes []string) (*UploadSession, []int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[wark]
	if !exists {
		return nil, nil, ErrSessionNotFound
	}

	// Validate and reserve chunks
	offsets := make([]int64, 0, len(chunkHashes))

	for _, hash := range chunkHashes {
		// Check if chunk is needed
		if !contains(session.Need, hash) {
			return nil, nil, ErrChunkNotNeeded
		}

		// Check if chunk is already being written
		if session.Busy[hash] {
			return nil, nil, ErrChunkAlreadyBusy
		}

		// Find chunk index
		chunkIndex := indexOf(session.Hashes, hash)
		if chunkIndex < 0 {
			return nil, nil, ErrInvalidChunkHash
		}

		// Calculate offset
		offset := int64(chunkIndex) * int64(session.ChunkSize)
		offsets = append(offsets, offset)

		// Mark as busy
		session.Busy[hash] = true
	}

	session.LastPoke = time.Now()

	return session, offsets, nil
}

// ConfirmChunks marks chunks as successfully written
func (r *Registry) ConfirmChunks(wark string, chunkHashes []string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[wark]
	if !exists {
		return 0, ErrSessionNotFound
	}

	for _, hash := range chunkHashes {
		// Remove from busy
		delete(session.Busy, hash)

		// Remove from need
		session.Need = removeString(session.Need, hash)
	}

	session.LastPoke = time.Now()

	remaining := len(session.Need)

	// If all chunks complete, finalize
	if remaining == 0 {
		if err := r.finalizeUpload(session); err != nil {
			return 0, fmt.Errorf("failed to finalize upload: %w", err)
		}
		delete(r.sessions, wark)
	}

	return remaining, nil
}

// ReleaseChunks releases busy chunks (on error)
func (r *Registry) ReleaseChunks(wark string, chunkHashes []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[wark]
	if !exists {
		return ErrSessionNotFound
	}

	for _, hash := range chunkHashes {
		delete(session.Busy, hash)
	}

	return nil
}

// GetSession returns a copy of the session (thread-safe)
func (r *Registry) GetSession(wark string) (*UploadSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[wark]
	if !exists {
		return nil, ErrSessionNotFound
	}

	// Return a copy to prevent external modification
	sessionCopy := *session
	return &sessionCopy, nil
}

// preallocateFile creates the temporary file with proper size
func (r *Registry) preallocateFile(session *UploadSession) error {
	f, err := os.Create(session.TempPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Truncate to full size (creates sparse file on supporting filesystems)
	if err := f.Truncate(session.Size); err != nil {
		return err
	}

	return nil
}

// finalizeUpload moves temp file to final location
func (r *Registry) finalizeUpload(session *UploadSession) error {
	// Rename .PARTIAL to final filename
	if err := os.Rename(session.TempPath, session.FinalPath); err != nil {
		return err
	}

	// Set modification time
	if !session.LastMod.IsZero() {
		os.Chtimes(session.FinalPath, session.LastMod, session.LastMod)
	}

	// Record in database
	if r.config.DB != nil {
		err := r.config.DB.RecordUpload(
			session.Wark,
			session.Filename,
			session.Size,
			time.Now(),
			session.Username,
			session.ClientIP,
		)
		if err != nil {
			r.logger.Error("failed to record upload in db", "error", err)
		}
	}

	r.logger.Info("upload completed", "wark", session.Wark, "filename", session.Filename)

	return nil
}

// buildHandshakeResponse creates response from session
func (r *Registry) buildHandshakeResponse(session *UploadSession) *HandshakeResponse {
	return &HandshakeResponse{
		Wark:      session.Wark,
		Dwrk:      session.Dwrk,
		Filename:  session.Filename,
		Size:      session.Size,
		ChunkSize: session.ChunkSize,
		Hashes:    session.Need, // Return missing chunks
		Sparse:    session.SupportsSparse,
	}
}

// cleanupLoop removes abandoned sessions
func (r *Registry) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.cleanupAbandoned()
	}
}

// cleanupAbandoned removes old sessions
func (r *Registry) cleanupAbandoned() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for wark, session := range r.sessions {
		if now.Sub(session.LastPoke) > r.config.SessionTimeout {
			// Remove temp file
			os.Remove(session.TempPath)

			delete(r.sessions, wark)
			r.logger.Info("cleaned up abandoned session", "wark", wark, "age", now.Sub(session.LastPoke))
		}
	}
}

// Helper functions

func checkSparseSupport(dir string) bool {
	// Simple check: assume Unix-like systems support sparse files
	// On Windows, NTFS supports sparse files
	// TODO: Implement actual sparse file detection
	return true
}

func sanitizeFilename(name string) string {
	// Remove path separators and dangerous characters
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	return name
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
