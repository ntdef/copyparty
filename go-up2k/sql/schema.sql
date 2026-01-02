-- up2k uploads table
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
