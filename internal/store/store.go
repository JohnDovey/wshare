package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is a file or directory tracked for sharing.
type Entry struct {
	ID            int64
	Path          string // absolute path on disk
	Name          string // display name (base name)
	IsDir         bool
	Size          int64
	ParentID      sql.NullInt64
	RootID        int64
	AddedAt       time.Time
	DownloadCount int64 // file downloads, or folder zip downloads
	Available     bool  // set when checking disk presence
}

// Root is a top-level path that was shared (file or folder).
type Root struct {
	ID      int64
	Path    string
	IsDir   bool
	Name    string
	AddedAt time.Time
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single writer is fine for this app; avoid lock issues on concurrent requests.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS roots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	is_dir INTEGER NOT NULL DEFAULT 0,
	added_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS entries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	is_dir INTEGER NOT NULL DEFAULT 0,
	size INTEGER NOT NULL DEFAULT 0,
	parent_id INTEGER,
	root_id INTEGER NOT NULL,
	added_at TEXT NOT NULL,
	download_count INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (parent_id) REFERENCES entries(id) ON DELETE CASCADE,
	FOREIGN KEY (root_id) REFERENCES roots(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_entries_root ON entries(root_id);
CREATE INDEX IF NOT EXISTS idx_entries_parent ON entries(parent_id);
`)
	if err != nil {
		return err
	}
	// Existing DBs created before download_count: add the column if missing.
	return s.ensureColumn("entries", "download_count", "INTEGER NOT NULL DEFAULT 0")
}

func (s *Store) ensureColumn(table, column, decl string) error {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl))
	return err
}

// AddRoot registers a root path if not already present. Returns root ID.
func (s *Store) AddRoot(absPath string, isDir bool) (int64, error) {
	absPath = filepath.Clean(absPath)
	name := filepath.Base(absPath)
	now := time.Now().UTC().Format(time.RFC3339)

	var id int64
	err := s.db.QueryRow(`SELECT id FROM roots WHERE path = ?`, absPath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	res, err := s.db.Exec(
		`INSERT INTO roots (path, name, is_dir, added_at) VALUES (?, ?, ?, ?)`,
		absPath, name, boolToInt(isDir), now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpsertEntry inserts or updates an entry under a root.
func (s *Store) UpsertEntry(absPath, name string, isDir bool, size int64, parentID *int64, rootID int64) (int64, error) {
	absPath = filepath.Clean(absPath)
	now := time.Now().UTC().Format(time.RFC3339)

	var id int64
	err := s.db.QueryRow(`SELECT id FROM entries WHERE path = ?`, absPath).Scan(&id)
	if err == nil {
		_, err = s.db.Exec(
			`UPDATE entries SET name = ?, is_dir = ?, size = ?, parent_id = ?, root_id = ? WHERE id = ?`,
			name, boolToInt(isDir), size, nullInt(parentID), rootID, id,
		)
		return id, err
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	res, err := s.db.Exec(
		`INSERT INTO entries (path, name, is_dir, size, parent_id, root_id, added_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		absPath, name, boolToInt(isDir), size, nullInt(parentID), rootID, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListRoots returns all share roots.
func (s *Store) ListRoots() ([]Root, error) {
	rows, err := s.db.Query(`SELECT id, path, name, is_dir, added_at FROM roots ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Root
	for rows.Next() {
		var r Root
		var isDir int
		var added string
		if err := rows.Scan(&r.ID, &r.Path, &r.Name, &isDir, &added); err != nil {
			return nil, err
		}
		r.IsDir = isDir == 1
		r.AddedAt, _ = time.Parse(time.RFC3339, added)
		out = append(out, r)
	}
	return out, rows.Err()
}

const entrySelectCols = `id, path, name, is_dir, size, parent_id, root_id, added_at, download_count`

// ListEntries returns all entries, optionally filtered by root ID (0 = all).
func (s *Store) ListEntries(rootID int64) ([]Entry, error) {
	var rows *sql.Rows
	var err error
	if rootID > 0 {
		rows, err = s.db.Query(
			`SELECT `+entrySelectCols+`
			 FROM entries WHERE root_id = ? ORDER BY is_dir DESC, name COLLATE NOCASE`, rootID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT `+entrySelectCols+`
			 FROM entries ORDER BY root_id, is_dir DESC, name COLLATE NOCASE`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// ListChildren returns direct children of a parent entry (parentID nil/0 for root-level under a share root).
func (s *Store) ListChildren(rootID int64, parentID *int64) ([]Entry, error) {
	var rows *sql.Rows
	var err error
	if parentID == nil || *parentID == 0 {
		rows, err = s.db.Query(
			`SELECT `+entrySelectCols+`
			 FROM entries WHERE root_id = ? AND parent_id IS NULL
			 ORDER BY is_dir DESC, name COLLATE NOCASE`, rootID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT `+entrySelectCols+`
			 FROM entries WHERE root_id = ? AND parent_id = ?
			 ORDER BY is_dir DESC, name COLLATE NOCASE`, rootID, *parentID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// GetEntry returns a single entry by ID.
func (s *Store) GetEntry(id int64) (*Entry, error) {
	row := s.db.QueryRow(
		`SELECT `+entrySelectCols+` FROM entries WHERE id = ?`, id,
	)
	e, err := scanEntry(row)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// IncrementDownloadCount adds one to the entry's download counter
// (file download or folder zip download) and returns the new total.
func (s *Store) IncrementDownloadCount(id int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE entries SET download_count = download_count + 1 WHERE id = ?`, id,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, sql.ErrNoRows
	}
	var count int64
	err = s.db.QueryRow(`SELECT download_count FROM entries WHERE id = ?`, id).Scan(&count)
	return count, err
}

// GetRoot returns a root by ID.
func (s *Store) GetRoot(id int64) (*Root, error) {
	row := s.db.QueryRow(`SELECT id, path, name, is_dir, added_at FROM roots WHERE id = ?`, id)
	var r Root
	var isDir int
	var added string
	if err := row.Scan(&r.ID, &r.Path, &r.Name, &isDir, &added); err != nil {
		return nil, err
	}
	r.IsDir = isDir == 1
	r.AddedAt, _ = time.Parse(time.RFC3339, added)
	return &r, nil
}

// RemoveEntry deletes an entry by ID. Directories cascade via parent links only if we delete children explicitly.
func (s *Store) RemoveEntry(id int64) error {
	e, err := s.GetEntry(id)
	if err != nil {
		return err
	}
	// Remove this entry and any whose path is under this path when it's a directory.
	if e.IsDir {
		_, err = s.db.Exec(`DELETE FROM entries WHERE path = ? OR path LIKE ?`, e.Path, e.Path+string(os.PathSeparator)+"%")
	} else {
		_, err = s.db.Exec(`DELETE FROM entries WHERE id = ?`, id)
	}
	return err
}

// RemoveRoot removes a root and all its entries.
func (s *Store) RemoveRoot(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM entries WHERE root_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM roots WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveByPath removes a root or entry matching path (and descendants for dirs).
func (s *Store) RemoveByPath(absPath string) (int64, error) {
	absPath = filepath.Clean(absPath)

	var rootID int64
	err := s.db.QueryRow(`SELECT id FROM roots WHERE path = ?`, absPath).Scan(&rootID)
	if err == nil {
		if err := s.RemoveRoot(rootID); err != nil {
			return 0, err
		}
		return 1, nil
	}

	var entryID int64
	err = s.db.QueryRow(`SELECT id FROM entries WHERE path = ?`, absPath).Scan(&entryID)
	if err == sql.ErrNoRows {
		// Try prefix delete for directories under a root.
		res, err := s.db.Exec(
			`DELETE FROM entries WHERE path = ? OR path LIKE ?`,
			absPath, absPath+string(os.PathSeparator)+"%",
		)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	if err != nil {
		return 0, err
	}
	if err := s.RemoveEntry(entryID); err != nil {
		return 0, err
	}
	return 1, nil
}

// CleanupMissing removes entries (and empty roots) whose paths no longer exist on disk.
// Returns the number of entries removed.
func (s *Store) CleanupMissing() (int, error) {
	entries, err := s.ListEntries(0)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if _, err := os.Stat(e.Path); os.IsNotExist(err) {
			if _, err := s.db.Exec(`DELETE FROM entries WHERE id = ?`, e.ID); err != nil {
				return removed, err
			}
			removed++
		}
	}
	// Drop roots with no remaining entries and missing on disk.
	roots, err := s.ListRoots()
	if err != nil {
		return removed, err
	}
	for _, r := range roots {
		if _, err := os.Stat(r.Path); os.IsNotExist(err) {
			var count int
			_ = s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE root_id = ?`, r.ID).Scan(&count)
			if count == 0 {
				_, _ = s.db.Exec(`DELETE FROM roots WHERE id = ?`, r.ID)
			}
		}
	}
	return removed, nil
}

// MarkAvailability sets Available on each entry based on disk presence.
func MarkAvailability(entries []Entry) {
	for i := range entries {
		_, err := os.Stat(entries[i].Path)
		entries[i].Available = err == nil
	}
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (*Entry, error) {
	var e Entry
	var isDir int
	var added string
	if err := row.Scan(&e.ID, &e.Path, &e.Name, &isDir, &e.Size, &e.ParentID, &e.RootID, &added, &e.DownloadCount); err != nil {
		return nil, err
	}
	e.IsDir = isDir == 1
	e.AddedAt, _ = time.Parse(time.RFC3339, added)
	return &e, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullInt(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
