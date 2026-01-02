-- name: RecordUpload :exec
INSERT INTO up2k_uploads (wark, filename, size, uploaded_at, username, client_ip)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(wark) DO UPDATE SET
    filename = excluded.filename,
    size = excluded.size,
    uploaded_at = excluded.uploaded_at;

-- name: FindUploadByWark :one
SELECT wark, filename, size, uploaded_at, username, client_ip
FROM up2k_uploads
WHERE wark = ?;

-- name: DeleteUpload :exec
DELETE FROM up2k_uploads WHERE wark = ?;

-- name: ListRecentUploads :many
SELECT wark, filename, size, uploaded_at, username, client_ip
FROM up2k_uploads
ORDER BY uploaded_at DESC
LIMIT ?;

-- name: ListUploadsByUser :many
SELECT wark, filename, size, uploaded_at, username, client_ip
FROM up2k_uploads
WHERE username = ?
ORDER BY uploaded_at DESC
LIMIT ?;

-- name: CountUploads :one
SELECT COUNT(*) FROM up2k_uploads;

-- name: GetTotalUploadSize :one
SELECT COALESCE(SUM(size), 0) FROM up2k_uploads;
