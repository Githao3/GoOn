package extract

import (
	"bufio"
	"encoding/json"
	"os"
)

type claudeRec struct {
	Type        string          `json:"type"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	Cwd         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
}

type claudeMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ParseClaudeFile reads one Claude Code session JSONL into a SessionModel.
func ParseClaudeFile(path string) (SessionModel, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionModel{}, err
	}
	defer f.Close()

	m := SessionModel{Source: "claude-code"}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec claudeRec
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.SessionID != "" {
			m.SessionID = rec.SessionID
		}
		if rec.Cwd != "" {
			m.Project.CWD = rec.Cwd
		}
		if rec.GitBranch != "" {
			m.Project.Branch = rec.GitBranch
		}
		if rec.IsSidechain || (rec.Type != "user" && rec.Type != "assistant") || len(rec.Message) == 0 {
			continue
		}
		var msg claudeMsg
		if json.Unmarshal(rec.Message, &msg) != nil {
			continue
		}
		m.Events = append(m.Events, parseClaudeContent(rec, msg)...)
	}
	return m, sc.Err()
}

func parseClaudeContent(rec claudeRec, msg claudeMsg) []Event {
	var s string
	if json.Unmarshal(msg.Content, &s) == nil {
		return []Event{{Kind: msg.Role, Text: s, Timestamp: rec.Timestamp}}
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if bl.Text != "" {
				out = append(out, Event{Kind: msg.Role, Text: bl.Text, Timestamp: rec.Timestamp})
			}
		case "tool_use":
			out = append(out, Event{Kind: "tool", ToolName: bl.Name, FileEdit: claudeFileEdit(bl), Timestamp: rec.Timestamp})
		}
	}
	return out
}

func claudeFileEdit(bl claudeBlock) *FileEdit {
	var in struct {
		FilePath string `json:"file_path"`
	}
	if json.Unmarshal(bl.Input, &in) == nil && in.FilePath != "" {
		return &FileEdit{Path: in.FilePath}
	}
	return nil
}
