package codegraph

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// Storage persists codegraph data in SQLite.
type Storage struct {
	db    *sql.DB
	mu    sync.Mutex
	stmts map[string]*sql.Stmt
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

	s := &Storage{db: db, stmts: make(map[string]*sql.Stmt)}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	s.prepareStatements()
	return s, nil
}

// Close closes the database.
func (s *Storage) Close() error {
	if s.db != nil {
		for _, stmt := range s.stmts {
			stmt.Close()
		}
		return s.db.Close()
	}
	return nil
}

func (s *Storage) getStmt(key string) *sql.Stmt {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stmts[key]
}

func (s *Storage) setStmt(key string, stmt *sql.Stmt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stmts[key] = stmt
}

func (s *Storage) prepareStatements() {
	var err error
	s.setStmt("upsertFile", mustPrepare(s.db, `
		INSERT INTO files (path, hash, mtime, language, is_test)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			hash = excluded.hash,
			mtime = excluded.mtime,
			language = excluded.language,
			is_test = excluded.is_test
	`))
	s.setStmt("getFileByPath", mustPrepare(s.db, `SELECT id, path, hash, mtime, language, is_test FROM files WHERE path = ?`))
	s.setStmt("insertNode", mustPrepare(s.db, `
		INSERT INTO nodes (file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`))
	s.setStmt("clearNodesForFile", mustPrepare(s.db, `DELETE FROM nodes WHERE file_id = ?`))
	s.setStmt("deleteFile", mustPrepare(s.db, `DELETE FROM files WHERE id = ?`))
	s.setStmt("insertEdge", mustPrepare(s.db, `
		INSERT INTO edges (source_id, target_id, relation, metadata, confidence)
		VALUES (?, ?, ?, ?, ?)
	`))
	s.setStmt("getNodesForFile", mustPrepare(s.db, `SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content FROM nodes WHERE file_id = ?`))
	s.setStmt("getFileById", mustPrepare(s.db, `SELECT id, path, hash, mtime, language, is_test FROM files WHERE id = ?`))
	s.setStmt("getNodeById", mustPrepare(s.db, `SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content FROM nodes WHERE id = ?`))
	s.setStmt("findNodeByName", mustPrepare(s.db, `
		SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content
		FROM nodes
		WHERE qualified_name = ? OR name = ?
		ORDER BY (qualified_name = ?) DESC
		LIMIT 1
	`))
	s.setStmt("findNodeByTypeAndName", mustPrepare(s.db, `
		SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content
		FROM nodes
		WHERE type = ? AND (name = ? OR qualified_name = ?)
		LIMIT 1
	`))
	s.setStmt("listFilesByLang", mustPrepare(s.db, `SELECT id, path, hash, mtime, language, is_test FROM files WHERE language = ? ORDER BY path`))
	s.setStmt("listFilesAll", mustPrepare(s.db, `SELECT id, path, hash, mtime, language, is_test FROM files ORDER BY path`))
	s.setStmt("listFilesAllLimit", mustPrepare(s.db, `SELECT id, path, hash, mtime, language, is_test FROM files ORDER BY path LIMIT ?`))
	s.setStmt("searchFts", mustPrepare(s.db, `
		SELECT n.id, n.name, n.type, f.path, n.start_line, n.end_line
		FROM nodes_fts fts
		JOIN nodes n ON fts.rowid = n.id
		JOIN files f ON n.file_id = f.id
		WHERE nodes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`))
	s.setStmt("findDeadSymbols", mustPrepare(s.db, `
		SELECT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM nodes n
		JOIN files f ON n.file_id = f.id
		WHERE n.type NOT IN ('module', 'EXTERNAL')
		  AND NOT EXISTS (SELECT 1 FROM edges WHERE target_id = n.id)
		LIMIT ?
	`))
	s.setStmt("findOrphanFiles", mustPrepare(s.db, `
		SELECT f.path, f.language
		FROM files f
		WHERE NOT EXISTS (
			SELECT 1 FROM nodes n
			JOIN edges e ON e.target_id = n.id
			WHERE n.file_id = f.id
		)
	`))
	s.setStmt("findFileCycles", mustPrepare(s.db, `
		SELECT DISTINCT fs.path AS source_path, ft.path AS target_path
		FROM edges e1
		JOIN nodes n1 ON e1.source_id = n1.id AND n1.type = 'module'
		JOIN nodes n2 ON e1.target_id = n2.id AND n2.type = 'module'
		JOIN files fs ON n1.file_id = fs.id
		JOIN files ft ON n2.file_id = ft.id
	`))
	s.setStmt("getNeighborCallers", mustPrepare(s.db, `
		SELECT DISTINCT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content, e.relation
		FROM edges e
		JOIN nodes n ON e.source_id = n.id
		JOIN files f ON n.file_id = f.id
		WHERE e.target_id = ?
		ORDER BY f.path
		LIMIT ?
	`))
	s.setStmt("getNeighborCallees", mustPrepare(s.db, `
		SELECT DISTINCT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content, e.relation
		FROM edges e
		JOIN nodes n ON e.target_id = n.id
		JOIN files f ON n.file_id = f.id
		WHERE e.source_id = ?
		ORDER BY f.path
		LIMIT ?
	`))
	s.setStmt("findNodesByName", mustPrepare(s.db, `
		SELECT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM nodes n
		JOIN files f ON n.file_id = f.id
		WHERE n.name = ? OR n.qualified_name = ?
	`))
	s.setStmt("getNodesByName", mustPrepare(s.db, `
		SELECT id, file_id, type, name, language, path, qualified_name, signature, docstring, start_line, end_line, content
		FROM nodes
		WHERE name = ? OR qualified_name = ?
	`))
	s.setStmt("setMetadata", mustPrepare(s.db, `INSERT INTO metadata (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`))
	s.setStmt("getMetadata", mustPrepare(s.db, `SELECT value FROM metadata WHERE key = ?`))
	_ = err
}

func mustPrepare(db *sql.DB, query string) *sql.Stmt {
	stmt, err := db.Prepare(query)
	if err != nil {
		panic(fmt.Sprintf("failed to prepare statement: %v", err))
	}
	return stmt
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
			metadata TEXT,
			confidence TEXT NOT NULL DEFAULT 'EXTRACTED'
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

	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_id);
		CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
		CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
		CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
		CREATE INDEX IF NOT EXISTS idx_edges_confidence ON edges(confidence);
		CREATE INDEX IF NOT EXISTS idx_edges_source_relation ON edges(source_id, relation);
		CREATE INDEX IF NOT EXISTS idx_edges_target_relation ON edges(target_id, relation);
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
	err := s.db.QueryRow(`SELECT id, path, hash, mtime, language, is_test FROM files WHERE path = ?`, path).
		Scan(&fr.ID, &fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest)
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

// InsertNodes inserts multiple nodes in a single transaction and returns the last ID.
func (s *Storage) InsertNodes(nodes []NodeRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var lastID int64
	for _, node := range nodes {
		err := tx.QueryRow(`
			INSERT INTO nodes (file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id
		`, node.FileID, node.Type, node.Name, node.Language, node.QualifiedName, node.Signature, node.Docstring, node.StartLine, node.EndLine, node.Content).
			Scan(&lastID)
		if err != nil {
			return 0, err
		}
	}
	return lastID, tx.Commit()
}

// DeleteNodesForFile removes all nodes for a file.
func (s *Storage) DeleteNodesForFile(fileID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM nodes WHERE file_id = ?`, fileID)
	return err
}

// ClearNodesForFile removes all nodes for a file using prepared statement.
func (s *Storage) ClearNodesForFile(fileID int64) {
	stmt := s.getStmt("clearNodesForFile")
	if stmt == nil {
		s.db.Exec(`DELETE FROM nodes WHERE file_id = ?`, fileID)
		return
	}
	stmt.Exec(fileID)
}

// InsertEdge inserts an edge.
func (s *Storage) InsertEdge(sourceID, targetID int64, relation, metadata string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO edges (source_id, target_id, relation, metadata, confidence) VALUES (?, ?, ?, ?, ?)`,
		sourceID, targetID, relation, metadata, "EXTRACTED")
	return err
}

// InsertEdges inserts multiple edges in a single transaction.
func (s *Storage) InsertEdges(edges []EdgeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO edges (source_id, target_id, relation, metadata, confidence)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range edges {
		_, err := stmt.Exec(e.SourceID, e.TargetID, e.Relation, e.Metadata, e.Confidence)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
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
		SELECT n.id, n.name, n.qualified_name, n.type, f.path, n.start_line, n.end_line
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
		if err := rows.Scan(&r.ID, &r.Name, &r.QualifiedName, &r.Type, &r.Path, &r.StartLine, &r.EndLine); err != nil {
			continue
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// FindNodesByName finds nodes by name or qualified name.
func (s *Storage) FindNodesByName(name string) ([]NodeRecord, error) {
	rows, err := s.db.Query(`SELECT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content FROM nodes n JOIN files f ON n.file_id = f.id WHERE n.name = ? OR n.qualified_name = ?`, name, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.Path, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetNeighbors returns callers and callees for a node.
func (s *Storage) GetNeighbors(nodeID int64, limit int) (callers []NodeRecord, callees []NodeRecord, err error) {
	callers, err = s.queryNeighbors("getNeighborCallers", nodeID, limit)
	if err != nil {
		return nil, nil, err
	}

	callees, err = s.queryNeighbors("getNeighborCallees", nodeID, limit)
	if err != nil {
		return nil, nil, err
	}

	return callers, callees, nil
}

func (s *Storage) queryNeighbors(stmtKey string, nodeID int64, limit int) ([]NodeRecord, error) {
	stmt := s.getStmt(stmtKey)
	if stmt == nil {
		return nil, nil
	}
	rows, err := stmt.Query(nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNodes(rows)
}

// ListFiles returns indexed files, optionally filtered by language.
func (s *Storage) ListFiles(filter string, limit int) ([]FileRecord, error) {
	var rows *sql.Rows
	var err error
	if filter != "" {
		if limit > 0 {
			rows, err = s.db.Query(`SELECT id, path, hash, mtime, language, is_test FROM files WHERE language = ? ORDER BY path LIMIT ?`, filter, limit)
		} else {
			rows, err = s.db.Query(`SELECT id, path, hash, mtime, language, is_test FROM files WHERE language = ? ORDER BY path`, filter)
		}
	} else {
		if limit > 0 {
			rows, err = s.db.Query(`SELECT id, path, hash, mtime, language, is_test FROM files ORDER BY path LIMIT ?`, limit)
		} else {
			rows, err = s.db.Query(`SELECT id, path, hash, mtime, language, is_test FROM files ORDER BY path`)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.ID, &fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest); err != nil {
			continue
		}
		files = append(files, fr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
	Path          string
	QualifiedName string
	Signature     string
	Docstring     string
	StartLine     int
	EndLine       int
	Content       string
	Relation      string
}

// NodeSearchResult is a lightweight search result.
type NodeSearchResult struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	Type          string `json:"type"`
	Path          string `json:"path"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
}

func scanNodes(rows *sql.Rows) ([]NodeRecord, error) {
	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.Path, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content, &n.Relation); err != nil {
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
	rows, err := s.db.Query(`SELECT id, path, hash, mtime, language, is_test FROM files`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.ID, &fr.Path, &fr.Hash, &fr.MTime, &fr.Language, &fr.IsTest); err != nil {
			continue
		}
		files = append(files, fr)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return files
}

// GetRelatedForFile returns related file edges for a given file path.
func (s *Storage) GetRelatedForFile(filePath string, limit int) ([]RelatedRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fileID := int64(0)
	err := s.db.QueryRow(`SELECT id FROM files WHERE path = ?`, filePath).Scan(&fileID)
	if err != nil {
		return nil, nil
	}

	rows, err := s.db.Query(`SELECT id FROM nodes WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}
	var nodeIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			nodeIDs = append(nodeIDs, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(nodeIDs)), ",")
	query := fmt.Sprintf(`
		SELECT DISTINCT
			e.relation,
			f.path AS related_path,
			f.language AS related_language,
			n.name AS symbol_name,
			n.type AS symbol_type,
			CASE WHEN e.source_id IN (%[1]s) THEN 'outgoing' ELSE 'incoming' END AS direction
		FROM edges e
		JOIN nodes n ON (
			(e.source_id IN (%[1]s) AND e.target_id = n.id) OR
			(e.target_id IN (%[1]s) AND e.source_id = n.id)
		)
		JOIN files f ON n.file_id = f.id
		WHERE f.id != ?
		ORDER BY e.relation, direction, f.path
		LIMIT ?
	`, placeholders)

	params := make([]any, 0, len(nodeIDs)*3+2)
	for i := 0; i < 3; i++ {
		for _, id := range nodeIDs {
			params = append(params, id)
		}
	}
	params = append(params, fileID, limit)

	rows, err = s.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RelatedRecord
	for rows.Next() {
		var r RelatedRecord
		if err := rows.Scan(&r.Relation, &r.RelatedPath, &r.RelatedLanguage, &r.SymbolName, &r.SymbolType, &r.Direction); err == nil {
			results = append(results, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// FindCycles finds cycles in the graph.
func (s *Storage) FindCycles() ([]CycleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT f1.path, f2.path
		FROM edges e1
		JOIN nodes n1 ON e1.source_id = n1.id AND n1.type = 'module'
		JOIN nodes n2 ON e1.target_id = n2.id AND n2.type = 'module'
		JOIN files f1 ON n1.file_id = f1.id
		JOIN files f2 ON n2.file_id = f2.id
		GROUP BY f1.path, f2.path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cycleMap := make(map[string]bool)
	var cycles []CycleRecord
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err == nil {
			key := from + "|" + to
			if !cycleMap[key] {
				cycleMap[key] = true
				cycles = append(cycles, CycleRecord{
					Files: []string{from, to},
					Edges: []CycleEdge{{From: from, To: to}, {From: to, To: from}},
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cycles, nil
}

// FindShortestPath finds the shortest path between two symbols.
func (s *Storage) FindShortestPath(fromName, toName string, limit int) (*ShortestPathResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 6
	}

	fromNodes, err := s.getNodesByName(fromName)
	if err != nil {
		return &ShortestPathResult{Found: false}, nil
	}
	toNodes, err := s.getNodesByName(toName)
	if err != nil {
		return &ShortestPathResult{Found: false}, nil
	}

	if len(fromNodes) == 0 || len(toNodes) == 0 {
		return &ShortestPathResult{Found: false}, nil
	}

	fromID := fromNodes[0].ID
	toID := toNodes[0].ID

	type bfsItem struct {
		nodeID int64
		path   []ShortestPathStep
		depth  int
	}

	visited := make(map[int64]bool)
	queue := []bfsItem{{fromID, nil, 0}}
	visited[fromID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.nodeID == toID {
			return &ShortestPathResult{Found: true, Path: current.path}, nil
		}

		if current.depth >= limit {
			continue
		}

		rows, err := s.db.Query(`
			SELECT n.name, f.path, n.start_line, n.end_line, e.relation
			FROM edges e
			JOIN nodes n ON e.target_id = n.id
			JOIN files f ON n.file_id = f.id
			WHERE e.source_id = ?
		`, current.nodeID)
		if err != nil {
			continue
		}
		var outgoing []struct {
			name      string
			path      string
			startLine int
			endLine   int
			relation  string
		}
		for rows.Next() {
			var n struct {
				name      string
				path      string
				startLine int
				endLine   int
				relation  string
			}
			if err := rows.Scan(&n.name, &n.path, &n.startLine, &n.endLine, &n.relation); err == nil {
				outgoing = append(outgoing, n)
			}
		}
		if err := rows.Err(); err != nil {
			continue
		}
		rows.Close()

		for _, edge := range outgoing {
			nextID := int64(0)
			_ = s.db.QueryRow(`SELECT id FROM nodes WHERE name = ? AND file_id = (SELECT id FROM files WHERE path = ?)`, edge.name, edge.path).Scan(&nextID)
			if nextID == 0 || visited[nextID] {
				continue
			}
			visited[nextID] = true
			newPath := make([]ShortestPathStep, len(current.path)+1)
			copy(newPath, current.path)
			newPath[len(current.path)] = ShortestPathStep{
				SourceName: fromNodes[0].Name,
				SourceFile: fromNodes[0].Path,
				SourceLine: fromNodes[0].StartLine,
				TargetName: edge.name,
				TargetFile: edge.path,
				TargetLine: edge.startLine,
				Relation:   edge.relation,
			}
			queue = append(queue, bfsItem{nextID, newPath, current.depth + 1})
		}
	}

	return &ShortestPathResult{Found: false}, nil
}

// GetNodesForFile returns all nodes for a given file ID.
func (s *Storage) GetNodesForFile(fileID int64) ([]NodeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content FROM nodes WHERE file_id = ?`, fileID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Storage) getNodesByName(name string) ([]NodeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, file_id, type, name, language, path, qualified_name, signature, docstring, start_line, end_line, content FROM nodes WHERE name = ? OR qualified_name = ?`, name, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.Path, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

// FindNodeByName finds a single node by name, preferring qualified_name matches.
func (s *Storage) FindNodeByName(name string) *NodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n NodeRecord
	err := s.db.QueryRow(`
		SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content
		FROM nodes
		WHERE qualified_name = ? OR name = ?
		ORDER BY (qualified_name = ?) DESC
		LIMIT 1
	`, name, name, name).Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content)
	if err != nil {
		return nil
	}
	n.Path = s.getFilePathForNode(n.FileID)
	return &n
}

// FindNodeByTypeAndName finds a node by type and name.
func (s *Storage) FindNodeByTypeAndName(typ, name string) *NodeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n NodeRecord
	err := s.db.QueryRow(`
		SELECT id, file_id, type, name, language, qualified_name, signature, docstring, start_line, end_line, content
		FROM nodes
		WHERE type = ? AND (name = ? OR qualified_name = ?)
		LIMIT 1
	`, typ, name, name).Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content)
	if err != nil {
		return nil
	}
	n.Path = s.getFilePathForNode(n.FileID)
	return &n
}

func (s *Storage) getFilePathForNode(fileID int64) string {
	var path string
	_ = s.db.QueryRow(`SELECT path FROM files WHERE id = ?`, fileID).Scan(&path)
	return path
}

// GetAllEdges returns all edges with optional path filter and limit.
func (s *Storage) GetAllEdges(pathFilter string, limit int) []EdgeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT e.source_id, e.target_id, e.relation, e.metadata, e.confidence
		FROM edges e
	`
	args := []any{}
	if pathFilter != "" {
		query += ` WHERE e.source_id IN (SELECT id FROM nodes WHERE file_id IN (SELECT id FROM files WHERE path LIKE ?))`
		args = append(args, "%"+pathFilter+"%")
	}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []EdgeRecord
	for rows.Next() {
		var e EdgeRecord
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Relation, &e.Metadata, &e.Confidence); err == nil {
			results = append(results, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return results
}

// GetAllEdgeRecords returns raw edge records with IDs.
func (s *Storage) GetAllEdgeRecords(pathFilter string, limit int) []RawEdgeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT e.source_id, e.target_id, e.relation
		FROM edges e
		JOIN nodes s ON e.source_id = s.id
		JOIN nodes t ON e.target_id = t.id
		JOIN files fs ON s.file_id = fs.id
		JOIN files ft ON t.file_id = ft.id
	`
	var rows *sql.Rows
	var err error

	if pathFilter != "" {
		query += ` WHERE fs.path LIKE ? OR ft.path LIKE ?`
		if limit > 0 {
			query += ` LIMIT ?`
			rows, err = s.db.Query(query, "%"+pathFilter+"%", "%"+pathFilter+"%", limit)
		} else {
			rows, err = s.db.Query(query, "%"+pathFilter+"%", "%"+pathFilter+"%")
		}
	} else {
		if limit > 0 {
			query += ` LIMIT ?`
			rows, err = s.db.Query(query, limit)
		} else {
			rows, err = s.db.Query(query)
		}
	}

	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []RawEdgeRecord
	for rows.Next() {
		var e RawEdgeRecord
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Relation); err == nil {
			results = append(results, e)
		}
	}
	return results
}

// SearchSymbols searches symbols using FTS.
func (s *Storage) SearchSymbols(query string, limit int) []NodeSearchResult {
	sanitized := sanitizeFtsQuery(query)
	rows, err := s.db.Query(`
		SELECT n.id, n.name, n.qualified_name, n.type, f.path, n.start_line, n.end_line
		FROM nodes n
		JOIN nodes_fts fts ON n.id = fts.rowid
		JOIN files f ON n.file_id = f.id
		WHERE nodes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, sanitized, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []NodeSearchResult
	for rows.Next() {
		var r NodeSearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.QualifiedName, &r.Type, &r.Path, &r.StartLine, &r.EndLine); err != nil {
			continue
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return results
}

// GetStats returns graph statistics.
func (s *Storage) GetStats() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var fileCount, nodeCount, edgeCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&fileCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&nodeCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&edgeCount)

	return map[string]int{
		"fileCount": fileCount,
		"nodeCount": nodeCount,
		"edgeCount": edgeCount,
	}
}

// FindDeadCode returns dead code analysis results.
func (s *Storage) FindDeadCode(query string, limit int) *DeadcodeResult {
	if limit <= 0 {
		limit = 200
	}

	symbolQuery := `
		SELECT n.id, n.file_id, n.type, n.name, n.language, f.path, n.qualified_name, n.signature, n.docstring, n.start_line, n.end_line, n.content
		FROM nodes n
		JOIN files f ON n.file_id = f.id
		WHERE n.type NOT IN ('module', 'EXTERNAL')
		  AND NOT EXISTS (SELECT 1 FROM edges WHERE target_id = n.id)
	`
	args := []any{}

	if query != "" {
		symbolQuery += ` AND (f.path LIKE ? OR n.name LIKE ?)`
		args = append(args, "%"+query+"%", "%"+query+"%")
	}

	symbolQuery += `
		AND NOT EXISTS (
			SELECT 1 FROM nodes n2
			WHERE n2.id != n.id
			  AND n2.content LIKE '%' || n.name || '%'
		)
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := s.db.Query(symbolQuery, args...)
	if err != nil {
		return &DeadcodeResult{Symbols: nil, OrphanFiles: nil}
	}
	defer rows.Close()

	var symbols []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.ID, &n.FileID, &n.Type, &n.Name, &n.Language, &n.Path, &n.QualifiedName, &n.Signature, &n.Docstring, &n.StartLine, &n.EndLine, &n.Content); err == nil {
			symbols = append(symbols, n)
		}
	}

	// Find orphan files (files where no node has incoming edges)
	rows, err = s.db.Query(`
		SELECT f.path, f.language
		FROM files f
		WHERE NOT EXISTS (
			SELECT 1 FROM nodes n
			JOIN edges e ON e.target_id = n.id
			WHERE n.file_id = f.id
		)
	`)
	if err != nil {
		return &DeadcodeResult{Symbols: symbols, OrphanFiles: nil}
	}
	defer rows.Close()

	var orphanFiles []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(&fr.Path, &fr.Language); err == nil {
			orphanFiles = append(orphanFiles, fr)
		}
	}

	// Secondary content-scan fallback for orphan candidates whose stem appears
	// in another file's import string
	if len(orphanFiles) > 0 {
		importCheck, err := s.db.Prepare(`
			SELECT 1 FROM nodes n2
			JOIN files f2 ON n2.file_id = f2.id
			WHERE f2.path != ?
			  AND (n2.content LIKE ? OR n2.content LIKE ?)
			LIMIT 1
		`)
		if err == nil {
			defer importCheck.Close()
			referencedByImport := make(map[string]bool)
			for _, orphan := range orphanFiles {
				stem := strings.TrimSuffix(orphan.Path, filepath.Ext(orphan.Path))
				base := filepath.Base(stem)
				if len(base) < 4 {
					continue
				}
				var exists int
				_ = importCheck.QueryRow(orphan.Path, "%./"+base+"%", "%../"+base+"%").Scan(&exists)
				if exists > 0 {
					referencedByImport[orphan.Path] = true
				}
			}
			if len(referencedByImport) > 0 {
				filtered := make([]FileRecord, 0, len(orphanFiles))
				for _, of := range orphanFiles {
					if !referencedByImport[of.Path] {
						filtered = append(filtered, of)
					}
				}
				orphanFiles = filtered
			}
		}
	}

	return &DeadcodeResult{
		Symbols:     symbols,
		OrphanFiles: orphanFiles,
	}
}

// FileRecord matches the TS FileRecord shape.
type FileRecord struct {
	ID       int64
	Path     string
	Hash     string
	MTime    int64
	Language string
	IsTest   bool
}

// EdgeRecord represents an edge.
type EdgeRecord struct {
	SourceID   int64
	TargetID   int64
	Relation   string
	Metadata   string
	Confidence string
}

// RawEdgeRecord is a minimal edge record.
type RawEdgeRecord struct {
	SourceID int64
	TargetID int64
	Relation string
}
