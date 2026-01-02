package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yourusername/up2k"
)

func main() {
	// Open SQLite database
	db, err := sql.Open("sqlite3", "./uploads.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create database wrapper (using sqlc)
	database, err := up2k.NewSQLCDatabase(db)
	if err != nil {
		log.Fatal(err)
	}

	// OR use the simple SQLite wrapper:
	// database, err := up2k.NewSQLiteDB(db)

	// Configure up2k
	config := &up2k.Config{
		UploadDir:        "./uploads",
		TempDir:          "./uploads/.tmp",
		SessionTimeout:   24 * time.Hour,
		SnapshotInterval: 5 * time.Minute,
		SnapshotPath:     "./uploads/.snapshot.json.gz",
		WarkSalt:         "change-this-to-a-random-secret-value",
		MaxConcurrentChunks: 50,
		MaxFileSize:      10 * 1024 * 1024 * 1024, // 10 GB
		DB:               database,
	}

	// Create registry
	registry, err := up2k.NewRegistry(config)
	if err != nil {
		log.Fatal(err)
	}
	defer registry.Close()

	// Create HTTP handler
	handler := up2k.NewHandler(registry)

	// Setup routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Serve static files (for the example client)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	// Start server
	server := &http.Server{
		Addr:         ":8080",
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Minute,  // Long timeout for large uploads
		WriteTimeout: 30 * time.Minute,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down server...")
		registry.Close()
		server.Close()
	}()

	log.Printf("Server started at http://localhost:8080")
	log.Printf("Upload endpoint: http://localhost:8080/upload/handshake")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// corsMiddleware adds CORS headers for development
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Upload-Wark, X-Upload-Hash, X-User-Name")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
