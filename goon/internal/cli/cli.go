package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"goon/internal/config"
	"goon/internal/discover"
	"goon/internal/distill"
	"goon/internal/extract"
	"goon/internal/gitrepo"
	"goon/internal/handoff"
	"goon/internal/llm"
	"goon/internal/redact"
	"goon/internal/resume"
	"goon/internal/store"
)

// sourceSlug constrains a handoff source to a safe, filename-friendly token so
// it can never smuggle a path separator or control char into an id or header.
var sourceSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// App bundles the working directory, resolved config, and an injectable clock.
// roots are the resolved per-client session locations used for auto-discovery.
type App struct {
	Root  string
	Cfg   config.Config
	Now   func() time.Time
	roots discover.Roots
}

// expandHome resolves a leading "~" (as "~", "~/x" or "~\x") against home, so a
// config file written with shell-style paths still points at the real dir. Go
// never expands "~" itself, and an unset home leaves the value untouched.
func expandHome(p, home string) string {
	if home == "" || !strings.HasPrefix(p, "~") {
		return p
	}
	if p == "~" {
		return home
	}
	if len(p) > 1 && (p[1] == '/' || p[1] == '\\') {
		return filepath.Join(home, filepath.ToSlash(p[2:]))
	}
	return p
}

// withRootDefaults expands "~" in every configured root, then fills the EMPTY
// ones from the user's home dir. An empty home leaves those empty (discovery
// then simply finds nothing).
func withRootDefaults(cfg config.Config, home string) config.Config {
	set := func(p *string, def string) {
		if *p == "" {
			*p = def
		}
	}
	cfg.ClaudeProjects = expandHome(cfg.ClaudeProjects, home)
	cfg.CodexSessions = expandHome(cfg.CodexSessions, home)
	cfg.OpenCodeDB = expandHome(cfg.OpenCodeDB, home)
	cfg.ZcodeDB = expandHome(cfg.ZcodeDB, home)
	if home != "" {
		set(&cfg.ClaudeProjects, filepath.Join(home, ".claude", "projects"))
		set(&cfg.CodexSessions, filepath.Join(home, ".codex", "sessions"))
		set(&cfg.OpenCodeDB, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
		set(&cfg.ZcodeDB, filepath.Join(home, ".zcode", "cli", "db", "db.sqlite"))
	}
	return cfg
}

// New builds an App for the given project root, loading global+project config.
func New(root string) *App {
	home := ""
	if h, err := os.UserHomeDir(); err == nil {
		home = h
	}
	cfg := config.Config{HandoffDir: ".goon/handoffs", SalvageDir: ".goon/salvage", DriftVerbosity: "summary", SalvageKeepDays: 7, LLM: config.LLM{Provider: "openai-compatible", APIKeyEnv: "GOON_LLM_KEY", Model: "gpt-4o-mini"}}
	if home != "" {
		c, err := config.Load(filepath.Join(home, ".goon", "config.yaml"), filepath.Join(root, ".goon", "config.yaml"))
		if err != nil {
			// A config file exists but is malformed; missing files are not an
			// error. Warn instead of silently falling back to defaults.
			fmt.Fprintln(os.Stderr, "goon: ignoring invalid config:", err)
		} else {
			cfg = c
		}
	}
	cfg = withRootDefaults(cfg, home)
	return &App{Root: root, Cfg: cfg, Now: time.Now, roots: discover.Roots{
		ClaudeProjects: cfg.ClaudeProjects,
		CodexSessions:  cfg.CodexSessions,
		OpenCodeDB:     cfg.OpenCodeDB,
		ZcodeDB:        cfg.ZcodeDB,
	}}
}

func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now()
	}
	return a.Now()
}

func (a *App) store() (*store.Store, error) {
	return store.Init(a.Root, filepath.Dir(a.Cfg.HandoffDir))
}

func (a *App) Init() error {
	_, err := a.store()
	return err
}

// Distill produces a handoff body via the LLM, captures a git snapshot, chains,
// and stores it. Returns the new handoff id.
func (a *App) Distill(m extract.SessionModel, chat llm.Chat) (string, error) {
	body, err := distill.Run(m, chat)
	if err != nil {
		return "", err
	}
	if miss := handoff.ValidateBody(body); len(miss) > 0 {
		return "", fmt.Errorf("distilled body missing sections: %v", miss)
	}
	// Snapshot before the store is (re)initialised so freshly-created .goon
	// files never show up in the recorded dirty_files.
	snap, _ := gitrepo.Snapshot(a.Root)
	s, err := a.store()
	if err != nil {
		return "", err
	}
	id := handoff.NewID(a.now(), m.Source)
	h := handoff.Handoff{FM: handoff.FrontMatter{
		Goon: 1, ID: id, Created: a.now().UTC().Format(time.RFC3339), Source: m.Source, Project: filepath.Base(a.Root),
		Git: handoff.Git{Branch: snap.Branch, Commit: snap.Commit, Dirty: snap.Dirty, DirtyFiles: snap.DirtyFiles},
	}}
	h.Body = body
	if err := s.Save(h, a.now()); err != nil {
		return "", err
	}
	return id, nil
}

// DistillFile parses one session (source: claude-code|codex take a file path,
// opencode|zcode take a session id) and distills it.
func (a *App) DistillFile(path, source string) (string, error) {
	m, err := a.parseSession(path, source)
	if err != nil {
		return "", err
	}
	return a.Distill(m, llm.New(a.Cfg.LLM))
}

// SalvageFile parses one session and writes a raw (non-LLM) transcript draft.
func (a *App) SalvageFile(path, source string) (string, error) {
	m, err := a.parseSession(path, source)
	if err != nil {
		return "", err
	}
	return a.WriteSalvage(m)
}

func (a *App) parseSession(path, source string) (extract.SessionModel, error) {
	switch source {
	case "claude-code":
		return extract.ParseClaudeFile(path)
	case "codex":
		return extract.ParseCodexFile(path)
	case "opencode":
		return extract.ParseOpenCodeDB(a.roots.OpenCodeDB, path)
	case "zcode":
		return extract.ParseZcodeDB(a.roots.ZcodeDB, path)
	default:
		return extract.SessionModel{}, fmt.Errorf("unsupported source %q (supports claude-code, codex, opencode, zcode)", source)
	}
}

// parseCandidate turns a discovered candidate into a session model. JSONL
// candidates carry the file path in Ref; SQLite candidates use "db#sessionID".
func (a *App) parseCandidate(c discover.Candidate) (extract.SessionModel, error) {
	switch c.Kind {
	case discover.KindSQLite:
		db, id, err := splitSQLiteRef(c.Ref)
		if err != nil {
			return extract.SessionModel{}, err
		}
		if c.Client == "zcode" {
			return extract.ParseZcodeDB(db, id)
		}
		return extract.ParseOpenCodeDB(db, id)
	default: // KindJSONL
		return a.parseSession(c.Ref, c.Client)
	}
}

// splitSQLiteRef splits a discovered SQLite ref into db path + session id on
// the LAST '#': a db path may itself contain '#', and only the final one is
// the separator written by discover (dbPath + "#" + sessionID).
func splitSQLiteRef(ref string) (string, string, error) {
	i := strings.LastIndex(ref, "#")
	if i < 0 {
		return "", "", fmt.Errorf("bad sqlite ref %q", ref)
	}
	return ref[:i], ref[i+1:], nil
}

// recentSession returns the newest discovered session for the project, or error.
func (a *App) recentSession() (extract.SessionModel, error) {
	cands := discover.Recent(a.Root, a.roots)
	if len(cands) == 0 {
		return extract.SessionModel{}, fmt.Errorf("no recent session found for this project (checked claude-code, codex, opencode, zcode)")
	}
	return a.parseCandidate(cands[0])
}

// AutoSalvage writes a raw draft from the most recent discovered session.
func (a *App) AutoSalvage() (string, error) {
	m, err := a.recentSession()
	if err != nil {
		return "", err
	}
	return a.WriteSalvage(m)
}

// AutoDistill distills the most recent discovered session (needs LLM).
func (a *App) AutoDistill() (string, error) {
	m, err := a.recentSession()
	if err != nil {
		return "", err
	}
	return a.Distill(m, llm.New(a.Cfg.LLM))
}

// WriteSalvage writes a redacted raw transcript to the store's gitignored
// salvage dir; returns the filename.
func (a *App) WriteSalvage(m extract.SessionModel) (string, error) {
	s, err := a.store()
	if err != nil {
		return "", err
	}
	name := handoff.NewID(a.now(), m.Source) + ".raw.md"
	content := "# Salvage raw extract — " + printOnly(m.Source) + " " + printOnly(m.SessionID) + "\n\n" +
		redact.Wrap(redact.Redact(m.Transcript())) + "\n"
	full := filepath.Join(s.SalvageDir(), name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// Resume returns the assembled continuation prompt for id (or latest when empty).
func (a *App) Resume(id string) (string, error) {
	s, err := a.store()
	if err != nil {
		return "", err
	}
	if id == "" {
		if id, err = s.LatestID(); err != nil {
			return "", err
		}
	} else if err := safeID(id); err != nil {
		return "", err
	}
	h, err := s.Load(id)
	if err != nil {
		return "", err
	}
	d, err := gitrepo.Drift(a.Root, h.FM.Git.Commit, h.FM.Git.Branch, h.FM.Created)
	if err != nil {
		return "", err
	}
	return resume.Compose(h, d), nil
}

// Finalize validates a stored handoff body and refreshes the index.
func (a *App) Finalize(id string) error {
	if err := safeID(id); err != nil {
		return err
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	h, err := s.Load(id)
	if err != nil {
		return err
	}
	if miss := handoff.ValidateBody(h.Body); len(miss) > 0 {
		return fmt.Errorf("handoff %q missing sections: %v", id, miss)
	}
	return s.Save(h, a.now())
}

// New creates a blank-template handoff (manual save path); returns its id.
func (a *App) New(source string) (string, error) {
	if !sourceSlug.MatchString(source) {
		return "", fmt.Errorf("invalid source %q", source)
	}
	// Snapshot before the store is (re)initialised (see Distill).
	snap, _ := gitrepo.Snapshot(a.Root)
	s, err := a.store()
	if err != nil {
		return "", err
	}
	id := handoff.NewID(a.now(), source)
	if err := safeID(id); err != nil {
		return "", err
	}
	h := handoff.Handoff{FM: handoff.FrontMatter{
		Goon: 1, ID: id, Created: a.now().UTC().Format(time.RFC3339), Source: source, Project: filepath.Base(a.Root),
		Git: handoff.Git{Branch: snap.Branch, Commit: snap.Commit, Dirty: snap.Dirty, DirtyFiles: snap.DirtyFiles},
	}}
	h.Body = handoff.BlankBody()
	return id, s.Save(h, a.now())
}

func (a *App) List() (string, error) {
	s, err := a.store()
	if err != nil {
		return "", err
	}
	ids, err := s.List()
	if err != nil {
		return "", err
	}
	return strings.Join(ids, "\n"), nil
}

func (a *App) Status() (string, error) {
	snap, _ := gitrepo.Snapshot(a.Root)
	latest := "-"
	if s, err := a.store(); err == nil {
		if id, e := s.LatestID(); e == nil {
			latest = id
		}
	}
	return fmt.Sprintf("branch=%s commit=%s dirty=%v latest=%s", snap.Branch, snap.Commit, snap.Dirty, latest), nil
}

// safeID rejects handoff ids that could escape the handoffs dir.
func safeID(id string) error {
	if id == "" {
		return fmt.Errorf("empty id")
	}
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") || id != filepath.Base(id) {
		return fmt.Errorf("invalid handoff id %q", id)
	}
	return nil
}

// printOnly neutralizes non-printable runes in untrusted values before they are
// concatenated into a salvage file header line.
func printOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, s)
}
