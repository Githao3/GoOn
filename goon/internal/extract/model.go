package extract

import (
	"fmt"
	"strings"
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

// Transcript produces a trimmed plain-text timeline used as the distill prompt body.
func (m SessionModel) Transcript() string {
	var b strings.Builder
	for _, e := range m.Events {
		switch e.Kind {
		case "tool":
			if e.FileEdit != nil {
				fmt.Fprintf(&b, "TOOL %s %s (+%d -%d)\n", e.ToolName, e.FileEdit.Path, e.FileEdit.Added, e.FileEdit.Removed)
			} else {
				fmt.Fprintf(&b, "TOOL %s\n", e.ToolName)
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", strings.ToUpper(e.Kind), strings.TrimSpace(e.Text))
		}
	}
	return b.String()
}
