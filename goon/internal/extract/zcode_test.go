package extract

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the synthetic fixture
)

// newZcodeFixture builds a synthetic zcode-shaped SQLite DB in a temp dir and
// returns its path, the chosen cwd (BACKSLASHES, as zcode stores directories)
// and the target session id. zcode differs from opencode by carrying a
// `sequence` column on message/part and an extra `session_entry` metadata table
// that the shared loader must never read.
func newZcodeFixture(t *testing.T) (dbPath, cwd, sessionID string) {
	t.Helper()

	dir := t.TempDir()
	dbPath = filepath.Join(dir, "zcode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()

	const schema = `
CREATE TABLE session (
	id TEXT PRIMARY KEY,
	directory TEXT,
	path TEXT,
	title TEXT,
	time_updated INTEGER
);
CREATE TABLE message (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	time_created INTEGER,
	data TEXT,
	sequence INTEGER
);
CREATE TABLE part (
	id TEXT PRIMARY KEY,
	message_id TEXT,
	session_id TEXT,
	time_created INTEGER,
	data TEXT,
	sequence INTEGER
);
CREATE TABLE session_entry (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	type TEXT,
	time_created INTEGER,
	time_updated INTEGER,
	data TEXT
);
CREATE TABLE todo (
	session_id TEXT,
	content TEXT,
	status TEXT,
	priority TEXT,
	position INTEGER
);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// zcode stores directories with BACKSLASHES; matching this against a plain
	// cwd proves normalizeDir/ToSlash does the cross-client canonicalization.
	cwd = `D:\attempt\zcode\proj`
	sessionID = "sess_z1"
	otherCWD := `D:\attempt\zcode\elsewhere`

	if _, err := db.Exec(`INSERT INTO session (id, directory, path, title, time_updated) VALUES
		(?, ?, '', 'Z', 1700000000000),
		(?, ?, '', 'Other', 1600000000000)`,
		sessionID, cwd, "sess_other", otherCWD); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO message (id, session_id, time_created, data, sequence) VALUES
		('msg1', ?, 100, '{"role":"user"}', 1),
		('msg2', ?, 200, '{"role":"assistant"}', 2)`,
		sessionID, sessionID); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, data, sequence) VALUES
		('p1', 'msg1', ?, 100, '{"type":"text","text":"改登录"}', 1),
		('p2', 'msg2', ?, 200, '{"type":"text","text":"先看 types.ts"}', 2),
		('p3', 'msg2', ?, 300, '{"type":"tool","tool":"Edit","state":{"status":"error","input":{"file_path":"web/src/types.ts"}}}', 3)`,
		sessionID, sessionID, sessionID); err != nil {
		t.Fatalf("insert parts: %v", err)
	}

	// session_entry metadata must be ignored entirely by the shared loader.
	if _, err := db.Exec(`INSERT INTO session_entry (id, session_id, type, time_created, time_updated, data) VALUES
		('se1', ?, 'runtime/model_selection', 100, 100, '{"modelSelection":{"modelId":"glm"}}')`,
		sessionID); err != nil {
		t.Fatalf("insert session_entry: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO todo (session_id, content, status, priority, position) VALUES
		(?, '补测试', 'pending', 'high', 0)`, sessionID); err != nil {
		t.Fatalf("insert todo: %v", err)
	}

	return dbPath, cwd, sessionID
}

func TestParseZcodeDB(t *testing.T) {
	dbPath, _, sid := newZcodeFixture(t)
	m, err := ParseZcodeDB(dbPath, sid)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "zcode" {
		t.Fatalf("source %q", m.Source)
	}
	tr := m.Transcript()
	for _, want := range []string{"USER: 改登录", "ASSISTANT: 先看 types.ts", "TOOL Edit web/src/types.ts"} {
		if !strings.Contains(tr, want) {
			t.Fatalf("missing %q:\n%s", want, tr)
		}
	}
	// session_entry must not appear
	if strings.Contains(tr, "glm") || strings.Contains(tr, "model_selection") {
		t.Fatalf("session_entry leaked into transcript:\n%s", tr)
	}
	if len(m.Todos) == 0 || m.Todos[0] != "补测试" {
		t.Fatalf("todos %v", m.Todos)
	}
}

func TestListZcodeSessionsBackslashDir(t *testing.T) {
	dbPath, cwd, sid := newZcodeFixture(t)
	got, err := ListZcodeSessions(dbPath, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != sid {
		t.Fatalf("backslash-dir match failed: %+v", got)
	}
}
