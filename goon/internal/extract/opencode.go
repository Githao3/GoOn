package extract

import "goon/internal/sqliteopen"

// ParseOpenCodeDB loads one opencode session into a SessionModel.
func ParseOpenCodeDB(dbPath, sessionID string) (SessionModel, error) {
	db, err := sqliteopen.Open(dbPath)
	if err != nil {
		return SessionModel{}, err
	}
	defer db.Close()
	return loadSQLite(db, sessionID, "opencode")
}

// ListOpenCodeSessions returns opencode sessions for a project dir, newest first.
func ListOpenCodeSessions(dbPath, cwd string) ([]SessionSummary, error) {
	db, err := sqliteopen.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return listSQLite(db, cwd)
}
