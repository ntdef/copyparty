package up2k

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Handler wraps the registry with HTTP handlers
type Handler struct {
	registry *Registry
	logger   Logger
}

// NewHandler creates a new HTTP handler
func NewHandler(registry *Registry) *Handler {
	return &Handler{
		registry: registry,
		logger:   registry.logger,
	}
}

// HandleHandshake processes upload initialization requests
// POST /upload/handshake
func (h *Handler) HandleHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req HandshakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode handshake request", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Extract user context (you may want to get this from middleware/auth)
	username := r.Header.Get("X-User-Name")
	clientIP := getClientIP(r)

	// Process handshake
	resp, err := h.registry.Handshake(&req, username, clientIP)
	if err != nil {
		h.logger.Error("handshake failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleChunkUpload processes chunk upload requests
// POST /upload/chunk
func (h *Handler) HandleChunkUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse headers
	wark := r.Header.Get("X-Upload-Wark")
	hashesStr := r.Header.Get("X-Upload-Hash")

	if wark == "" || hashesStr == "" {
		http.Error(w, "missing required headers: X-Upload-Wark, X-Upload-Hash", http.StatusBadRequest)
		return
	}

	// Parse chunk hashes (comma-separated)
	chunkHashes := strings.Split(hashesStr, ",")
	for i := range chunkHashes {
		chunkHashes[i] = strings.TrimSpace(chunkHashes[i])
	}

	// Reserve chunks
	session, offsets, err := h.registry.ReserveChunks(wark, chunkHashes)
	if err != nil {
		h.logger.Error("failed to reserve chunks", "error", err, "wark", wark)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Open temp file for writing
	file, err := os.OpenFile(session.TempPath, os.O_WRONLY, 0644)
	if err != nil {
		h.registry.ReleaseChunks(wark, chunkHashes)
		h.logger.Error("failed to open temp file", "error", err, "path", session.TempPath)
		http.Error(w, "failed to open temp file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Write each chunk
	writtenHashes := []string{}
	for i, chunkHash := range chunkHashes {
		offset := offsets[i]

		// Seek to position
		if _, err := file.Seek(offset, 0); err != nil {
			h.registry.ReleaseChunks(wark, chunkHashes)
			h.logger.Error("failed to seek in temp file", "error", err, "offset", offset)
			http.Error(w, "failed to seek in temp file", http.StatusInternalServerError)
			return
		}

		// Read chunk data from request body
		chunkSize := session.ChunkSize
		if offset+int64(chunkSize) > session.Size {
			// Last chunk may be smaller
			chunkSize = int(session.Size - offset)
		}

		chunkData := make([]byte, chunkSize)
		n, err := io.ReadFull(r.Body, chunkData)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			h.registry.ReleaseChunks(wark, chunkHashes)
			h.logger.Error("failed to read chunk data", "error", err)
			http.Error(w, "failed to read chunk data", http.StatusBadRequest)
			return
		}
		chunkData = chunkData[:n]

		// Verify hash
		if !VerifyChunkHash(chunkData, chunkHash) {
			h.registry.ReleaseChunks(wark, chunkHashes)
			h.logger.Error("chunk hash mismatch", "expected", chunkHash, "offset", offset)
			http.Error(w, fmt.Sprintf("chunk hash mismatch at offset %d", offset), http.StatusBadRequest)
			return
		}

		// Write to file
		if _, err := file.Write(chunkData); err != nil {
			h.registry.ReleaseChunks(wark, chunkHashes)
			h.logger.Error("failed to write chunk", "error", err, "offset", offset)
			http.Error(w, "failed to write chunk", http.StatusInternalServerError)
			return
		}

		writtenHashes = append(writtenHashes, chunkHash)
		h.logger.Debug("wrote chunk", "wark", wark, "hash", chunkHash[:10], "offset", offset, "size", n)
	}

	// Confirm chunks
	remaining, err := h.registry.ConfirmChunks(wark, writtenHashes)
	if err != nil {
		h.logger.Error("failed to confirm chunks", "error", err)
		http.Error(w, "failed to confirm chunks", http.StatusInternalServerError)
		return
	}

	// Send response
	response := map[string]interface{}{
		"status":    "ok",
		"remaining": remaining,
		"completed": remaining == 0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	if remaining == 0 {
		h.logger.Info("upload completed", "wark", wark, "filename", session.Filename)
	}
}

// HandleStatus returns upload session status (optional helper)
// GET /upload/status?wark=XXX
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wark := r.URL.Query().Get("wark")
	if wark == "" {
		http.Error(w, "missing wark parameter", http.StatusBadRequest)
		return
	}

	session, err := h.registry.GetSession(wark)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"wark":      session.Wark,
		"filename":  session.Filename,
		"size":      session.Size,
		"remaining": len(session.Need),
		"total":     len(session.Hashes),
		"progress":  float64(len(session.Hashes)-len(session.Need)) / float64(len(session.Hashes)) * 100,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes registers all handlers with a mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/upload/handshake", h.HandleHandshake)
	mux.HandleFunc("/upload/chunk", h.HandleChunkUpload)
	mux.HandleFunc("/upload/status", h.HandleStatus)
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
