package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Scanner walks a workspace and determines which files need indexing.
type Scanner struct {
	storage       *Storage
	rootDir       string
	parser        *Parser
	mu            sync.Mutex
}

// NewScanner creates a new scanner.
func NewScanner(storage *Storage, rootDir string) *Scanner {
	s := &Scanner{
		storage: storage,
		rootDir: rootDir,
		parser:  &Parser{},
	}
	return s
}

// SetParser sets the parser reference for extension detection.
func (s *Scanner) SetParser(p *Parser) {
	s.parser = p
}

// FileRecord matches the TS FileRecord shape.
type FileRecord struct {
	Path     string
	Hash     string
	MTime    int64
	Language string
	IsTest   bool
}

const maxFileSize = 500 * 1024
const maxFiles = 10000

// GetFilesToProcess returns files that need re-indexing.
func (s *Scanner) GetFilesToProcess() ([]FileRecord, error) {
	files, err := s.getDiskFiles()
	if err != nil {
		return nil, err
	}

	if len(files) > maxFiles {
		return nil, fmt.Errorf("too many files (%d), limit is %d", len(files), maxFiles)
	}

	var toProcess []FileRecord
	for _, fullPath := range files {
		relPath, err := filepath.Rel(s.rootDir, fullPath)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)

		if !isSupported(relPath) {
			continue
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if info.Size() > maxFileSize {
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])
		mtime := info.ModTime().UnixMilli()
		lang := detectLanguage(relPath)

		cached := s.storage.GetFileByPath(relPath)
		if cached == nil || cached.Hash != hashStr || cached.MTime != mtime {
			toProcess = append(toProcess, FileRecord{
				Path:     relPath,
				Hash:     hashStr,
				MTime:    mtime,
				Language: lang,
				IsTest:   isTest(relPath, lang),
			})
		}
	}

	return toProcess, nil
}

func (s *Scanner) getDiskFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".zenmcp" || name == "__pycache__" || name == "dist" || name == "build" {
				return fs.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func isSupported(relPath string) bool {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs",
		".go", ".py", ".rs", ".java",
		".c", ".cpp", ".h", ".hpp",
		".rb", ".lua":
		return true
	}
	return false
}

func detectLanguage(relPath string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		return "typescript"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	}
	return "unknown"
}

func isTest(relPath string, lang string) bool {
	lower := strings.ToLower(relPath)
	if strings.Contains(lower, "/tests/") || strings.Contains(lower, "/test/") || strings.Contains(lower, "/__tests__/") {
		return true
	}
	switch lang {
	case "go":
		return strings.HasSuffix(lower, "_test.go")
	case "python":
		return strings.HasSuffix(lower, "_test.py") || strings.HasPrefix(filepath.Base(lower), "test_")
	case "typescript", "javascript":
		return strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.")
	case "rust":
		return strings.HasSuffix(lower, "_test.rs")
	case "java":
		base := filepath.Base(lower)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		return strings.HasSuffix(stem, "Test") || strings.HasPrefix(stem, "Test")
	case "ruby":
		return strings.HasSuffix(lower, "_test.rb") || strings.HasSuffix(lower, "_spec.rb")
	default:
		return strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || strings.HasPrefix(filepath.Base(lower), "test_")
	}
}
