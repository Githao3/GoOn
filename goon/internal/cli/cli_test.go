package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goon/internal/config"
	"goon/internal/extract"
	"goon/internal/handoff"
	"goon/internal/store"
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
