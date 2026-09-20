package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goon/internal/config"
	"goon/internal/discover"
	"goon/internal/extract"
	"goon/internal/handoff"
	"goon/internal/store"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the synthetic fixture
)

type fakeChat struct{}

func (fakeChat) Complete(string) (string, error) {
	return "## 目标\n做GoOn\n## 当前状态\n进行中\n## 关键决策\n用Go\n## 改动文件\na.go\n## 下一步\n测\n## 坑与约定\n无\n## 开放问题\n无", nil
}

func newApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	cfg, err := config.Load("/no/such/g", "/no/such/p")
	if err != nil {
		t.Fatal(err)
	}
	return &App{Root: root, Cfg: cfg, Now: func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }}
}

func TestEndToEnd_DistillResumeSalvage(t *testing.T) {
	app := newApp(t)
	if err := app.Init(); err != nil {
		t.Fatal(err)
	}
	m := extract.SessionModel{
		Source:    "claude-code",
		SessionID: "s1",
		Project:   extract.Project{CWD: app.Root, Branch: "main"},
		Events:    []extract.Event{{Kind: "user", Text: "开始"}},
	}
	id, err := app.Distill(m, fakeChat{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Init(app.Root, ".goon")
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if h.FM.Created == "" {
		t.Fatal("handoff Created should be set")
	}
	if !strings.Contains(h.Body, "做GoOn") || h.FM.Source != "claude-code" {
		t.Fatalf("bad handoff: %+v", h.FM)
	}
	prompt, err := app.Resume(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Drift Report") || !strings.Contains(prompt, "做GoOn") {
		t.Fatalf("resume incomplete:\n%s", prompt)
	}
	name, err := app.WriteSalvage(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app.Root, ".goon", "salvage", name)); err != nil {
		t.Fatalf("salvage raw not written: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(app.Root, ".goon", "salvage", name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "<untrusted>") {
		t.Fatalf("salvage should not use raw untrusted tags:\n%s", raw)
	}
}

func TestResume_RejectsPathTraversalID(t *testing.T) {
	app := newApp(t)
	if err := app.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Resume(`..\evil`); err == nil {
		t.Fatal("expected invalid id error (backslash)")
	}
	if _, err := app.Resume("../../etc/passwd"); err == nil {
		t.Fatal("expected invalid id error (forward slash)")
	}
}

func TestNew_RejectsInvalidSource(t *testing.T) {
	app := newApp(t)
	if err := app.Init(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "..", "a/b", "fo<o", strings.Repeat("x", 40)} {
		if _, err := app.New(bad); err == nil {
			t.Fatalf("expected invalid source rejection for %q", bad)
		}
	}
	if _, err := app.New("codex"); err != nil {
		t.Fatalf("valid source should succeed: %v", err)
	}
}

func TestFinalize_RejectsMissingSections(t *testing.T) {
	app := newApp(t)
	if err := app.Init(); err != nil {
		t.Fatal(err)
	}
	s, _ := store.Init(app.Root, ".goon")
	bad := handoff.Handoff{FM: handoff.FrontMatter{Goon: 1, ID: "z", Source: "codex"}, Body: "## 目标\n只这节"}
	if err := s.Save(bad, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := app.Finalize("z"); err == nil {
		t.Fatal("finalize should fail on missing sections")
	}
}

// newTestOpenCodeDB lays down a synthetic opencode-shaped SQLite DB holding one
// session for project dir `directory` with a single user text part `content`.
// Returns the db path and its session id.
func newTestOpenCodeDB(t *testing.T, directory, content string) (string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	const schema = `CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, path TEXT, title TEXT, time_updated INTEGER);
CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, data TEXT);
CREATE TABLE part (id TEXT PRIMARY KEY, session_id TEXT, message_id TEXT, data TEXT, time_created INTEGER);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	sid := "ses_synthetic1"
	if _, err := db.Exec(`INSERT INTO session (id, directory, path, title, time_updated) VALUES (?, ?, '', ?, ?)`,
		sid, directory, "合成会话", time.Now().UnixMilli()); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO message (id, session_id, data) VALUES (?, ?, ?)`,
		"msg1", sid, `{"role":"user"}`); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part (id, session_id, message_id, data, time_created) VALUES (?, ?, ?, ?, ?)`,
		"part1", sid, "msg1", `{"type":"text","text":"`+content+`"}`, time.Now().UnixMilli()); err != nil {
		t.Fatalf("insert part: %v", err)
	}
	return dbPath, sid
}

func TestAutoSalvage_FromOpencode(t *testing.T) {
	root := t.TempDir()
	dbPath, sid := newTestOpenCodeDB(t, filepath.ToSlash(root), "继续做GoOn")
	app := &App{Root: root, Cfg: config.Config{HandoffDir: ".goon/handoffs", SalvageDir: ".goon/salvage"},
		Now: func() time.Time { return time.Unix(0, 0) }}
	app.roots = discover.Roots{OpenCodeDB: dbPath}

	name, err := app.AutoSalvage()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".goon", "salvage", name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "继续做GoOn") {
		t.Fatalf("salvage missing content:\n%s", data)
	}
	if !strings.Contains(string(data), sid) {
		t.Fatalf("salvage header should name the session %q:\n%s", sid, data)
	}
}

func TestParseCandidate_SQLiteRefAndBadRef(t *testing.T) {
	root := t.TempDir()
	dbPath, sid := newTestOpenCodeDB(t, filepath.ToSlash(root), "显式调用")
	app := &App{Root: root, Cfg: config.Config{HandoffDir: ".goon/handoffs", SalvageDir: ".goon/salvage"},
		Now: func() time.Time { return time.Unix(0, 0) }}

	m, err := app.parseCandidate(discover.Candidate{Client: "opencode", Kind: discover.KindSQLite, Ref: dbPath + "#" + sid})
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "opencode" || m.SessionID != sid || !strings.Contains(m.Transcript(), "显式调用") {
		t.Fatalf("bad model: %+v", m)
	}
	if _, err := app.parseCandidate(discover.Candidate{Client: "opencode", Kind: discover.KindSQLite, Ref: dbPath}); err == nil {
		t.Fatal("expected error for sqlite ref without '#'")
	}
	// Explicit source path routes through the configured root as well.
	app.roots = discover.Roots{OpenCodeDB: dbPath}
	if _, err := app.parseSession(sid, "opencode"); err != nil {
		t.Fatalf("parseSession opencode: %v", err)
	}
	if _, err := app.parseSession("x", "gemini"); err == nil || !strings.Contains(err.Error(), "zcode") {
		t.Fatalf("unsupported source message should list all four clients: %v", err)
	}
}

func TestAuto_NoSessionsErrors(t *testing.T) {
	root := t.TempDir()
	app := &App{Root: root, Cfg: config.Config{HandoffDir: ".goon/handoffs", SalvageDir: ".goon/salvage"},
		Now: func() time.Time { return time.Unix(0, 0) }}
	app.roots = discover.Roots{ // nothing exists behind these paths
		ClaudeProjects: filepath.Join(root, "nope-claude"),
		CodexSessions:  filepath.Join(root, "nope-codex"),
		OpenCodeDB:     filepath.Join(root, "nope.db"),
		ZcodeDB:        filepath.Join(root, "nope-z.db"),
	}
	if _, err := app.AutoSalvage(); err == nil {
		t.Fatal("expected error when nothing discovered")
	}
	if _, err := app.AutoDistill(); err == nil {
		t.Fatal("expected error when nothing discovered (distill)")
	}
}

func TestWithRootDefaults(t *testing.T) {
	cfg := withRootDefaults(config.Config{ClaudeProjects: "D:/custom"}, `C:\Users\me`)
	if cfg.ClaudeProjects != "D:/custom" {
		t.Fatalf("explicit value must survive, got %q", cfg.ClaudeProjects)
	}
	if cfg.CodexSessions != filepath.Join(`C:\Users\me`, ".codex", "sessions") {
		t.Fatalf("codex_sessions not defaulted: %q", cfg.CodexSessions)
	}
	if cfg.OpenCodeDB == "" || cfg.ZcodeDB == "" {
		t.Fatalf("opencode/zcode roots should default: %+v", cfg)
	}
	// No home: leave everything empty rather than invent paths.
	if got := withRootDefaults(config.Config{}, ""); got.OpenCodeDB != "" {
		t.Fatalf("empty home should not default roots, got %q", got.OpenCodeDB)
	}
}
