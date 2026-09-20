// Package gitrepo records git state on save and computes a human-readable
// drift report on resume, degrading gracefully for non-git dirs or a base
// commit that has disappeared (rebase/squash).
package gitrepo

import (
	"fmt"
	"os/exec"
	"strings"
)

// State is the git snapshot captured at handoff-save time.
// (Named State rather than Snapshot because the function Snapshot and its
// return type cannot share an identifier at package scope in Go.)
type State struct {
	Branch     string
	Commit     string
	Dirty      bool
	DirtyFiles []string
	IsRepo     bool
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Capture stdout only: stderr carries "fatal: ..." text (e.g. an empty-repo
	// HEAD lookup) that must never leak into a snapshot or the resume prompt.
	out, err := cmd.Output()
	// Trim only trailing whitespace: git's porcelain lines carry a leading
	// status column (e.g. " M path") whose spaces are positionally significant.
	return strings.TrimRight(string(out), " \t\r\n"), err
}

// Snapshot captures current branch/commit/dirty state. Non-git dirs return
// State{IsRepo:false} with no error. A repo without any commit yet keeps
// Commit/Branch empty rather than surfacing git's stderr.
func Snapshot(dir string) (State, error) {
	inside, err := run(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return State{}, nil
	}
	s := State{IsRepo: true}
	if b, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		s.Branch = b
	}
	if c, err := run(dir, "rev-parse", "--verify", "HEAD"); err == nil {
		s.Commit = c
	}
	por, _ := run(dir, "status", "--porcelain")
	if por != "" {
		s.Dirty = true
		for _, line := range strings.Split(por, "\n") {
			if len(line) > 3 {
				s.DirtyFiles = append(s.DirtyFiles, strings.TrimSpace(line[3:]))
			}
		}
	}
	return s, nil
}

// Report is the result of comparing a recorded base commit to current HEAD.
// (Named Report rather than Drift for the same package-scope reason as State.)
type Report struct {
	Body        string
	NewCommits  int
	BaseMissing bool
}

// Drift compares baseCommit..HEAD. baseBranch is the branch recorded at handoff
// time (warns if it changed). since (RFC3339) is a fallback used only when the
// base commit is missing (rebase/squash).
func Drift(dir, baseCommit, baseBranch, since string) (Report, error) {
	cur, _ := Snapshot(dir)
	var d Report
	var b strings.Builder

	if baseCommit == "" || !cur.IsRepo {
		return Report{Body: "✓ 未使用 git 或无基准，跳过对账。"}, nil
	}
	if _, err := run(dir, "cat-file", "-e", baseCommit+"^{commit}"); err != nil {
		d.BaseMissing = true
		fmt.Fprintf(&b, "⚠️ 找不到基准 commit（可能 rebase/squash）。近期提交：\n")
		log, _ := run(dir, "log", "--oneline", "--since="+orStr(since, "1 day ago"))
		b.WriteString(ind(log))
		return Report{Body: b.String(), BaseMissing: true}, nil
	}

	log, _ := run(dir, "log", "--oneline", baseCommit+"..HEAD")
	if log != "" {
		d.NewCommits = len(strings.Split(log, "\n"))
	}
	names, _ := run(dir, "diff", "--name-status", baseCommit, "HEAD")
	fmt.Fprintf(&b, "当前分支: %s\n", cur.Branch)
	fmt.Fprintf(&b, "基准 %s → HEAD %s，%d 个新提交\n", short(baseCommit), short(cur.Commit), d.NewCommits)
	if baseBranch != "" && cur.Branch != "" && cur.Branch != baseBranch {
		fmt.Fprintf(&b, "⚠️ 分支变化: %s → %s\n", baseBranch, cur.Branch)
	}
	if names != "" {
		b.WriteString("变更文件:\n")
		b.WriteString(ind(names))
	}
	if cur.Dirty {
		fmt.Fprintf(&b, "⚠️ 当前工作区有 %d 个未提交改动（勿直接覆盖）:\n", len(cur.DirtyFiles))
		b.WriteString(ind(strings.Join(cur.DirtyFiles, "\n")))
	}
	if d.NewCommits == 0 && names == "" && !cur.Dirty {
		b.WriteString("✓ 自上次交接后无代码变动。\n")
	}
	d.Body = b.String()
	return d, nil
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func short(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func ind(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}
