package up2k

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Snapshot represents the serialized registry state
type Snapshot struct {
	Sessions  map[string]*UploadSession `json:"sessions"`
	Timestamp time.Time                 `json:"timestamp"`
}

// SaveSnapshot saves the current registry state to disk (gzipped JSON)
func (r *Registry) SaveSnapshot() error {
	if r.config.SnapshotPath == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create snapshot
	snapshot := &Snapshot{
		Sessions:  r.sessions,
		Timestamp: time.Now(),
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first (atomic operation)
	tempPath := r.config.SnapshotPath + ".tmp"
	f, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Compress with gzip
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(data); err != nil {
		gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	// Atomic rename
	if err := os.Rename(tempPath, r.config.SnapshotPath); err != nil {
		return err
	}

	r.logger.Debug("snapshot saved", "path", r.config.SnapshotPath, "sessions", len(r.sessions))
	return nil
}

// LoadSnapshot restores registry state from snapshot
func (r *Registry) LoadSnapshot() error {
	if r.config.SnapshotPath == "" {
		return nil
	}

	f, err := os.Open(r.config.SnapshotPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Decompress
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	// Decode JSON
	var snapshot Snapshot
	if err := json.NewDecoder(gz).Decode(&snapshot); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate sessions and restore
	validSessions := 0
	for wark, session := range snapshot.Sessions {
		// Check if temp file still exists
		if _, err := os.Stat(session.TempPath); os.IsNotExist(err) {
			r.logger.Info("temp file missing, skipping session", "wark", wark, "path", session.TempPath)
			continue
		}

		// Restore session
		r.sessions[wark] = session
		validSessions++
	}

	r.logger.Info("snapshot loaded", "total", len(snapshot.Sessions), "valid", validSessions, "age", time.Since(snapshot.Timestamp))
	return nil
}

// snapshotLoop periodically saves snapshots
func (r *Registry) snapshotLoop() {
	ticker := time.NewTicker(r.config.SnapshotInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := r.SaveSnapshot(); err != nil {
			r.logger.Error("failed to save snapshot", "error", err)
		}
	}
}

// Close gracefully shuts down the registry and saves final snapshot
func (r *Registry) Close() error {
	r.logger.Info("shutting down registry")

	// Save final snapshot
	if err := r.SaveSnapshot(); err != nil {
		r.logger.Error("failed to save final snapshot", "error", err)
		return err
	}

	return nil
}
