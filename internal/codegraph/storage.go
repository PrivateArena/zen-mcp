package codegraph

import (
	"database/sql"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

// Storage persists codegraph data in SQLite.
type Storage struct {
	db *sql.DB
	mu sync.Mutex
}

// NewStorage opens or creates the codegraph database.
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, err
	}

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Storage) initSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			hash TEXT NOT NULL,
			mtime INTEGER NOT NULL,
			language TEXT,
			is_test INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			language TEXT,
			qualified_name TEXT,
			signature TEXT,
			docstring TEXT,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			content TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			target_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
			relation TEXT NOT NULL,
			metadata TEXT
		);
		CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
			name, qualified_name, content, docstring, signature,
			content='nodes', content_rowid='id'
		);
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		return err
	}

	// Triggers for FTS5
	_, err = s.db.Exec(`
		DROP TRIGGER IF EXISTS nodes_ai;
		CREATE TRIGGER nodes_ai AFTER INSERT ON nodes BEGIN
			INSERT INTO nodes_fts(rowid, name, qualified_name, content, docstring, signature)
			VALUES (new.id, new.name, new.qualified_name, new.content, new.docstring, new.signature);
		END;
		DROP TRIGGER IF EXISTS nodes_ad;
		CREATE TRIGGER nodes_ad AFTER DELETE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, name, qualified_name, content, docstring, signature)
			VALUES ('delete', old.id, old.name, old.qualified_name, old.content, old.docstring, old.signature);
		END;
		DROP TRIGGER IF EXISTS nodes_au;
		CREATE TRIGGER nodes_au AFTER UPDATE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, name, qualified_name, content, docstring, signature)
			VALUES ('delete', old.id, old.name, old.qualified_name, old.content, old.docstring, old.signature);
			INSERT INTO nodes_fts(rowid, name, qualified_name, content, docstring, signature)
			VALUES (new.id, new.name, new.qualified_name, new.content, new.docstring, new.signature);
		END;
	`)
	if err != nil {
		return err
	}

	// Indexes
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_id);
		CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
		CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
		CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
	`)
	if err != nil {
		return err
	}

	return nil
}

// UpsertFile inserts or updates a file record and returns its ID.
func (s *Storage) UpsertFile(fr FileRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO files (path, hash, mtime, language, is_test)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			hash = excluded.hash,
			mtime = excluded.mtime,
			language = excluded.language,
			is_test = excluded.is_test
		RETURNING id
	`, fr.Path, fr.Hash, fr.MTime, fr.Language, boolToInt(fr.IsTest)).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetFileByPath returns a file record by path.
func (s *Storage) GetFileByPath(path string) *FileRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var fr FileRecord
	err := s.db.QueryRow(`SELECT path, hash, mtime, language, is_test FROM files WHERE path = ?`, path).
		Scan(&fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest)
	if err != nil {
		return nil
	}
	return &fr
}

// DeleteFile removes a file and its nodes/edges.
func (s *Storage) DeleteFile(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

// InsertNode inserts a single node and returns its ID.
func (s *Storage) InsertNode(node NodeRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO nodes (file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, node.FileID, node.Type, node.Name, node.Language, node.QualifiedName, node.Signature, node.Docstring, node.StartLine, node.EndLine, node.Content).
		Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteNodesForFile removes all nodes for a file.
func (s *Storage) DeleteNodesForFile(fileID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM nodes WHERE file_id = ?`, fileID)
	return err
}

// InsertEdge inserts an edge.
func (s *Storage) InsertEdge(sourceID, targetID int64, relation, metadata string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO edges (source_id, target_id, relation, metadata) VALUES (?, ?, ?, ?)`,
		sourceID, targetID, relation, metadata)
	return err
}

// DeleteEdgesForFile removes edges for nodes in a file.
func (s *Storage) DeleteEdgesForFile(fileID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM edges WHERE source_id IN (SELECT id FROM nodes WHERE file_id = ?) OR target_id IN (SELECT id FROM nodes WHERE file_id = ?)`, fileID, fileID)
	return err
}

// SearchFTS searches the FTS5 index.
func (s *Storage) SearchFTS(query string) ([]NodeSearchResult, error) {
	sanitized := sanitizeFtsQuery(query)
	rows, err := s.db.Query(`
		SELECT n.id, n.name, n.type, f.path, n.start_line, n.end_line
		FROM nodes_fts fts
		JOIN nodes n ON fts.rowid = n.id
		JOIN files f ON n.file_id = f.id
		WHERE nodes_fts MATCH ?
		ORDER BY rank
		LIMIT 50
	`, sanitized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []NodeSearchResult
	for rows.Next() {
		var r NodeSearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Path, &r.StartLine, &r.EndLine); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// FindNodesByName finds nodes by name or qualified name.
func (s *Storage) FindNodesByName(name string) ([]NodeRecord, error) {
	rows, err := s.db.Query(`SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content FROM nodes WHERE name = ? OR qualified_name = ?`, name, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// GetNeighbors returns callers and callees for a node.
func (s *Storage) GetNeighbors(nodeID int64, limit int) (callers []NodeRecord, callees []NodeRecord, err error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT n.id, n.file_id, n.type, n.name, n.language, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM edges e
		JOIN nodes n ON e.source_id = n.id
		WHERE e.target_id = ?
		ORDER BY n.start_line
		LIMIT ?
	`, nodeID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	callers, err = scanNodes(rows)
	if err != nil {
		return nil, nil, err
	}

	rows, err = s.db.Query(`
		SELECT DISTINCT n.id, n.file_id, n.type, n.name, n.language, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM edges e
		JOIN nodes n ON e.target_id = n.id
		WHERE e.source_id = ?
		ORDER BY n.start_line
		LIMIT ?
	`, nodeID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	callees, err = scanNodes(rows)
	if err != nil {
		return nil, nil, err
	}

	return callers, callees, nil
}

// ListFiles returns indexed files, optionally filtered by language.
func (s *Storage) ListFiles(filter string, limit int) ([]FileRecord, error) {
	var rows *sql.Rows
	var err error
	if filter != "" {
		if limit > 0 {
			rows, err = s.db.Query(`SELECT path, hash, mtime, language, is_test FROM files WHERE language = ? ORDER BY path LIMIT ?`, filter, limit)
		} else {
			rows, err = s.db.Query(`SELECT path, hash, mtime, language, is_test FROM files WHERE language = ? ORDER BY path`, filter)
		}
	} else {
		if limit > 0 {
			rows, err = s.db.Query(`SELECT path, hash, mtime, language, is_test FROM files ORDER BY path LIMIT ?`, limit)
		} else {
			rows, err = s.db.Query(`SELECT path, hash, mtime, language, is_test FROM files ORDER BY path`)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest); err != nil {
			continue
		}
		files = append(files, fr)
	}
	return files, nil
}

// NodeRecord represents a code symbol.
type NodeRecord struct {
	ID            int64
	FileID        int64
	Type          string
	Name          string
	Language      string
	QualifiedName string
	Signature     string
	Docstring     string
	StartLine     int
	EndLine       int
	Content       string
}

// NodeSearchResult is a lightweight search result.
type NodeSearchResult struct {
	ID        int64
	Name      string
	Type      string
	Path      string
	StartLine int
	EndLine   int
}

func scanNodes(rows *sql.Rows) ([]NodeRecord, error) {
	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sanitizeFtsQuery(q string) string {
	q = strings.ReplaceAll(q, "\"", "\"\"")
	if strings.ContainsAny(q, " \t") {
		q = "\"" + q + "\""
	}
	return q
}

// RunInTransaction runs fn in a transaction.
func (s *Storage) RunInTransaction(fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// SetMetadata sets a metadata key/value.
func (s *Storage) SetMetadata(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO metadata (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetMetadata gets a metadata value.
func (s *Storage) GetMetadata(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

// GetAllFiles returns all indexed files.
func (s *Storage) GetAllFiles() []FileRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT path, hash, mtime, language, is_test FROM files`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest); err != nil {
			continue
		}
		files = append(files, fr)
	}
	return files
}
