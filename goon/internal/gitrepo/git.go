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
	out, err := cmd.CombinedOutput()
	// Trim only trailing whitespace: git's porcelain lines carry a leading
	// status column (e.g. " M path") whose spaces are positionally significant.
	return strings.TrimRight(string(out), " \t\r\n"), err
}

// Snapshot captures current branch/commit/dirty state. Non-git dirs return
// State{IsRepo:false} with no error.
func Snapshot(dir string) (State, error) {
	if _, err := run(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return State{}, nil
	}
	s := State{IsRepo: true}
	s.Branch, _ = run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	s.Commit, _ = run(dir, "rev-parse", "HEAD")
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

// Drift compares baseCommit..HEAD. since (RFC3339) is a fallback used only when
// the base commit is missing (rebase/squash).
func Drift(dir, baseCommit, since string) (Report, error) {
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
	if names != "" {
		b.WriteString("变更文件:\n")
		b.WriteString(ind(names))
	} else if d.NewCommits == 0 {
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
