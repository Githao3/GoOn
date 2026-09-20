package sqliteopen

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no CGO)
)

// Open opens an existing SQLite database READ-ONLY and verifies connectivity.
// It never creates a file and never writes.
func Open(path string) (*sql.DB, error) {
	// file: URI with mode=ro; normalize separators for Windows.
	uri := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(2000)"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open readonly %s: %w", path, err)
	}
	return db, nil
}
