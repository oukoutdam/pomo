package pomo

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "history.sqlite3")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db}
	if err := store.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize() error {
	if _, err := s.db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return err
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("history schema %d is newer than this pomo binary supports", version)
	}
	if version == 1 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY,
			kind TEXT NOT NULL CHECK(kind IN ('focus','break')),
			started_ns INTEGER NOT NULL,
			planned_ns INTEGER NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS spans (
			session_id INTEGER NOT NULL REFERENCES sessions(id),
			ordinal INTEGER NOT NULL,
			start_ns INTEGER NOT NULL,
			end_ns INTEGER NOT NULL CHECK(end_ns >= start_ns),
			PRIMARY KEY(session_id, ordinal)
		);
		CREATE INDEX IF NOT EXISTS spans_time ON spans(start_ns, end_ns);
		PRAGMA user_version=1;
	`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

// Recover is only called while holding the exclusive timer lock. It never counts
// the time between the last checkpoint and the next application launch.
func (s *Store) Recover() error {
	_, err := s.db.Exec(`UPDATE sessions SET state = CASE WHEN completed = 1 THEN 'completed' ELSE 'interrupted' END WHERE state NOT IN ('completed','interrupted')`)
	return err
}

func (s *Store) Save(session *Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id := session.ID
	if id == 0 {
		result, err := tx.Exec(`INSERT INTO sessions(kind,started_ns,planned_ns,completed,state) VALUES(?,?,?,?,?)`, session.Kind, session.Started.UnixNano(), int64(session.Planned), session.Completed, session.State)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE sessions SET completed=?,state=? WHERE id=?`, session.Completed, session.State, id); err != nil {
			return err
		}
	}
	for i, span := range session.Spans {
		if _, err := tx.Exec(`INSERT INTO spans(session_id,ordinal,start_ns,end_ns) VALUES(?,?,?,?) ON CONFLICT(session_id,ordinal) DO UPDATE SET end_ns=excluded.end_ns`, id, i, span.Start.UnixNano(), span.End.UnixNano()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	session.ID = id
	return nil
}

func (s *Store) FocusSpans() ([]Span, error) {
	rows, err := s.db.Query(`SELECT start_ns,end_ns FROM spans JOIN sessions ON sessions.id=spans.session_id WHERE kind='focus' ORDER BY start_ns`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spans []Span
	for rows.Next() {
		var start, end int64
		if err := rows.Scan(&start, &end); err != nil {
			return nil, err
		}
		spans = append(spans, Span{time.Unix(0, start), time.Unix(0, end)})
	}
	return spans, rows.Err()
}

func (s *Store) CompletedFocus() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE kind='focus' AND completed=1`).Scan(&count)
	return count, err
}
