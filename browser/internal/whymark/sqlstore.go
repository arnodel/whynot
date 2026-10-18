//go:build !js

package whymark

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the "sqlite" driver, in pure Go
)

// SQLiteStore is a Store keeping pages in a SQLite database, which also
// lists a document's pages (see PageLister), as the tree of what a reader
// did needs. It's safe for several programs at once.
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLiteStore opens the database at path, making it if need be.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Waiting rather than failing when another program writes, and the
	// write-ahead log, so that readers and a writer don't block each
	// other.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pages (
			id       TEXT PRIMARY KEY,
			document TEXT NOT NULL,
			parent   TEXT NOT NULL DEFAULT '',
			passage  TEXT NOT NULL DEFAULT '',
			title    TEXT NOT NULL DEFAULT '',
			-- Times are ISO 8601, in UTC, always with nine digits of
			-- fraction, so that they sort as text (see timeText).
			created_at      TEXT NOT NULL,
			last_visited_at TEXT NOT NULL,
			location TEXT NOT NULL,
			state    TEXT,             -- as encodeValue gives it, in JSON
			notes    TEXT,
			input    TEXT,
			output   TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS pages_by_document ON pages (document, created_at);
		CREATE INDEX IF NOT EXISTS pages_by_parent ON pages (parent, created_at);
		CREATE INDEX IF NOT EXISTS pages_by_last_visit ON pages (last_visited_at);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("making %s: %w", path, err)
	}
	return &SQLiteStore{db: db}, nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) SavePage(id string, p SavedPage) error {
	state, err := json.Marshal(p.State)
	if err != nil {
		return err
	}
	notes, err := json.Marshal(p.Notes)
	if err != nil {
		return err
	}
	input, err := json.Marshal(p.Input)
	if err != nil {
		return err
	}
	// Its creation is its first visit; saving it again keeps its later
	// ones.
	created := timeText(p.Time)
	_, err = s.db.Exec(`INSERT INTO pages
		(id, document, parent, passage, title, created_at, last_visited_at, location, state, notes, input, output)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			document = excluded.document, parent = excluded.parent, passage = excluded.passage,
			title = excluded.title, created_at = excluded.created_at, location = excluded.location,
			state = excluded.state, notes = excluded.notes, input = excluded.input, output = excluded.output,
			last_visited_at = max(last_visited_at, excluded.last_visited_at)`,
		id, p.Document, p.Parent, p.Passage, p.Title, created, created, p.Location,
		string(state), string(notes), string(input), p.Output)
	return err
}

// Visit records that the page id was visited at t.
func (s *SQLiteStore) Visit(id string, t time.Time) error {
	_, err := s.db.Exec(`UPDATE pages SET last_visited_at = max(last_visited_at, ?) WHERE id = ?`, timeText(t), id)
	return err
}

// timeLayout is how times are kept: ISO 8601, in UTC, always with nine
// digits of fraction, so that they sort as text and SQLite's date
// functions read them.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func timeText(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTimeText(s string) (time.Time, error) { return time.Parse(timeLayout, s) }

func (s *SQLiteStore) LoadPage(id string) (SavedPage, bool, error) {
	var p SavedPage
	var created string
	var state, notes, input sql.NullString
	err := s.db.QueryRow(`SELECT document, parent, passage, title, created_at, location, state, notes, input, output
		FROM pages WHERE id = ?`, id).
		Scan(&p.Document, &p.Parent, &p.Passage, &p.Title, &created, &p.Location, &state, &notes, &input, &p.Output)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	if p.Time, err = parseTimeText(created); err != nil {
		return p, true, fmt.Errorf("page %s: %w", id, err)
	}
	// Decoded as LoadPage's JSON is, so that integers stay integers.
	for _, f := range []struct {
		text sql.NullString
		into any
	}{{state, &p.State}, {notes, &p.Notes}, {input, &p.Input}} {
		if !f.text.Valid {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(f.text.String))
		dec.UseNumber()
		if err := dec.Decode(f.into); err != nil {
			return p, true, fmt.Errorf("page %s: %w", id, err)
		}
	}
	return p, true, nil
}

// Pages lists the pages of document, by when they were made.
func (s *SQLiteStore) Pages(document string) ([]PageInfo, error) {
	rows, err := s.db.Query(`SELECT id, parent, passage, title, created_at, last_visited_at FROM pages
		WHERE document = ? ORDER BY created_at, id`, document)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []PageInfo
	for rows.Next() {
		var p PageInfo
		var created, visited string
		if err := rows.Scan(&p.ID, &p.Parent, &p.Passage, &p.Title, &created, &visited); err != nil {
			return nil, err
		}
		if p.Time, err = parseTimeText(created); err != nil {
			return nil, err
		}
		if p.LastVisited, err = parseTimeText(visited); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
