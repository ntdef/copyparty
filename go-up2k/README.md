# up2k - Resumable Upload Protocol for Go

A production-ready Go module implementing the **up2k** resumable upload protocol, inspired by copyparty. Features chunk-based uploads, crash recovery, deduplication, and HTTP/2 support.

## Features

- ✅ **Resumable Uploads** - Resume interrupted uploads from where they left off
- ✅ **Chunk-Based** - Files split into chunks with individual SHA-256 verification
- ✅ **Crash Recovery** - Automatic snapshot persistence and restoration
- ✅ **Deduplication** - Content-based file deduplication using deterministic session IDs
- ✅ **Parallel Uploads** - Configurable concurrent chunk uploads (1-50+)
- ✅ **HTTP/2 Ready** - Optimized for HTTP/2 multiplexing
- ✅ **SQLite Integration** - Built-in database support with sqlc
- ✅ **Sparse Files** - Efficient storage on supporting filesystems
- ✅ **Thread-Safe** - Concurrent upload handling with proper locking

## Installation

```bash
go get github.com/yourusername/up2k
```

### Using with sqlc

1. Install sqlc:
```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

2. Generate database code:
```bash
cd go-up2k
sqlc generate
```

## Quick Start

### Server Setup

```go
package main

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yourusername/up2k"
)

func main() {
	// Open SQLite database
	db, _ := sql.Open("sqlite3", "./uploads.db")
	defer db.Close()

	// Create database wrapper
	database, _ := up2k.NewSQLCDatabase(db)

	// Configure up2k
	config := &up2k.Config{
		UploadDir:        "./uploads",
		WarkSalt:         "your-secret-salt-change-this",
		SessionTimeout:   24 * time.Hour,
		SnapshotPath:     "./uploads/.snapshot.json.gz",
		DB:               database,
	}

	// Create registry and handler
	registry, _ := up2k.NewRegistry(config)
	defer registry.Close()

	handler := up2k.NewHandler(registry)

	// Setup routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### Client Setup (JavaScript)

```javascript
// Include uploader.js
const uploader = new Up2kUploader('http://localhost:8080');

// Upload a file
const file = document.getElementById('fileInput').files[0];
const result = await uploader.uploadFile(file);
console.log('Upload result:', result);
```

## Configuration

### Config Options

```go
type Config struct {
	// Storage
	UploadDir string // Base directory for uploads
	TempDir   string // Directory for .PARTIAL files (optional)

	// Session management
	SessionTimeout   time.Duration // Abandon uploads after this (default: 24h)
	SnapshotInterval time.Duration // Snapshot frequency (default: 5min)
	SnapshotPath     string        // Snapshot file path

	// Security
	WarkSalt string // REQUIRED: Secret salt for session ID generation

	// Limits
	MaxConcurrentChunks int   // Max chunks per session (default: 50)
	MaxFileSize         int64 // Max file size (0 = unlimited)

	// Database
	DB Database // Database interface
}
```

### HTTP/2 Configuration

To enable HTTP/2 for optimal performance:

```go
srv := &http.Server{
	Addr:    ":8443",
	Handler: handler,
	TLSConfig: &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS12,
	},
}

// Enable HTTP/2
http2.ConfigureServer(srv, &http2.Server{
	MaxConcurrentStreams: 250,
})

srv.ListenAndServeTLS("cert.pem", "key.pem")
```

## API Endpoints

### POST /upload/handshake

Initializes or resumes an upload session.

**Request:**
```json
{
  "name": "video.mp4",
  "size": 104857600,
  "lmod": 1704067200,
  "hash": ["chunk1_sha256", "chunk2_sha256", ...]
}
```

**Response:**
```json
{
  "wark": "session_key_44chars",
  "dwrk": "deterministic_wark",
  "name": "video.mp4",
  "size": 104857600,
  "chunk_size": 4194304,
  "hash": ["chunk3_sha256", "chunk7_sha256"],
  "sprs": true
}
```

### POST /upload/chunk

Uploads one or more chunks.

**Headers:**
- `X-Upload-Wark`: Session key from handshake
- `X-Upload-Hash`: Comma-separated chunk hashes
- `Content-Length`: Total bytes

**Body:** Binary chunk data

**Response:**
```json
{
  "status": "ok",
  "remaining": 5,
  "completed": false
}
```

### GET /upload/status?wark=XXX

Get upload progress (optional).

**Response:**
```json
{
  "wark": "session_key",
  "filename": "video.mp4",
  "size": 104857600,
  "remaining": 5,
  "total": 256,
  "progress": 98.05
}
```

## Database Integration

### Using sqlc (Recommended)

The module includes sqlc configuration for type-safe database queries.

**Generate code:**
```bash
sqlc generate
```

**Use in your app:**
```go
db, _ := sql.Open("sqlite3", "./uploads.db")
database, _ := up2k.NewSQLCDatabase(db)
```

### Using Basic SQLite Wrapper

For simpler use cases:

```go
db, _ := sql.Open("sqlite3", "./uploads.db")
database, _ := up2k.NewSQLiteDB(db)
```

### Custom Database Implementation

Implement the `Database` interface:

```go
type Database interface {
	RecordUpload(wark, filename string, size int64, uploadedAt time.Time, username, ip string) error
	FindUploadByWark(wark string) (*UploadRecord, error)
	DeleteUpload(wark string) error
}
```

## How It Works

### Upload Flow

```
┌─────────────────────────────────────────────────┐
│  1. CLIENT: Hash file chunks (SHA-256)          │
│     - Split file into optimal chunk size        │
│     - Compute hash for each chunk               │
└─────────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│  2. HANDSHAKE: POST /upload/handshake           │
│     - Server generates wark (session ID)        │
│     - Server checks for deduplication           │
│     - Server returns missing chunks             │
└─────────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│  3. UPLOAD CHUNKS: POST /upload/chunk           │
│     - Upload missing chunks in parallel         │
│     - Server verifies hash of each chunk        │
│     - Server writes to .PARTIAL file            │
└─────────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│  4. VERIFY: POST /upload/handshake (again)      │
│     - Server returns empty hash array if done   │
│     - Server renames .PARTIAL → final file      │
│     - Server records in database                │
└─────────────────────────────────────────────────┘
```

### Chunk Size Calculation

Based on file size, optimized for 256 chunks (up to 4096 for very large files):

- **100 MB** → 1 MB chunks (100 chunks)
- **1 GB** → 4 MB chunks (256 chunks)
- **10 GB** → 40 MB chunks (256 chunks)
- **100 GB** → 32 MB chunks (3125 chunks, max 32 MB)

### Session ID (Wark) Generation

**Hash-based (deterministic):**
```
wark = base64url(sha256(salt + filesize + chunk_hash1 + chunk_hash2 + ...))
```

This enables content-based deduplication - identical files get the same wark.

**Metadata-based (for non-hashable clients):**
```
wark = "#" + base64url(sha256(salt + lastmod + size + filename + random))
```

### Crash Recovery

Snapshots are saved periodically (default: 5 minutes) as gzipped JSON:

```bash
./uploads/.snapshot.json.gz
```

On server restart, the registry automatically:
1. Loads the snapshot
2. Verifies .PARTIAL files still exist
3. Resumes active uploads

## Example Application

See the `example/` directory for a complete working example with:

- HTTP server setup
- Web UI with drag-and-drop
- Progress tracking
- Parallel uploads
- Pause/resume functionality

**Run the example:**
```bash
cd example
go run main.go
# Visit http://localhost:8080
```

## Performance Tips

### HTTP/1.1
- Use **2-4 parallel uploads** for optimal performance
- Max 6 concurrent connections (browser limit)

### HTTP/2
- Use **20-50 parallel uploads** for high-latency connections
- Single TCP connection with unlimited streams
- Significantly faster for cross-continental uploads

### Storage
- Enable sparse file support for out-of-order chunk uploads
- Use SSD for temp directory to minimize I/O latency
- Consider separate temp directory on different disk

## Security Considerations

1. **Wark Salt**: MUST be a random secret - prevents session ID prediction
2. **File Validation**: All chunks verified with SHA-256 before acceptance
3. **Path Sanitization**: Filenames sanitized to prevent directory traversal
4. **Rate Limiting**: Consider adding rate limiting per IP
5. **Authentication**: Add authentication middleware before handlers
6. **HTTPS**: Always use HTTPS in production

## Comparison to Similar Protocols

| Feature | up2k | tus.io | Resumable.js |
|---------|------|--------|--------------|
| Protocol | Custom | Standardized | Custom |
| Chunk hashing | SHA-256 | Optional | MD5 |
| Deduplication | Built-in | No | No |
| HTTP/2 | Optimized | Yes | Limited |
| Crash recovery | Yes | Extension | No |
| Browser support | All modern | All modern | All |

## License

MIT

## Contributing

Pull requests welcome! Please ensure:
- Tests pass
- Code is formatted (`go fmt`)
- Documentation updated

## Credits

Inspired by [copyparty](https://github.com/9001/copyparty) by @9001
