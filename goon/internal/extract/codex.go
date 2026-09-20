package extract

import (
	"bufio"
	"encoding/json"
	"os"
)

type codexRec struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type codexMeta struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

type codexMsg struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// ParseCodexFile reads one Codex rollout JSONL into a SessionModel.
func ParseCodexFile(path string) (SessionModel, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionModel{}, err
	}
	defer f.Close()

	m := SessionModel{Source: "codex"}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec codexRec
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		switch rec.Type {
		case "session_meta":
			var meta codexMeta
			if json.Unmarshal(rec.Payload, &meta) == nil {
				if meta.SessionID != "" {
					m.SessionID = meta.SessionID
				}
				if meta.Cwd != "" {
					m.Project.CWD = meta.Cwd
				}
			}
		case "response_item":
			var msg codexMsg
			if json.Unmarshal(rec.Payload, &msg) == nil && msg.Type == "message" {
				var text string
				for _, c := range msg.Content {
					if c.Type == "input_text" || c.Type == "output_text" {
						text += c.Text
					}
				}
				if text != "" {
					m.Events = append(m.Events, Event{Kind: msg.Role, Text: text})
				}
			}
		}
	}
	return m, sc.Err()
}
