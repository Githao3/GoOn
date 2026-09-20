package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"goon/internal/handoff"
)

type Store struct {
	root        string
	dir         string
	base        string
	handoffsDir string
}

// Init creates the .goon layout (idempotent) and returns a usable Store.
func Init(root, dir string) (*Store, error) {
	s := &Store{root: root, dir: dir}
	s.base = filepath.Join(root, dir)
	s.handoffsDir = filepath.Join(s.base, "handoffs")
	for _, d := range []string{s.base, s.handoffsDir, filepath.Join(s.base, "salvage")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(filepath.Join(s.base, ".gitignore")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(s.base, ".gitignore"), []byte("salvage/\n"), 0o644); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) path(id string) string { return filepath.Join(s.handoffsDir, id+".md") }

// Save writes a handoff: if an earlier one exists and this one has no explicit
// Supersedes, it is chained to the current latest, then index.md is refreshed.
func (s *Store) Save(h handoff.Handoff, now time.Time) error {
	if prev, ok, _ := s.latestUnlocked(); ok && h.FM.Supersedes == "" {
		h.FM.Supersedes = prev
	}
	text, err := handoff.Render(h)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path(h.FM.ID), []byte(text), 0o644); err != nil {
		return err
	}
	return s.writeIndex()
}

func (s *Store) Load(id string) (handoff.Handoff, error) {
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return handoff.Handoff{}, err
	}
	return handoff.Parse(string(b))
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.handoffsDir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(ids) // id prefixes are timestamps -> lexical == chronological
	return ids, nil
}

func (s *Store) latestUnlocked() (string, bool, error) {
	ids, err := s.List()
	if err != nil {
		return "", false, err
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[len(ids)-1], true, nil
}

func (s *Store) LatestID() (string, error) {
	id, ok, err := s.latestUnlocked()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no handoffs yet")
	}
	return id, nil
}

func (s *Store) writeIndex() error {
	ids, err := s.List()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# GoOn handoff index\n\n newest-first\n\n")
	for i := len(ids) - 1; i >= 0; i-- {
		h, err := s.Load(ids[i])
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "- `%s`  [%s]  git:%s\n", h.FM.ID, h.FM.Source, h.FM.Git.Commit)
	}
	return os.WriteFile(filepath.Join(s.base, "index.md"), []byte(b.String()), 0o644)
}
