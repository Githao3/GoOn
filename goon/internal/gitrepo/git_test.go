package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func g(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=x", "GIT_AUTHOR_EMAIL=x@x", "GIT_COMMITTER_NAME=x", "GIT_COMMITTER_EMAIL=x@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	dir := t.TempDir()
	g(t, dir, "init", "-q")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n"), 0o644)
	g(t, dir, "add", "a.txt")
	g(t, dir, "commit", "-qm", "init")
	return dir
}

func TestSnapshot_CleanThenDirty(t *testing.T) {
	dir := newRepo(t)
	s, err := Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dirty || s.Commit == "" {
		t.Fatalf("expected clean snapshot, got %+v", s)
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("2\n"), 0o644)
	s2, _ := Snapshot(dir)
	if !s2.Dirty || len(s2.DirtyFiles) != 1 || s2.DirtyFiles[0] != "a.txt" {
		t.Fatalf("expected dirty a.txt, got %+v", s2)
	}
}

func TestDrift_DetectsChangesSince(t *testing.T) {
	dir := newRepo(t)
	base, _ := Snapshot(dir)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new\n"), 0o644)
	g(t, dir, "add", "b.txt")
	g(t, dir, "commit", "-qm", "add b")
	d, err := Drift(dir, base.Commit, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Body, "b.txt") || d.NewCommits != 1 {
		t.Fatalf("drift should list b.txt and 1 new commit: %+v", d)
	}
}

func TestDrift_MissingBaseDegrades(t *testing.T) {
	dir := newRepo(t)
	d, err := Drift(dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "2020-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Body, "基准 commit") || !strings.Contains(d.Body, "rebase/squash") {
		t.Fatalf("missing base should degrade with hint, got: %q", d.Body)
	}
}
