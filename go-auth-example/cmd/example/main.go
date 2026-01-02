package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/example/go-auth-system/internal/auth"
)

func main() {
	ctx := context.Background()

	// Initialize auth service
	service, err := auth.NewService("auth.db")
	if err != nil {
		log.Fatalf("Failed to create auth service: %v", err)
	}
	defer service.Close()

	// Setup example data
	if err := setupExampleData(ctx, service); err != nil {
		log.Fatalf("Failed to setup example data: %v", err)
	}

	// Create middleware
	middleware := auth.NewMiddleware(service)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/login", middleware.LoginHandler())
	mux.HandleFunc("/logout", middleware.LogoutHandler())

	// Protected endpoint - requires authentication only
	mux.Handle("/profile", middleware.AuthenticateRequest(
		middleware.RequireAuth(
			http.HandlerFunc(profileHandler),
		),
	))

	// Protected endpoint - requires read permission on "documents" volume
	mux.Handle("/documents/list", middleware.AuthenticateRequest(
		middleware.RequireRead("documents")(
			http.HandlerFunc(listDocumentsHandler),
		),
	))

	// Protected endpoint - requires write permission on "documents" volume
	mux.Handle("/documents/upload", middleware.AuthenticateRequest(
		middleware.RequireWrite("documents")(
			http.HandlerFunc(uploadDocumentHandler),
		),
	))

	// Protected endpoint - requires admin permission on "documents" volume
	mux.Handle("/documents/admin", middleware.AuthenticateRequest(
		middleware.RequireAdmin("documents")(
			http.HandlerFunc(adminHandler),
		),
	))

	// Start server
	log.Println("Server starting on :8080")
	log.Println("\nExample users:")
	log.Println("  - alice:password123 (admin on documents)")
	log.Println("  - bob:password456 (read/write on documents)")
	log.Println("  - charlie:password789 (read-only on documents)")
	log.Println("\nEndpoints:")
	log.Println("  - POST /login (username=alice&password=password123)")
	log.Println("  - GET /logout")
	log.Println("  - GET /profile (requires auth)")
	log.Println("  - GET /documents/list (requires read)")
	log.Println("  - POST /documents/upload (requires write)")
	log.Println("  - GET /documents/admin (requires admin)")
	log.Println("\nTest with curl:")
	log.Println("  curl -X POST http://localhost:8080/login -d 'username=alice&password=password123' -c cookies.txt")
	log.Println("  curl http://localhost:8080/profile -b cookies.txt")
	log.Println("  curl http://localhost:8080/documents/list -b cookies.txt")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func setupExampleData(ctx context.Context, service *auth.Service) error {
	log.Println("Setting up example data...")

	// Create users
	users := []struct {
		username string
		password string
	}{
		{"alice", "password123"},
		{"bob", "password456"},
		{"charlie", "password789"},
	}

	for _, u := range users {
		err := service.CreateUser(ctx, u.username, u.password)
		if err != nil && err != auth.ErrUserExists {
			return fmt.Errorf("failed to create user %s: %w", u.username, err)
		}
	}

	// Create groups
	groups := []string{"admins", "editors", "viewers"}
	for _, g := range groups {
		err := service.CreateGroup(ctx, g)
		if err != nil && err != auth.ErrGroupExists {
			return fmt.Errorf("failed to create group %s: %w", g, err)
		}
	}

	// Add users to groups
	service.AddUserToGroup(ctx, "alice", "admins")
	service.AddUserToGroup(ctx, "bob", "editors")
	service.AddUserToGroup(ctx, "charlie", "viewers")

	// Create volumes
	volumes := []struct {
		name string
		path string
	}{
		{"documents", "/data/documents"},
		{"photos", "/data/photos"},
		{"videos", "/data/videos"},
	}

	for _, v := range volumes {
		err := service.CreateVolume(ctx, v.name, v.path)
		if err != nil && err != auth.ErrVolumeExists {
			return fmt.Errorf("failed to create volume %s: %w", v.name, err)
		}
	}

	// Set permissions
	// Alice - admin on documents
	service.SetUserPermissions(ctx, "alice", "documents", auth.Permissions{
		CanRead:   true,
		CanWrite:  true,
		CanMove:   true,
		CanDelete: true,
		CanAdmin:  true,
		CanGet:    true,
		CanDot:    true,
	})

	// Bob - read/write on documents
	service.SetUserPermissions(ctx, "bob", "documents", auth.Permissions{
		CanRead:  true,
		CanWrite: true,
		CanGet:   true,
	})

	// Charlie - read-only on documents
	service.SetUserPermissions(ctx, "charlie", "documents", auth.Permissions{
		CanRead: true,
		CanGet:  true,
	})

	// Set group permissions
	// Admins group - full access to photos
	service.SetGroupPermissions(ctx, "admins", "photos", auth.Permissions{
		CanRead:   true,
		CanWrite:  true,
		CanMove:   true,
		CanDelete: true,
		CanAdmin:  true,
		CanGet:    true,
		CanDot:    true,
	})

	// Editors group - read/write to videos
	service.SetGroupPermissions(ctx, "editors", "videos", auth.Permissions{
		CanRead:  true,
		CanWrite: true,
		CanGet:   true,
	})

	// Viewers group - read-only to videos
	service.SetGroupPermissions(ctx, "viewers", "videos", auth.Permissions{
		CanRead: true,
		CanGet:  true,
	})

	log.Println("Example data setup complete!")
	return nil
}

func profileHandler(w http.ResponseWriter, r *http.Request) {
	username, ok := auth.GetUsername(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Profile for user: %s\n", username)
	fmt.Fprintf(w, "You are authenticated!\n")
}

func listDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	username, _ := auth.GetUsername(r)
	perms, _ := auth.GetPermissions(r)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Documents list for user: %s\n\n", username)
	fmt.Fprintf(w, "Your permissions:\n")
	fmt.Fprintf(w, "  - Read: %v\n", perms.CanRead)
	fmt.Fprintf(w, "  - Write: %v\n", perms.CanWrite)
	fmt.Fprintf(w, "  - Move: %v\n", perms.CanMove)
	fmt.Fprintf(w, "  - Delete: %v\n", perms.CanDelete)
	fmt.Fprintf(w, "  - Admin: %v\n", perms.CanAdmin)
	fmt.Fprintf(w, "\nDocuments:\n")
	fmt.Fprintf(w, "  1. report.pdf\n")
	fmt.Fprintf(w, "  2. presentation.pptx\n")
	fmt.Fprintf(w, "  3. data.xlsx\n")
}

func uploadDocumentHandler(w http.ResponseWriter, r *http.Request) {
	username, _ := auth.GetUsername(r)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Upload endpoint for user: %s\n", username)
	fmt.Fprintf(w, "You have write permission!\n")
	fmt.Fprintf(w, "File upload would happen here...\n")
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	username, _ := auth.GetUsername(r)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Admin panel for user: %s\n", username)
	fmt.Fprintf(w, "You have admin permission!\n")
	fmt.Fprintf(w, "\nAdmin actions:\n")
	fmt.Fprintf(w, "  - View all users\n")
	fmt.Fprintf(w, "  - Manage permissions\n")
	fmt.Fprintf(w, "  - View logs\n")
}
