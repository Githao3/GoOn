package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goon/internal/handoff"
)

func TestStore_InitSaveChainLatest(t *testing.T) {
	root := t.TempDir()
	s, err := Init(root, ".goon")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"handoffs", "salvage"} {
		if fi, err := os.Stat(filepath.Join(root, ".goon", want)); err != nil || !fi.IsDir() {
			t.Fatalf("missing dir %s", want)
		}
	}
	gi, _ := os.ReadFile(filepath.Join(root, ".goon", ".gitignore"))
	if !strings.Contains(string(gi), "salvage/") {
		t.Fatalf("gitignore should exclude salvage/, got %q", gi)
	}

	h1 := handoff.Handoff{FM: handoff.FrontMatter{Goon: 1, ID: "2026-09-20T10-00-00-claude-code", Source: "claude-code", Git: handoff.Git{Commit: "a"}}, Body: "## 目标\none"}
	if err := s.Save(h1, time.Now()); err != nil {
		t.Fatal(err)
	}
	h2 := handoff.Handoff{FM: handoff.FrontMatter{Goon: 1, ID: "2026-09-20T11-00-00-codex", Source: "codex", Git: handoff.Git{Commit: "b"}}, Body: "## 目标\ntwo"}
	if err := s.Save(h2, time.Now()); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(h2.FM.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FM.Supersedes != h1.FM.ID {
		t.Fatalf("supersedes wrong: %q", loaded.FM.Supersedes)
	}
	if latest, _ := s.LatestID(); latest != h2.FM.ID {
		t.Fatalf("latest should be h2, got %q", latest)
	}
	idx, _ := os.ReadFile(filepath.Join(root, ".goon", "index.md"))
	if !strings.Contains(string(idx), h1.FM.ID) || !strings.Contains(string(idx), h2.FM.ID) {
		t.Fatalf("index should list both: %q", idx)
	}
}
