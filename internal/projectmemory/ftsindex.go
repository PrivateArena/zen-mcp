package projectmemory

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"zen-mcp/internal/logfilter"
)

// randHex8 is a helper function
func randHex8() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08d", time.Now().UnixNano()%100000000)
	}
	return hex.EncodeToString(b)
}

// MemoryIndexItem mirrors the TS MemoryIndexItem shape. Embeddings are
// intentionally omitted: the CodeGraphEmbedder (F6) is the only source of
// them and TS falls back to lexical-only search when it is unavailable, so
// porting the embedding column as NULL reproduces that fallback exactly.
type MemoryIndexItem struct {
	ID       int
	Type     string
	Title    string
	Content  string
	Archived int
	Metadata *string
}

var (
	dbMu    sync.Mutex
	dbCache = map[string]*sql.DB{}
)

// GetDatabase is a helper function
func GetDatabase(dbPath string) *sql.DB {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db, ok := dbCache[dbPath]; ok {
		return db
	}
	if dir := filepath.Dir(dbPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to open database %s: %v", dbPath, err)
		return nil
	}
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode = WAL;")
	db.Exec("PRAGMA busy_timeout = 3000;")
	initMemorySchema(db)
	dbCache[dbPath] = db
	return db
}

// ClearAllDatabaseCache is a helper function
func ClearAllDatabaseCache() {
	dbMu.Lock()
	defer dbMu.Unlock()
	for _, db := range dbCache {
		_ = db.Close()
	}
	dbCache = map[string]*sql.DB{}
}

// ClearDatabase is a helper function
func ClearDatabase(dbPath string) error {
	dbMu.Lock()
	if db, ok := dbCache[dbPath]; ok {
		_ = db.Close()
		delete(dbCache, dbPath)
	}
	dbMu.Unlock()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// initMemorySchema is a helper function
func initMemorySchema(db *sql.DB) {
	const schema = `
CREATE TABLE IF NOT EXISTS memory (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  archived INTEGER DEFAULT 0,
  metadata TEXT,
  embedding BLOB
);

CREATE INDEX IF NOT EXISTS idx_memory_archived ON memory(archived);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
  type, title, content,
  content='memory', content_rowid='id'
);

DROP TRIGGER IF EXISTS pm_ai;
CREATE TRIGGER pm_ai AFTER INSERT ON memory BEGIN
  INSERT INTO memory_fts(rowid, type, title, content)
  VALUES (new.id, new.type, new.title, new.content);
END;

DROP TRIGGER IF EXISTS pm_ad;
CREATE TRIGGER pm_ad AFTER DELETE ON memory BEGIN
  INSERT INTO memory_fts(memory_fts, rowid, type, title, content)
  VALUES ('delete', old.id, old.type, old.title, old.content);
END;

DROP TRIGGER IF EXISTS pm_au;
CREATE TRIGGER pm_au AFTER UPDATE ON memory BEGIN
  INSERT INTO memory_fts(memory_fts, rowid, type, title, content)
  VALUES ('delete', old.id, old.type, old.title, old.content);
  INSERT INTO memory_fts(rowid, type, title, content)
  VALUES (new.id, new.type, new.title, new.content);
END;

CREATE TABLE IF NOT EXISTS virtual_store (
  id TEXT PRIMARY KEY,
  session_token TEXT,
  source_tool TEXT,
  intent TEXT,
  raw_payload TEXT,
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS virtual_analysis (
  id TEXT PRIMARY KEY,
  file_type TEXT NOT NULL,
  subtype TEXT DEFAULT '',
  confidence REAL DEFAULT 0,
  tool_hint TEXT DEFAULT '',
  reading_advice TEXT DEFAULT '{}',
  metadata TEXT DEFAULT '{}',
  FOREIGN KEY (id) REFERENCES virtual_store(id)
);
`
	if _, err := db.Exec(schema); err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to init memory schema: %v", err)
	}

	_, ftsErr := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS virtual_fts USING fts5(
  id UNINDEXED,
  source_tool,
  intent,
  content,
  tokenize='porter'
);`)
	if ftsErr != nil {
		if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS virtual_fts USING fts5(
  id UNINDEXED,
  source_tool,
  intent,
  content
);`); err != nil {
			logfilter.Debugf("[ProjectMemory] Failed to create virtual_fts: %v", err)
		}
	}
}

// IndexActiveMemory ports indexActiveMemory: diffs against currently active
// rows, deleting rows that dropped out and inserting only new/changed items.
func IndexActiveMemory(dbPath string, items []MemoryIndexItem) {
	db := GetDatabase(dbPath)
	if db == nil {
		return
	}

	rows, err := db.Query(`SELECT id, type, title, content FROM memory WHERE archived = 0`)
	if err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to query active rows: %v", err)
		return
	}
	type activeRow struct {
		id  int64
		key string
	}
	var existing []activeRow
	for rows.Next() {
		var id int64
		var typ, title, content string
		if err := rows.Scan(&id, &typ, &title, &content); err != nil {
			continue
		}
		existing = append(existing, activeRow{id: id, key: keyOf(typ, title, content)})
	}
	rows.Close()

	existingByKey := map[string]int64{}
	for _, r := range existing {
		existingByKey[r.key] = r.id
	}
	incomingKeys := map[string]bool{}
	for _, i := range items {
		incomingKeys[keyOf(i.Type, i.Title, i.Content)] = true
	}

	var toInsert []MemoryIndexItem
	for _, i := range items {
		if _, ok := existingByKey[keyOf(i.Type, i.Title, i.Content)]; !ok {
			toInsert = append(toInsert, i)
		}
	}
	var toDelete []int64
	for _, r := range existing {
		if !incomingKeys[r.key] {
			toDelete = append(toDelete, r.id)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to begin tx: %v", err)
		return
	}
	defer tx.Rollback()

	for _, id := range toDelete {
		if _, err := tx.Exec(`DELETE FROM memory WHERE id = ?`, id); err != nil {
			logfilter.Debugf("[ProjectMemory] Failed to delete row %d: %v", id, err)
			return
		}
	}
	for _, item := range toInsert {
		if _, err := tx.Exec(
			`INSERT INTO memory (type, title, content, archived, metadata, embedding) VALUES (?, ?, ?, 0, ?, NULL)`,
			item.Type, item.Title, item.Content, item.Metadata,
		); err != nil {
			logfilter.Debugf("[ProjectMemory] Failed to insert row: %v", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to commit: %v", err)
	}
}

// keyOf is a helper function
func keyOf(typ, title, content string) string {
	return typ + "\x00" + title + "\x00" + content
}

// SearchIndexedMemory ports searchIndexedMemory. Embeddings are not ported,
// so the semantic leg is skipped and results are lexical-only, matching the
// TS fallback path when the embedder is unavailable.
func SearchIndexedMemory(dbPath, query string, limit int) []SearchResult {
	db := GetDatabase(dbPath)
	if db == nil {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}

	type doc struct {
		id         int64
		typ, title string
		content    string
		archived   int
		lexRank    int
	}
	var docs []doc

	rows, err := db.Query(`SELECT pm.id, pm.type, pm.title, pm.content, pm.archived FROM memory pm JOIN memory_fts fts ON pm.id = fts.rowid WHERE memory_fts MATCH ? LIMIT ?`, query, limit)
	if err == nil {
		rank := 1
		for rows.Next() {
			var d doc
			if err := rows.Scan(&d.id, &d.typ, &d.title, &d.content, &d.archived); err != nil {
				continue
			}
			d.lexRank = rank
			rank++
			docs = append(docs, d)
		}
		rows.Close()
	} else {
		logfilter.Debugf("[ProjectMemory] Lexical FTS5 query failed: %v", err)
	}

	const k = 60
	results := make([]SearchResult, 0, len(docs))
	for _, d := range docs {
		score := 0.0
		if d.lexRank > 0 {
			score += 1 / float64(k+d.lexRank)
		}
		results = append(results, SearchResult{
			Type:     d.typ,
			Title:    d.title,
			Content:  d.content,
			Archived: d.archived,
			Score:    score,
		})
	}

	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j-1].Score < results[j].Score; j-- {
			results[j-1], results[j] = results[j], results[j-1]
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

type SearchResult struct {
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Archived int     `json:"archived"`
	Score    float64 `json:"score"`
}

// IndexVirtualContext ports indexVirtualContext, returning the virt_ id.
func IndexVirtualContext(dbPath string, data VirtualContextData) string {
	db := GetDatabase(dbPath)
	if db == nil {
		return ""
	}
	id := fmt.Sprintf("virt_%d_%s", time.Now().UnixMilli(), randHex8())

	tx, err := db.Begin()
	if err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to begin virtual tx: %v", err)
		return id
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO virtual_store (id, session_token, source_tool, intent, raw_payload) VALUES (?, ?, ?, ?, ?)`,
		id, data.SessionToken, data.SourceTool, data.Intent, data.Payload,
	); err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to insert virtual_store: %v", err)
		return id
	}
	if _, err := tx.Exec(
		`INSERT INTO virtual_fts (id, source_tool, intent, content) VALUES (?, ?, ?, ?)`,
		id, data.SourceTool, data.Intent, data.Payload,
	); err != nil {
		// FTS fallback: ignore
	}
	if err := tx.Commit(); err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to commit virtual tx: %v", err)
	}
	return id
}

type VirtualContextData struct {
	SessionToken string
	SourceTool   string
	Intent       string
	Payload      string
}

// RetrieveVirtualContext ports retrieveVirtualContext (id + optional filter).
func RetrieveVirtualContext(dbPath, id, filterQuery string) string {
	db := GetDatabase(dbPath)
	if db == nil {
		return fmt.Sprintf("[Error] Virtual context with ID %s not found.", id)
	}

	var one int
	ftsErr := db.QueryRow(`
		SELECT 1 FROM virtual_store vs
		JOIN virtual_fts fts ON vs.id = fts.id
		WHERE fts.id = ?`, id).Scan(&one)
	if ftsErr == nil && one == 0 {
		return fmt.Sprintf("No content found in virtual context %s.", id)
	}
	if ftsErr == sql.ErrNoRows {
		return fmt.Sprintf("No content found in virtual context %s.", id)
	}

	var payload string
	if err := db.QueryRow(`SELECT raw_payload FROM virtual_store WHERE id = ?`, id).Scan(&payload); err != nil {
		return fmt.Sprintf("[Error] Virtual context with ID %s not found.", id)
	}

	if filterQuery == "" {
		return payload
	}

	cleanQuery := strings.NewReplacer("*", "", `"`, "", "'", "").Replace(filterQuery)
	cleanQuery = strings.ToLower(strings.TrimSpace(cleanQuery))
	terms := []string{}
	for _, t := range strings.Fields(cleanQuery) {
		if t != "" {
			terms = append(terms, t)
		}
	}

	var matched []string
	for _, line := range strings.Split(payload, "\n") {
		lower := strings.ToLower(line)
		if len(terms) > 0 {
			for _, term := range terms {
				if strings.Contains(lower, term) {
					matched = append(matched, line)
					break
				}
			}
		} else if strings.Contains(lower, cleanQuery) {
			matched = append(matched, line)
		}
	}

	if len(matched) == 0 {
		return fmt.Sprintf("No lines matched filter %q in virtual context %s.", filterQuery, id)
	}
	return strings.Join(matched, "\n")
}

// LogProjectEvent ports logProjectEvent: inserts an archived (1) memory row
// with a metadata JSON containing timestamp and any extra fields.
func LogProjectEvent(dbPath, typ, title, content string, metadata map[string]any) {
	db := GetDatabase(dbPath)
	if db == nil {
		return
	}
	merged := map[string]any{"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
	for k, v := range metadata {
		merged[k] = v
	}
	metaJSON, err := json.Marshal(merged)
	if err != nil {
		metaJSON = []byte("{}")
	}
	if _, err := db.Exec(
		`INSERT INTO memory (type, title, content, archived, metadata) VALUES (?, ?, ?, 1, ?)`,
		typ, title, content, string(metaJSON),
	); err != nil {
		logfilter.Debugf("[ProjectMemory] Failed to log project event: %v", err)
	}
}

// RecentCommand mirrors one recent shell/git command row in load output.
type RecentCommand struct {
	Command   string `json:"command"`
	Timestamp string `json:"timestamp"`
}

// RecentCommands ports the recent_commands query from loadProjectMemoryState:
// newest 200 archived shell/git rows, deduped by title, capped at 50.
func RecentCommands(dbPath string) []RecentCommand {
	db := GetDatabase(dbPath)
	if db == nil {
		return nil
	}
	rows, err := db.Query(
		`SELECT title, content, metadata FROM memory
		 WHERE archived = 1 AND type IN ('shell', 'git') AND metadata IS NOT NULL
		 ORDER BY id DESC LIMIT 200`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	seen := map[string]bool{}
	var commands []RecentCommand
	for rows.Next() {
		var title string
		var content *string
		var metadata string
		if err := rows.Scan(&title, &content, &metadata); err != nil {
			continue
		}
		meta := map[string]any{}
		if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
			continue
		}
		if seen[title] {
			continue
		}
		seen[title] = true
		commands = append(commands, RecentCommand{
			Command:   title,
			Timestamp: asString(meta["timestamp"]),
		})
		if len(commands) >= 50 {
			break
		}
	}
	return commands
}
