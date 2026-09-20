package extract

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// normalizeDir canonicalizes a path for cross-client project matching
// (opencode uses forward slashes, zcode backslashes; case-insensitive on Windows).
func normalizeDir(s string) string {
	return strings.Trim(strings.ToLower(filepath.ToSlash(strings.TrimSpace(s))), "/")
}

// loadSQLite builds a SessionModel from a client DB for one session using the
// shared schema (message.role + part text/tool + todo).
func loadSQLite(db *sql.DB, sessionID, source string) (SessionModel, error) {
	m := SessionModel{Source: source, SessionID: sessionID}

	var dir sql.NullString
	if err := db.QueryRow(`SELECT directory FROM session WHERE id = ?`, sessionID).Scan(&dir); err == nil {
		m.Project.CWD = dir.String
	}

	if rows, err := db.Query(`
		SELECT json_extract(m.data,'$.role') role, json_extract(p.data,'$.text') text
		FROM part p JOIN message m ON p.message_id = m.id
		WHERE p.session_id = ? AND json_extract(p.data,'$.type') = 'text'
		ORDER BY p.time_created`, sessionID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var role, text sql.NullString
			if rows.Scan(&role, &text) == nil && text.Valid && text.String != "" {
				kind := role.String
				if kind == "" {
					kind = "assistant"
				}
				m.Events = append(m.Events, Event{Kind: kind, Text: text.String})
			}
		}
	}

	if rows, err := db.Query(`
		SELECT json_extract(data,'$.tool') tool, json_extract(data,'$.state.input.file_path') fp
		FROM part
		WHERE session_id = ? AND json_extract(data,'$.type') = 'tool'
		ORDER BY time_created`, sessionID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var tool, fp sql.NullString
			if rows.Scan(&tool, &fp) == nil && fp.Valid && fp.String != "" {
				m.Events = append(m.Events, Event{Kind: "tool", ToolName: tool.String, FileEdit: &FileEdit{Path: fp.String}})
			}
		}
	}

	if rows, err := db.Query(`SELECT content FROM todo WHERE session_id = ? ORDER BY position`, sessionID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var c string
			if rows.Scan(&c) == nil && c != "" {
				m.Todos = append(m.Todos, c)
			}
		}
	}
	return m, nil
}

// listSQLite returns sessions whose directory matches cwd (normalized), newest first.
func listSQLite(db *sql.DB, cwd string) ([]SessionSummary, error) {
	target := normalizeDir(cwd)
	rows, err := db.Query(`SELECT id, title, directory, time_updated FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var id, title, directory sql.NullString
		var tu sql.NullInt64
		if rows.Scan(&id, &title, &directory, &tu) != nil {
			continue
		}
		if normalizeDir(directory.String) != target {
			continue
		}
		out = append(out, SessionSummary{ID: id.String, Title: title.String, Modified: msTime(tu.Int64)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

func msTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
