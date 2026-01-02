package up2k

import (
	"database/sql"
	"errors"
	"time"

	"github.com/yourusername/up2k/db"
)

// SQLCDatabase wraps sqlc-generated queries to implement Database interface
type SQLCDatabase struct {
	queries *db.Queries
	db      *sql.DB
}

// NewSQLCDatabase creates a new database instance using sqlc
func NewSQLCDatabase(database *sql.DB) (*SQLCDatabase, error) {
	s := &SQLCDatabase{
		queries: db.New(database),
		db:      database,
	}

	// Create schema
	if err := s.createSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SQLCDatabase) createSchema() error {
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
	CREATE INDEX IF NOT EXISTS idx_up2k_username ON up2k_uploads(username);
	`

	_, err := s.db.Exec(schema)
	return err
}

// RecordUpload stores completed upload metadata
func (s *SQLCDatabase) RecordUpload(wark, filename string, size int64, uploadedAt time.Time, username, ip string) error {
	ctx := context.Background()
	return s.queries.RecordUpload(ctx, db.RecordUploadParams{
		Wark:       wark,
		Filename:   filename,
		Size:       size,
		UploadedAt: uploadedAt.Unix(),
		Username:   sql.NullString{String: username, Valid: username != ""},
		ClientIp:   sql.NullString{String: ip, Valid: ip != ""},
	})
}

// FindUploadByWark checks if upload with wark already exists
func (s *SQLCDatabase) FindUploadByWark(wark string) (*UploadRecord, error) {
	ctx := context.Background()
	row, err := s.queries.FindUploadByWark(ctx, wark)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUploadNotFound
		}
		return nil, err
	}

	return &UploadRecord{
		Wark:       row.Wark,
		Filename:   row.Filename,
		Size:       row.Size,
		UploadedAt: time.Unix(row.UploadedAt, 0),
		Username:   row.Username.String,
		ClientIP:   row.ClientIp.String,
	}, nil
}

// DeleteUpload removes upload record
func (s *SQLCDatabase) DeleteUpload(wark string) error {
	ctx := context.Background()
	return s.queries.DeleteUpload(ctx, wark)
}

// ListRecentUploads returns recent uploads
func (s *SQLCDatabase) ListRecentUploads(limit int) ([]*UploadRecord, error) {
	ctx := context.Background()
	rows, err := s.queries.ListRecentUploads(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	records := make([]*UploadRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, &UploadRecord{
			Wark:       row.Wark,
			Filename:   row.Filename,
			Size:       row.Size,
			UploadedAt: time.Unix(row.UploadedAt, 0),
			Username:   row.Username.String,
			ClientIP:   row.ClientIp.String,
		})
	}

	return records, nil
}
