package analysis

import (
	"database/sql"
	"encoding/json"
	"strings"

	"zen-mcp/internal/projectmemory"
)

// OutputAnalysis mirrors the TS OutputAnalysis shape.
type OutputAnalysis struct {
	FileType      FileTypeResult `json:"file_type"`
	ReadingAdvice ReadingAdvice  `json:"reading_advice"`
	LineCount     int            `json:"line_count"`
	ByteSize      int            `json:"byte_size"`
	Sample        string         `json:"sample"`
}

// AnalyzeOutput ports analyzeOutput from analysis/index.ts.
func AnalyzeOutput(text string) OutputAnalysis {
	ft := DetectFileType(text)
	advice := SuggestReadingTool(ft)
	lines := strings.Split(text, "\n")
	sampleLines := lines
	if len(sampleLines) > 5 {
		sampleLines = sampleLines[:5]
	}
	return OutputAnalysis{
		FileType:      ft,
		ReadingAdvice: advice,
		LineCount:     len(lines),
		ByteSize:      len([]byte(text)),
		Sample:        strings.Join(sampleLines, "\n"),
	}
}

// StoreOutputAnalysis ports storeOutputAnalysis.
func StoreOutputAnalysis(dbPath, virtID string, analysis OutputAnalysis) error {
	db := projectmemory.GetDatabase(dbPath)
	if db == nil {
		return sql.ErrConnDone
	}
	readingJSON, err := json.Marshal(analysis.ReadingAdvice)
	if err != nil {
		return err
	}
	metaJSON, err := json.Marshal(analysis)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT OR REPLACE INTO virtual_analysis (id, file_type, subtype, confidence, tool_hint, reading_advice, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		virtID,
		analysis.FileType.Type,
		analysis.FileType.Subtype,
		analysis.FileType.Confidence,
		analysis.ReadingAdvice.Tool,
		string(readingJSON),
		string(metaJSON),
	)
	return err
}

// GetStoredAnalysis ports getStoredAnalysis.
func GetStoredAnalysis(dbPath, virtID string) *OutputAnalysis {
	db := projectmemory.GetDatabase(dbPath)
	if db == nil {
		return nil
	}
	var metadata string
	if err := db.QueryRow(`SELECT metadata FROM virtual_analysis WHERE id = ?`, virtID).Scan(&metadata); err != nil {
		return nil
	}
	var analysis OutputAnalysis
	if err := json.Unmarshal([]byte(metadata), &analysis); err != nil {
		return nil
	}
	return &analysis
}
