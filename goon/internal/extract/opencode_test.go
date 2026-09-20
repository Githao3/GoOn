package extract

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the synthetic fixture
)

// newOpenCodeFixture builds a synthetic opencode-shaped SQLite DB in a temp dir
// and returns its path, the chosen cwd (forward slashes) and the target session id.
func newOpenCodeFixture(t *testing.T) (dbPath, cwd, sessionID string) {
	t.Helper()

	dir := t.TempDir()
	dbPath = filepath.Join(dir, "oc.db")

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
	data TEXT
);
CREATE TABLE part (
	id TEXT PRIMARY KEY,
	message_id TEXT,
	session_id TEXT,
	time_created INTEGER,
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

	cwd = "C:/Users/Admin/Desktop/tmp"
	sessionID = "ses_t1"
	otherCWD := "C:/Users/Admin/Desktop/other"

	// Target session plus a second session in a different directory (to prove filtering).
	if _, err := db.Exec(`INSERT INTO session (id, directory, path, title, time_updated) VALUES
		(?, ?, '', 'T', 1700000000000),
		(?, ?, '', 'Other', 1600000000000)`,
		sessionID, cwd, "ses_other", otherCWD); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}

	// Two messages: user then assistant.
	if _, err := db.Exec(`INSERT INTO message (id, session_id, time_created, data) VALUES
		('msg1', ?, 100, '{"role":"user"}'),
		('msg2', ?, 200, '{"role":"assistant"}')`,
		sessionID, sessionID); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	// Text parts (ordered by time_created) plus one tool part.
	if _, err := db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, data) VALUES
		('p1', 'msg1', ?, 100, '{"type":"text","text":"hello"}'),
		('p2', 'msg2', ?, 200, '{"type":"text","text":"world"}'),
		('p3', 'msg2', ?, 300, '{"type":"tool","tool":"Write","state":{"status":"completed","input":{"file_path":"a.go"}}}')`,
		sessionID, sessionID, sessionID); err != nil {
		t.Fatalf("insert parts: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO todo (session_id, content, status, priority, position) VALUES
		(?, '下一步', 'pending', 'high', 0)`, sessionID); err != nil {
		t.Fatalf("insert todo: %v", err)
	}

	return dbPath, cwd, sessionID
}

func TestParseOpenCodeDB(t *testing.T) {
	dbPath, _, sid := newOpenCodeFixture(t)
	m, err := ParseOpenCodeDB(dbPath, sid)
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "opencode" || m.SessionID != sid {
		t.Fatalf("meta: %+v", m)
	}
	// Text events come first (ordered by p.time_created), then tool events from a
	// separate query; assert substring presence only, not cross-group ordering.
	tr := m.Transcript()
	for _, want := range []string{"USER: hello", "ASSISTANT: world", "TOOL Write a.go"} {
		if !strings.Contains(tr, want) {
			t.Fatalf("transcript missing %q:\n%s", want, tr)
		}
	}
	if len(m.Todos) == 0 || m.Todos[0] != "下一步" {
		t.Fatalf("todos: %v", m.Todos)
	}
}

func TestListOpenCodeSessionsFiltersByCwd(t *testing.T) {
	dbPath, cwd, sid := newOpenCodeFixture(t)
	got, err := ListOpenCodeSessions(dbPath, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != sid {
		t.Fatalf("expected only the matching session, got %+v", got)
	}
}
