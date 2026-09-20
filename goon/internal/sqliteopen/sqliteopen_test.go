package sqliteopen

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for fixture creation
)

// buildFixture creates a real, writable sqlite DB file with one row and closes it.
func buildFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open writable fixture: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('wantirna')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	return path, "wantirna"
}

func TestOpenReadsBackRow(t *testing.T) {
	path, want := buildFixture(t)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) returned error: %v", path, err)
	}
	defer db.Close()

	var got string
	if err := db.QueryRow(`SELECT name FROM items LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if got != want {
		t.Fatalf("read value = %q, want %q", got, want)
	}
}

func TestOpenNonExistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.db")

	db, err := Open(missing)
	if err == nil {
		db.Close()
		t.Fatalf("Open on non-existent path returned nil error, want error")
	}
}
