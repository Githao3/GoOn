package extract

import (
	"fmt"
	"strings"
	"time"
)

type FileEdit struct {
	Path    string
	Added   int
	Removed int
}

type Event struct {
	Kind      string // "user" | "assistant" | "tool"
	Text      string
	Timestamp string
	ToolName  string
	FileEdit  *FileEdit
}

type Project struct {
	CWD    string
	Branch string
	Commit string
}

type SessionModel struct {
	Source    string
	SessionID string
	Project   Project
	Events    []Event
	Todos     []string
}

// SessionSummary is a discoverable session entry (P2 discovery).
type SessionSummary struct {
	ID       string
	Title    string
	Modified time.Time
}

// Transcript produces a trimmed plain-text timeline used as the distill prompt body.
func (m SessionModel) Transcript() string {
	var b strings.Builder
	for _, e := range m.Events {
		switch e.Kind {
		case "tool":
			if e.FileEdit != nil {
				if e.FileEdit.Added == 0 && e.FileEdit.Removed == 0 {
					fmt.Fprintf(&b, "TOOL %s %s\n", e.ToolName, e.FileEdit.Path)
				} else {
					fmt.Fprintf(&b, "TOOL %s %s (+%d -%d)\n", e.ToolName, e.FileEdit.Path, e.FileEdit.Added, e.FileEdit.Removed)
				}
			} else {
				fmt.Fprintf(&b, "TOOL %s\n", e.ToolName)
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", strings.ToUpper(e.Kind), strings.TrimSpace(e.Text))
		}
	}
	return b.String()
}
