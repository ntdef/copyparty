package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/example/go-auth-system/internal/db"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUserExists         = errors.New("user already exists")
	ErrGroupExists        = errors.New("group already exists")
	ErrVolumeExists       = errors.New("volume already exists")
)

// Argon2 parameters (matching copyparty's defaults)
const (
	Argon2Time      = 3
	Argon2Memory    = 256 * 1024 // 256 MiB
	Argon2Threads   = 4
	Argon2KeyLength = 32
	SaltLength      = 16
	SessionIDLength = 32
)

// Service handles authentication and authorization
type Service struct {
	db      *sql.DB
	queries *db.Queries
	salt    []byte
}

// Permissions represents access rights for a user on a volume
type Permissions struct {
	CanRead   bool
	CanWrite  bool
	CanMove   bool
	CanDelete bool
	CanAdmin  bool
	CanGet    bool
	CanDot    bool
}

// NewService creates a new authentication service
func NewService(dbPath string) (*Service, error) {
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := database.Exec("PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Read schema and create tables
	schema := `
-- Users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    session_id TEXT UNIQUE NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    FOREIGN KEY (username) REFERENCES users(username) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_session_id ON sessions(session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- Groups table
CREATE TABLE IF NOT EXISTS groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_groups_name ON groups(name);

-- User groups mapping
CREATE TABLE IF NOT EXISTS user_groups (
    user_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, group_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_groups_user_id ON user_groups(user_id);
CREATE INDEX IF NOT EXISTS idx_user_groups_group_id ON user_groups(group_id);

-- Volumes table
CREATE TABLE IF NOT EXISTS volumes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    path TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_volumes_name ON volumes(name);

-- Permissions table
CREATE TABLE IF NOT EXISTS permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    volume_id INTEGER NOT NULL,
    user_id INTEGER,
    group_id INTEGER,
    can_read BOOLEAN DEFAULT 0,
    can_write BOOLEAN DEFAULT 0,
    can_move BOOLEAN DEFAULT 0,
    can_delete BOOLEAN DEFAULT 0,
    can_admin BOOLEAN DEFAULT 0,
    can_get BOOLEAN DEFAULT 0,
    can_dot BOOLEAN DEFAULT 0,
    FOREIGN KEY (volume_id) REFERENCES volumes(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    CHECK ((user_id IS NOT NULL AND group_id IS NULL) OR (user_id IS NULL AND group_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_permissions_volume_id ON permissions(volume_id);
CREATE INDEX IF NOT EXISTS idx_permissions_user_id ON permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_permissions_group_id ON permissions(group_id);
`

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Generate a random salt for password hashing
	salt := make([]byte, SaltLength)
	if _, err := rand.Read(salt); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	return &Service{
		db:      database,
		queries: db.New(database),
		salt:    salt,
	}, nil
}

// Close closes the database connection
func (s *Service) Close() error {
	return s.db.Close()
}

// hashPassword hashes a password using Argon2id
func (s *Service) hashPassword(password string) string {
	hash := argon2.IDKey(
		[]byte(password),
		s.salt,
		Argon2Time,
		Argon2Memory,
		Argon2Threads,
		Argon2KeyLength,
	)
	return hex.EncodeToString(hash)
}

// generateSessionID generates a cryptographically secure session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, SessionIDLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// CreateUser creates a new user with a hashed password
func (s *Service) CreateUser(ctx context.Context, username, password string) error {
	passwordHash := s.hashPassword(password)
	createdAt := time.Now().Unix()

	err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    createdAt,
	})

	if err != nil {
		if err.Error() == "UNIQUE constraint failed: users.username" {
			return ErrUserExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// Authenticate verifies credentials and returns a session ID
func (s *Service) Authenticate(ctx context.Context, username, password string) (string, error) {
	// Get the stored password hash
	storedHash, err := s.queries.GetUserPasswordHash(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Hash the provided password and compare
	providedHash := s.hashPassword(password)
	if providedHash != storedHash {
		return "", ErrInvalidCredentials
	}

	// Generate session ID
	sessionID, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Create session (expires in 24 hours by default)
	createdAt := time.Now().Unix()
	expiresAt := time.Now().Add(24 * time.Hour).Unix()

	err = s.queries.CreateSession(ctx, db.CreateSessionParams{
		Username:  username,
		SessionID: sessionID,
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	})

	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

// GetUsernameFromSession retrieves the username associated with a session ID
func (s *Service) GetUsernameFromSession(ctx context.Context, sessionID string) (string, error) {
	now := time.Now().Unix()

	session, err := s.queries.GetSessionByID(ctx, db.GetSessionByIDParams{
		SessionID: sessionID,
		ExpiresAt: now,
	})

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSessionNotFound
		}
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	return session.Username, nil
}

// DeleteSession removes a session (logout)
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	return s.queries.DeleteSession(ctx, sessionID)
}

// DeleteUserSessions removes all sessions for a user
func (s *Service) DeleteUserSessions(ctx context.Context, username string) error {
	return s.queries.DeleteUserSessions(ctx, username)
}

// CleanupExpiredSessions removes expired sessions
func (s *Service) CleanupExpiredSessions(ctx context.Context) error {
	now := time.Now().Unix()
	return s.queries.DeleteExpiredSessions(ctx, now)
}

// CreateGroup creates a new group
func (s *Service) CreateGroup(ctx context.Context, groupName string) error {
	err := s.queries.CreateGroup(ctx, groupName)
	if err != nil {
		if err.Error() == "UNIQUE constraint failed: groups.name" {
			return ErrGroupExists
		}
		return fmt.Errorf("failed to create group: %w", err)
	}
	return nil
}

// AddUserToGroup adds a user to a group
func (s *Service) AddUserToGroup(ctx context.Context, username, groupName string) error {
	// Get user ID
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get group ID
	group, err := s.queries.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("group not found")
		}
		return fmt.Errorf("failed to get group: %w", err)
	}

	// Add user to group
	err = s.queries.AddUserToGroup(ctx, db.AddUserToGroupParams{
		UserID:  user.ID,
		GroupID: group.ID,
	})

	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}

	return nil
}

// GetUserGroups returns all groups a user belongs to
func (s *Service) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	groups, err := s.queries.GetUserGroups(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}

	groupNames := make([]string, len(groups))
	for i, g := range groups {
		groupNames[i] = g.Name
	}

	return groupNames, nil
}

// CreateVolume creates a new volume (resource)
func (s *Service) CreateVolume(ctx context.Context, name, path string) error {
	err := s.queries.CreateVolume(ctx, db.CreateVolumeParams{
		Name: name,
		Path: path,
	})

	if err != nil {
		if err.Error() == "UNIQUE constraint failed: volumes.name" {
			return ErrVolumeExists
		}
		return fmt.Errorf("failed to create volume: %w", err)
	}

	return nil
}

// SetUserPermissions sets permissions for a user on a volume
func (s *Service) SetUserPermissions(ctx context.Context, username, volumeName string, perms Permissions) error {
	// Get user ID
	user, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get volume ID
	volume, err := s.queries.GetVolumeByName(ctx, volumeName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("volume not found")
		}
		return fmt.Errorf("failed to get volume: %w", err)
	}

	// Create or update permission
	err = s.queries.CreateUserPermission(ctx, db.CreateUserPermissionParams{
		VolumeID:  volume.ID,
		UserID:    sql.NullInt64{Int64: user.ID, Valid: true},
		CanRead:   perms.CanRead,
		CanWrite:  perms.CanWrite,
		CanMove:   perms.CanMove,
		CanDelete: perms.CanDelete,
		CanAdmin:  perms.CanAdmin,
		CanGet:    perms.CanGet,
		CanDot:    perms.CanDot,
	})

	if err != nil {
		// If permission exists, update it
		err = s.queries.UpdateUserPermission(ctx, db.UpdateUserPermissionParams{
			CanRead:   perms.CanRead,
			CanWrite:  perms.CanWrite,
			CanMove:   perms.CanMove,
			CanDelete: perms.CanDelete,
			CanAdmin:  perms.CanAdmin,
			CanGet:    perms.CanGet,
			CanDot:    perms.CanDot,
			VolumeID:  volume.ID,
			UserID:    sql.NullInt64{Int64: user.ID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update user permission: %w", err)
		}
	}

	return nil
}

// SetGroupPermissions sets permissions for a group on a volume
func (s *Service) SetGroupPermissions(ctx context.Context, groupName, volumeName string, perms Permissions) error {
	// Get group ID
	group, err := s.queries.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("group not found")
		}
		return fmt.Errorf("failed to get group: %w", err)
	}

	// Get volume ID
	volume, err := s.queries.GetVolumeByName(ctx, volumeName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("volume not found")
		}
		return fmt.Errorf("failed to get volume: %w", err)
	}

	// Create or update permission
	err = s.queries.CreateGroupPermission(ctx, db.CreateGroupPermissionParams{
		VolumeID:  volume.ID,
		GroupID:   sql.NullInt64{Int64: group.ID, Valid: true},
		CanRead:   perms.CanRead,
		CanWrite:  perms.CanWrite,
		CanMove:   perms.CanMove,
		CanDelete: perms.CanDelete,
		CanAdmin:  perms.CanAdmin,
		CanGet:    perms.CanGet,
		CanDot:    perms.CanDot,
	})

	if err != nil {
		// If permission exists, update it
		err = s.queries.UpdateGroupPermission(ctx, db.UpdateGroupPermissionParams{
			CanRead:   perms.CanRead,
			CanWrite:  perms.CanWrite,
			CanMove:   perms.CanMove,
			CanDelete: perms.CanDelete,
			CanAdmin:  perms.CanAdmin,
			CanGet:    perms.CanGet,
			CanDot:    perms.CanDot,
			VolumeID:  volume.ID,
			GroupID:   sql.NullInt64{Int64: group.ID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update group permission: %w", err)
		}
	}

	return nil
}

// GetUserPermissions retrieves a user's effective permissions on a volume
// This includes both direct user permissions and inherited group permissions
func (s *Service) GetUserPermissions(ctx context.Context, username, volumeName string) (*Permissions, error) {
	// Initialize with no permissions
	perms := &Permissions{}

	// Get direct user permissions
	userPerms, err := s.queries.GetUserPermissions(ctx, db.GetUserPermissionsParams{
		Username: username,
		Name:     volumeName,
	})

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Apply user permissions (OR operation)
	for _, p := range userPerms {
		perms.CanRead = perms.CanRead || p.CanRead
		perms.CanWrite = perms.CanWrite || p.CanWrite
		perms.CanMove = perms.CanMove || p.CanMove
		perms.CanDelete = perms.CanDelete || p.CanDelete
		perms.CanAdmin = perms.CanAdmin || p.CanAdmin
		perms.CanGet = perms.CanGet || p.CanGet
		perms.CanDot = perms.CanDot || p.CanDot
	}

	// Get user's groups
	groups, err := s.GetUserGroups(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}

	// Get permissions from each group
	for _, groupName := range groups {
		groupPerms, err := s.queries.GetGroupPermissions(ctx, db.GetGroupPermissionsParams{
			Name:   groupName,
			Name_2: volumeName,
		})

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to get group permissions: %w", err)
		}

		// Apply group permissions (OR operation)
		for _, p := range groupPerms {
			perms.CanRead = perms.CanRead || p.CanRead
			perms.CanWrite = perms.CanWrite || p.CanWrite
			perms.CanMove = perms.CanMove || p.CanMove
			perms.CanDelete = perms.CanDelete || p.CanDelete
			perms.CanAdmin = perms.CanAdmin || p.CanAdmin
			perms.CanGet = perms.CanGet || p.CanGet
			perms.CanDot = perms.CanDot || p.CanDot
		}
	}

	return perms, nil
}

// HasPermission is a helper to check if a user has a specific permission
func (p *Permissions) HasPermission(perm string) bool {
	switch perm {
	case "r", "read":
		return p.CanRead
	case "w", "write":
		return p.CanWrite
	case "m", "move":
		return p.CanMove
	case "d", "delete":
		return p.CanDelete
	case "a", "admin":
		return p.CanAdmin
	case "g", "get":
		return p.CanGet
	case ".", "dot":
		return p.CanDot
	default:
		return false
	}
}
