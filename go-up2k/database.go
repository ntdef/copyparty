package up2k

import (
	"database/sql"
	"errors"
	"time"
)

var (
	ErrUploadNotFound = errors.New("upload not found")
)

// SQLiteDB implements the Database interface using SQLite
type SQLiteDB struct {
	db *sql.DB
}

// NewSQLiteDB creates a new SQLite database instance
func NewSQLiteDB(db *sql.DB) (*SQLiteDB, error) {
	s := &SQLiteDB{db: db}
	if err := s.createSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// createSchema creates the necessary tables
func (s *SQLiteDB) createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS up2k_uploads (
		wark TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		size INTEGER NOT NULL,
		uploaded_at INTEGER NOT NULL,
		username TEXT,
		client_ip TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_up2k_uploaded_at ON up2k_uploads(uploaded_at);
	CREATE INDEX IF NOT EXISTS idx_up2k_filename ON up2k_uploads(filename);
	`

	_, err := s.db.Exec(schema)
	return err
}

// RecordUpload stores completed upload metadata
func (s *SQLiteDB) RecordUpload(wark, filename string, size int64, uploadedAt time.Time, username, ip string) error {
	query := `
		INSERT INTO up2k_uploads (wark, filename, size, uploaded_at, username, client_ip)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(wark) DO UPDATE SET
			filename = excluded.filename,
			size = excluded.size,
			uploaded_at = excluded.uploaded_at
	`

	_, err := s.db.Exec(query, wark, filename, size, uploadedAt.Unix(), username, ip)
	return err
}

// FindUploadByWark checks if upload with wark already exists
func (s *SQLiteDB) FindUploadByWark(wark string) (*UploadRecord, error) {
	query := `
		SELECT wark, filename, size, uploaded_at, username, client_ip
		FROM up2k_uploads
		WHERE wark = ?
	`

	var record UploadRecord
	var uploadedAt int64

	err := s.db.QueryRow(query, wark).Scan(
		&record.Wark,
		&record.Filename,
		&record.Size,
		&uploadedAt,
		&record.Username,
		&record.ClientIP,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUploadNotFound
	}
	if err != nil {
		return nil, err
	}

	record.UploadedAt = time.Unix(uploadedAt, 0)
	return &record, nil
}

// DeleteUpload removes upload record
func (s *SQLiteDB) DeleteUpload(wark string) error {
	query := `DELETE FROM up2k_uploads WHERE wark = ?`
	_, err := s.db.Exec(query, wark)
	return err
}

// ListRecentUploads returns recent uploads (optional helper)
func (s *SQLiteDB) ListRecentUploads(limit int) ([]*UploadRecord, error) {
	query := `
		SELECT wark, filename, size, uploaded_at, username, client_ip
		FROM up2k_uploads
		ORDER BY uploaded_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*UploadRecord
	for rows.Next() {
		var record UploadRecord
		var uploadedAt int64

		err := rows.Scan(
			&record.Wark,
			&record.Filename,
			&record.Size,
			&uploadedAt,
			&record.Username,
			&record.ClientIP,
		)
		if err != nil {
			return nil, err
		}

		record.UploadedAt = time.Unix(uploadedAt, 0)
		records = append(records, &record)
	}

	return records, rows.Err()
}
