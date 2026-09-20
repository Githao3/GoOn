package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"goon/internal/handoff"
)

type Store struct {
	root        string
	dir         string
	base        string
	handoffsDir string
	salvageDir  string
}

// Init creates the .goon layout (idempotent) and returns a usable Store.
func Init(root, dir string) (*Store, error) {
	s := &Store{root: root, dir: dir}
	s.base = filepath.Join(root, dir)
	s.handoffsDir = filepath.Join(s.base, "handoffs")
	s.salvageDir = filepath.Join(s.base, "salvage")
	for _, d := range []string{s.base, s.handoffsDir, s.salvageDir} {
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

// SalvageDir is the gitignored directory where raw salvage drafts are written.
func (s *Store) SalvageDir() string { return s.salvageDir }

func (s *Store) path(id string) string { return filepath.Join(s.handoffsDir, id+".md") }

// Save writes a handoff: if an earlier one exists and this one has no explicit
// Supersedes, it is chained to the current latest, then index.md is refreshed.
// Chaining is forward-only: re-saving an older entry (the finalize path) must
// never re-point it at a newer one, which would form a retrograde cycle.
func (s *Store) Save(h handoff.Handoff, now time.Time) error {
	if prev, ok, _ := s.latestUnlocked(); ok && h.FM.Supersedes == "" && prev < h.FM.ID {
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
		fmt.Fprintf(&b, "- `%s`  [%s]  git:%s\n", printOnly(h.FM.ID), printOnly(h.FM.Source), printOnly(h.FM.Git.Commit))
	}
	return os.WriteFile(filepath.Join(s.base, "index.md"), []byte(b.String()), 0o644)
}

// printOnly neutralizes non-printable runes (control chars, newlines) in
// untrusted frontmatter before they reach index.md, preventing line injection.
func printOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, s)
}
