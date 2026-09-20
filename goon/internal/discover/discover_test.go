package discover

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the synthetic fixture
)

// wantSlug mirrors the production slug so tests can lay out the expected
// per-project directory without importing the unexported helper's exact form.
func wantSlug(cwd string) string {
	return strings.NewReplacer(":", "-", `\`, "-", "/", "-").Replace(cwd)
}

// writeFileWithMtime writes content to path (creating parent dirs) and pins the
// modification time so merge ordering is deterministic.
func writeFileWithMtime(t *testing.T, path string, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// newOpenCodeDB builds a synthetic opencode-shaped SQLite DB with a single
// session whose directory matches cwd (normalized) and time_updated = modMs.
func newOpenCodeDB(t *testing.T, dbPath, directory, id, title string, modMs int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	const schema = `CREATE TABLE session (
	id TEXT PRIMARY KEY,
	directory TEXT,
	path TEXT,
	title TEXT,
	time_updated INTEGER);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO session (id, directory, path, title, time_updated) VALUES (?, ?, '', ?, ?)`,
		id, directory, title, modMs); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func TestRecent_MergesAndSorts(t *testing.T) {
	root := t.TempDir()
	cwd := `C:\goon-test\proj`

	// Two claude sessions under the slug dir: now-2h and now-1h.
	claudeRoot := filepath.Join(root, "claude-projects")
	slugDir := filepath.Join(claudeRoot, wantSlug(cwd))
	now := time.Now()
	writeFileWithMtime(t, filepath.Join(slugDir, "session-a.jsonl"),
		`{"type":"user","message":{"role":"user","content":"a"}}`+"\n", now.Add(-2*time.Hour))
	writeFileWithMtime(t, filepath.Join(slugDir, "session-b.jsonl"),
		`{"type":"user","message":{"role":"user","content":"b"}}`+"\n", now.Add(-1*time.Hour))

	// One opencode session matching the same cwd via forward slashes, now-30m.
	dbPath := filepath.Join(root, "oc.db")
	ocID := "ses_t1"
	newOpenCodeDB(t, dbPath, "C:/goon-test/proj", ocID, "OpenCode Title",
		now.Add(-30*time.Minute).UnixMilli())

	got := Recent(cwd, Roots{ClaudeProjects: claudeRoot, OpenCodeDB: dbPath})
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %+v", len(got), got)
	}
	// Newest first: opencode (-30m), then claude b (-1h), then claude a (-2h).
	if got[0].Client != "opencode" {
		t.Fatalf("first should be opencode, got %q", got[0].Client)
	}
	if want := dbPath + "#" + ocID; got[0].Ref != want {
		t.Fatalf("opencode ref = %q, want %q", got[0].Ref, want)
	}
	if got[0].Kind != KindSQLite {
		t.Fatalf("opencode kind = %v, want KindSQLite", got[0].Kind)
	}
	for _, c := range got[1:] {
		if c.Client != "claude-code" || c.Kind != KindJSONL {
			t.Fatalf("expected claude-code JSONL, got %+v", c)
		}
	}
	if !strings.HasSuffix(got[1].Ref, "session-b.jsonl") {
		t.Fatalf("second should be session-b, got %q", got[1].Ref)
	}
	if !strings.HasSuffix(got[2].Ref, "session-a.jsonl") {
		t.Fatalf("third should be session-a, got %q", got[2].Ref)
	}
}

func TestClaudeSlugAndTitle(t *testing.T) {
	root := t.TempDir()
	cwd := `D:\Attempt\ClaudeCode\Test`
	claudeRoot := filepath.Join(root, "claude-projects")
	slugDir := filepath.Join(claudeRoot, wantSlug(cwd))
	if got := filepath.Base(slugDir); got != "D--Attempt-ClaudeCode-Test" {
		t.Fatalf("unexpected slug dir name %q (want D--Attempt-ClaudeCode-Test)", got)
	}
	now := time.Now()
	jsonlPath := filepath.Join(slugDir, "s1.jsonl")
	writeFileWithMtime(t, jsonlPath,
		`{"type":"summary","summary":"ignore me"}`+"\n"+
			`{"type":"user","message":{"role":"user","content":"hello world"}}`+"\n",
		now)

	got := Recent(cwd, Roots{ClaudeProjects: claudeRoot})
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Client != "claude-code" || c.Kind != KindJSONL {
		t.Fatalf("bad client/kind: %+v", c)
	}
	if c.Ref != jsonlPath {
		t.Fatalf("ref = %q, want %q", c.Ref, jsonlPath)
	}
	if c.Title != "hello world" {
		t.Fatalf("title = %q, want %q", c.Title, "hello world")
	}
}

func TestCodexMatchesByMetaCwd(t *testing.T) {
	root := t.TempDir()
	cwd := `E:\work\repo`
	codexRoot := filepath.Join(root, "codex-sessions")

	// meta cwd is JSON-escaped (double backslashes) so it decodes to E:\work\repo,
	// matching the query cwd after normalization; rollout-y has a different cwd.
	writeFileWithMtime(t, filepath.Join(codexRoot, "rollout-x.jsonl"),
		`{"type":"session_meta","payload":{"cwd":"E:\\work\\repo"}}`+"\n", time.Now().Add(-time.Hour))
	writeFileWithMtime(t, filepath.Join(codexRoot, "rollout-y.jsonl"),
		`{"type":"session_meta","payload":{"cwd":"E:\\other\\place"}}`+"\n", time.Now())

	got := Recent(cwd, Roots{CodexSessions: codexRoot})
	if len(got) != 1 {
		t.Fatalf("expected 1 codex candidate, got %d: %+v", len(got), got)
	}
	c := got[0]
	if c.Client != "codex" || c.Kind != KindJSONL {
		t.Fatalf("bad client/kind: %+v", c)
	}
	if !strings.HasSuffix(c.Ref, "rollout-x.jsonl") {
		t.Fatalf("ref = %q, want rollout-x.jsonl", c.Ref)
	}
}

func TestRecent_MissingSourcesNoError(t *testing.T) {
	root := t.TempDir()
	r := Roots{
		ClaudeProjects: filepath.Join(root, "nope-claude"),
		CodexSessions:  filepath.Join(root, "nope-codex"),
		OpenCodeDB:     filepath.Join(root, "nope-oc.db"),
		ZcodeDB:        filepath.Join(root, "nope-zcode.db"),
	}
	if got := Recent(`C:\does\not\matter`, r); len(got) != 0 {
		t.Fatalf("expected no candidates, got %+v", got)
	}
	// Also must not panic on fully empty roots.
	if got := Recent(`C:\any\cwd`, Roots{}); len(got) != 0 {
		t.Fatalf("expected no candidates for empty roots, got %+v", got)
	}
}
