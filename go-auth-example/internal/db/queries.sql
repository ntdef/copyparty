-- name: CreateUser :exec
INSERT INTO users (username, password_hash, created_at)
VALUES (?, ?, ?);

-- name: GetUserByUsername :one
SELECT id, username, password_hash, created_at
FROM users
WHERE username = ?;

-- name: GetUserPasswordHash :one
SELECT password_hash
FROM users
WHERE username = ?;

-- name: ListUsers :many
SELECT id, username, created_at
FROM users
ORDER BY username;

-- name: DeleteUser :exec
DELETE FROM users
WHERE username = ?;

-- name: CreateSession :exec
INSERT INTO sessions (username, session_id, created_at, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionByID :one
SELECT username, created_at, expires_at
FROM sessions
WHERE session_id = ? AND expires_at > ?;

-- name: GetSessionsByUsername :many
SELECT session_id, created_at, expires_at
FROM sessions
WHERE username = ?
ORDER BY created_at DESC;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE session_id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= ?;

-- name: DeleteUserSessions :exec
DELETE FROM sessions
WHERE username = ?;

-- name: CreateGroup :exec
INSERT INTO groups (name)
VALUES (?);

-- name: GetGroupByName :one
SELECT id, name
FROM groups
WHERE name = ?;

-- name: ListGroups :many
SELECT id, name
FROM groups
ORDER BY name;

-- name: DeleteGroup :exec
DELETE FROM groups
WHERE name = ?;

-- name: AddUserToGroup :exec
INSERT INTO user_groups (user_id, group_id)
VALUES (?, ?);

-- name: RemoveUserFromGroup :exec
DELETE FROM user_groups
WHERE user_id = ? AND group_id = ?;

-- name: GetUserGroups :many
SELECT g.id, g.name
FROM groups g
JOIN user_groups ug ON g.id = ug.group_id
JOIN users u ON u.id = ug.user_id
WHERE u.username = ?;

-- name: GetGroupMembers :many
SELECT u.id, u.username
FROM users u
JOIN user_groups ug ON u.id = ug.user_id
JOIN groups g ON g.id = ug.group_id
WHERE g.name = ?;

-- name: CreateVolume :exec
INSERT INTO volumes (name, path)
VALUES (?, ?);

-- name: GetVolumeByName :one
SELECT id, name, path
FROM volumes
WHERE name = ?;

-- name: ListVolumes :many
SELECT id, name, path
FROM volumes
ORDER BY name;

-- name: DeleteVolume :exec
DELETE FROM volumes
WHERE name = ?;

-- name: CreateUserPermission :exec
INSERT INTO permissions (
    volume_id, user_id, can_read, can_write, can_move,
    can_delete, can_admin, can_get, can_dot
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateGroupPermission :exec
INSERT INTO permissions (
    volume_id, group_id, can_read, can_write, can_move,
    can_delete, can_admin, can_get, can_dot
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetUserPermissions :many
SELECT
    can_read, can_write, can_move, can_delete,
    can_admin, can_get, can_dot
FROM permissions p
JOIN volumes v ON p.volume_id = v.id
JOIN users u ON p.user_id = u.id
WHERE u.username = ? AND v.name = ?;

-- name: GetGroupPermissions :many
SELECT
    can_read, can_write, can_move, can_delete,
    can_admin, can_get, can_dot
FROM permissions p
JOIN volumes v ON p.volume_id = v.id
JOIN groups g ON p.group_id = g.id
WHERE g.name = ? AND v.name = ?;

-- name: GetAllUserPermissionsForVolume :many
SELECT
    u.username,
    p.can_read, p.can_write, p.can_move, p.can_delete,
    p.can_admin, p.can_get, p.can_dot
FROM permissions p
JOIN volumes v ON p.volume_id = v.id
JOIN users u ON p.user_id = u.id
WHERE v.name = ?;

-- name: GetAllGroupPermissionsForVolume :many
SELECT
    g.name as group_name,
    p.can_read, p.can_write, p.can_move, p.can_delete,
    p.can_admin, p.can_get, p.can_dot
FROM permissions p
JOIN volumes v ON p.volume_id = v.id
JOIN groups g ON p.group_id = g.id
WHERE v.name = ?;

-- name: DeleteUserPermission :exec
DELETE FROM permissions
WHERE volume_id = ? AND user_id = ?;

-- name: DeleteGroupPermission :exec
DELETE FROM permissions
WHERE volume_id = ? AND group_id = ?;

-- name: UpdateUserPermission :exec
UPDATE permissions
SET
    can_read = ?,
    can_write = ?,
    can_move = ?,
    can_delete = ?,
    can_admin = ?,
    can_get = ?,
    can_dot = ?
WHERE volume_id = ? AND user_id = ?;

-- name: UpdateGroupPermission :exec
UPDATE permissions
SET
    can_read = ?,
    can_write = ?,
    can_move = ?,
    can_delete = ?,
    can_admin = ?,
    can_get = ?,
    can_dot = ?
WHERE volume_id = ? AND group_id = ?;
