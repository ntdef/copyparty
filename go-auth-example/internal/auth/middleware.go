package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// ContextKeyUsername is the context key for the authenticated username
	ContextKeyUsername contextKey = "username"
	// ContextKeyPermissions is the context key for user permissions
	ContextKeyPermissions contextKey = "permissions"
)

// Middleware provides HTTP middleware for authentication and authorization
type Middleware struct {
	service *Service
}

// NewMiddleware creates a new authentication middleware
func NewMiddleware(service *Service) *Middleware {
	return &Middleware{
		service: service,
	}
}

// AuthenticateRequest extracts and validates authentication from the request
// It checks (in order):
// 1. Cookie (session_id)
// 2. Authorization header (Bearer token / session ID)
// 3. Basic auth (username:password)
// 4. Query parameter (?pw=)
func (m *Middleware) AuthenticateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var username string
		var err error

		// 1. Try cookie authentication (session-based)
		cookie, err := r.Cookie("session_id")
		if err == nil && cookie.Value != "" {
			username, err = m.service.GetUsernameFromSession(ctx, cookie.Value)
			if err == nil {
				ctx = context.WithValue(ctx, ContextKeyUsername, username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 2. Try Authorization header (Bearer token)
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				sessionID := strings.TrimPrefix(authHeader, "Bearer ")
				username, err = m.service.GetUsernameFromSession(ctx, sessionID)
				if err == nil {
					ctx = context.WithValue(ctx, ContextKeyUsername, username)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// 3. Try Basic auth (username:password)
		if username, password, ok := r.BasicAuth(); ok {
			sessionID, err := m.service.Authenticate(ctx, username, password)
			if err == nil {
				// Set session cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "session_id",
					Value:    sessionID,
					Path:     "/",
					HttpOnly: true,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteLaxMode,
					Expires:  time.Now().Add(24 * time.Hour),
				})
				ctx = context.WithValue(ctx, ContextKeyUsername, username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// 4. Try query parameter (?pw=sessionID or ?pw=password with ?user=username)
		queryPW := r.URL.Query().Get("pw")
		queryUser := r.URL.Query().Get("user")
		if queryPW != "" {
			// First try as session ID
			username, err = m.service.GetUsernameFromSession(ctx, queryPW)
			if err == nil {
				ctx = context.WithValue(ctx, ContextKeyUsername, username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// If that fails and user is provided, try as password
			if queryUser != "" {
				sessionID, err := m.service.Authenticate(ctx, queryUser, queryPW)
				if err == nil {
					ctx = context.WithValue(ctx, ContextKeyUsername, queryUser)
					// Set session cookie
					http.SetCookie(w, &http.Cookie{
						Name:     "session_id",
						Value:    sessionID,
						Path:     "/",
						HttpOnly: true,
						Secure:   r.TLS != nil,
						SameSite: http.SameSiteLaxMode,
						Expires:  time.Now().Add(24 * time.Hour),
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// No valid authentication found
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// RequireAuth ensures the request is authenticated
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := r.Context().Value(ContextKeyUsername)
		if username == nil || username == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission ensures the user has specific permissions on a volume
func (m *Middleware) RequirePermission(volumeName string, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := r.Context().Value(ContextKeyUsername).(string)
			if !ok || username == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Get user permissions
			perms, err := m.service.GetUserPermissions(r.Context(), username, volumeName)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Check if user has the required permission
			if !perms.HasPermission(permission) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Store permissions in context for later use
			ctx := context.WithValue(r.Context(), ContextKeyPermissions, perms)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRead is a convenience wrapper for RequirePermission("read")
func (m *Middleware) RequireRead(volumeName string) func(http.Handler) http.Handler {
	return m.RequirePermission(volumeName, "read")
}

// RequireWrite is a convenience wrapper for RequirePermission("write")
func (m *Middleware) RequireWrite(volumeName string) func(http.Handler) http.Handler {
	return m.RequirePermission(volumeName, "write")
}

// RequireAdmin is a convenience wrapper for RequirePermission("admin")
func (m *Middleware) RequireAdmin(volumeName string) func(http.Handler) http.Handler {
	return m.RequirePermission(volumeName, "admin")
}

// GetUsername extracts the authenticated username from the request context
func GetUsername(r *http.Request) (string, bool) {
	username, ok := r.Context().Value(ContextKeyUsername).(string)
	return username, ok
}

// GetPermissions extracts the permissions from the request context
func GetPermissions(r *http.Request) (*Permissions, bool) {
	perms, ok := r.Context().Value(ContextKeyPermissions).(*Permissions)
	return perms, ok
}

// LoginHandler provides an HTTP handler for login
func (m *Middleware) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Error(w, "Username and password required", http.StatusBadRequest)
			return
		}

		sessionID, err := m.service.Authenticate(r.Context(), username, password)
		if err != nil {
			if err == ErrInvalidCredentials {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(24 * time.Hour),
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Login successful"))
	}
}

// LogoutHandler provides an HTTP handler for logout
func (m *Middleware) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err == nil && cookie.Value != "" {
			// Delete session from database
			m.service.DeleteSession(r.Context(), cookie.Value)
		}

		// Clear cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Logout successful"))
	}
}
