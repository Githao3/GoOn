package discover

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"goon/internal/extract"
)

type Kind int

const (
	KindJSONL Kind = iota
	KindSQLite
)

// Roots are the per-client session locations for the current machine. Callers
// (the CLI) resolve these from config/home defaults; discover stays pure.
type Roots struct {
	ClaudeProjects string // dir containing per-project subdirs of <uuid>.jsonl
	CodexSessions  string // dir tree of rollout-*.jsonl
	OpenCodeDB     string // path to opencode.db
	ZcodeDB        string // path to zcode db.sqlite
}

type Candidate struct {
	Client   string // "claude-code" | "codex" | "opencode" | "zcode"
	Title    string
	Modified time.Time
	Kind     Kind
	Ref      string // JSONL: absolute file path. SQLite: dbPath + "#" + sessionID
}

// Recent returns matching sessions across all four clients, newest first.
// A source that is missing or errors is skipped (best-effort), never fatal.
func Recent(cwd string, r Roots) []Candidate {
	var out []Candidate
	out = append(out, claudeCandidates(cwd, r.ClaudeProjects)...)
	out = append(out, codexCandidates(cwd, r.CodexSessions)...)
	out = append(out, sqliteCandidates(r.OpenCodeDB, "opencode", cwd, extract.ListOpenCodeSessions)...)
	out = append(out, sqliteCandidates(r.ZcodeDB, "zcode", cwd, extract.ListZcodeSessions)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out
}

// claudeSlug turns a cwd into the observed per-project directory name by
// replacing path separators, the drive colon and whitespace with dashes, 1:1
// (consecutive separators are NOT collapsed — that is Claude's on-disk scheme).
var claudeSlugReplacer = strings.NewReplacer(":", "-", `\`, "-", "/", "-", " ", "-", "\t", "-")

func claudeSlug(cwd string) string {
	return claudeSlugReplacer.Replace(cwd)
}

// claudeCandidates lists <ClaudeProjects>/<slug(cwd)>/*.jsonl as candidates.
func claudeCandidates(cwd, projectsRoot string) []Candidate {
	if projectsRoot == "" {
		return nil
	}
	dir := filepath.Join(projectsRoot, claudeSlug(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		title := claudeTitle(path)
		if title == "" {
			title = e.Name()
		}
		out = append(out, Candidate{
			Client:   "claude-code",
			Title:    title,
			Modified: info.ModTime(),
			Kind:     KindJSONL,
			Ref:      path,
		})
	}
	return out
}

// claudeTitle reads a session file for the first "user" record's text, used as a
// short title. Best-effort: any error yields "".
func claudeTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "user" || len(rec.Message) == 0 {
			continue
		}
		var msg struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(rec.Message, &msg) != nil || len(msg.Content) == 0 {
			continue
		}
		if text := firstClaudeText(msg.Content); text != "" {
			return truncateRunes(text, 60)
		}
	}
	return ""
}

// firstClaudeText extracts a text payload from a Claude message.content, which
// may be a plain string or an array of content blocks.
func firstClaudeText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return strings.TrimSpace(b.Text)
			}
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// codexCandidates walks the codex session tree for rollout-*.jsonl whose
// session_meta cwd matches the target cwd.
func codexCandidates(cwd, sessionsRoot string) []Candidate {
	if sessionsRoot == "" {
		return nil
	}
	target := normalizeCwd(cwd)
	var out []Candidate
	_ = filepath.WalkDir(sessionsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // best-effort: skip unreadable entries
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		metaCwd, ok := codexMetaCwd(path)
		if !ok || metaCwd != target {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, Candidate{
			Client:   "codex",
			Title:    d.Name(),
			Modified: info.ModTime(),
			Kind:     KindJSONL,
			Ref:      path,
		})
		return nil
	})
	return out
}

// codexMetaCwd scans a rollout file until it finds the session_meta record and
// returns its normalized cwd. The second result is false when no meta was found.
func codexMetaCwd(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "session_meta" {
			continue
		}
		var meta struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(rec.Payload, &meta) != nil {
			return "", false
		}
		return normalizeCwd(meta.Cwd), true
	}
	return "", false
}

func normalizeCwd(s string) string {
	return strings.ToLower(filepath.ToSlash(strings.TrimSpace(s)))
}

// sqliteCandidates lists sessions from a client DB matching cwd.
func sqliteCandidates(dbPath, client string, cwd string, list func(string, string) ([]extract.SessionSummary, error)) []Candidate {
	if dbPath == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	sums, err := list(dbPath, cwd)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, s := range sums {
		out = append(out, Candidate{Client: client, Title: s.Title, Modified: s.Modified, Kind: KindSQLite, Ref: dbPath + "#" + s.ID})
	}
	return out
}
