package extract

import "goon/internal/sqliteopen"

// ParseZcodeDB loads one zcode session into a SessionModel.
func ParseZcodeDB(dbPath, sessionID string) (SessionModel, error) {
	db, err := sqliteopen.Open(dbPath)
	if err != nil {
		return SessionModel{}, err
	}
	defer db.Close()
	return loadSQLite(db, sessionID, "zcode")
}

// ListZcodeSessions returns zcode sessions for a project dir, newest first.
func ListZcodeSessions(dbPath, cwd string) ([]SessionSummary, error) {
	db, err := sqliteopen.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return listSQLite(db, cwd)
}
