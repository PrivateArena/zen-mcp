package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Scanner walks a workspace and determines which files need indexing.
type Scanner struct {
	storage      *Storage
	rootDir      string
	parser       *Parser
	mu           sync.Mutex
	ignore       *gitignore.GitIgnore
	aliasMap     map[string]string
	aliasBaseUrl string
}

// NewScanner creates a new scanner.
func NewScanner(storage *Storage, rootDir string) *Scanner {
	s := &Scanner{
		storage:      storage,
		rootDir:      rootDir,
		parser:       GetParser(),
		aliasMap:     make(map[string]string),
		aliasBaseUrl: ".",
	}
	s.loadIgnorePatterns()
	s.LoadTsConfigAliases()
	return s
}

// SetParser sets the parser reference for extension detection.
func (s *Scanner) SetParser(p *Parser) {
	s.parser = p
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

	// Load every stored file record once and diff in memory instead of running
	// one GetFileByPath query per file on disk.
	cachedByPath := make(map[string]FileRecord)
	if s.storage != nil {
		for _, fr := range s.storage.GetAllFiles() {
			cachedByPath[fr.Path] = fr
		}
	}

	var toProcess []FileRecord
	for _, relPath := range files {
		if s.IsIgnored(relPath, false) {
			continue
		}
		fullPath := filepath.Join(s.rootDir, relPath)

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

		mtime := info.ModTime().UnixMilli()
		lang := detectLanguage(relPath)

		cached, ok := cachedByPath[relPath]
		if ok && cached.MTime == mtime && cached.Hash != "" {
			// Fast path: unchanged mtime, skip the read + hash entirely.
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		if ok && cached.Hash == hashStr {
			// Content unchanged (touch / clock skew): refresh mtime only.
			if s.storage != nil {
				_ = s.storage.RefreshFileMTime(relPath, mtime)
			}
			continue
		}

		toProcess = append(toProcess, FileRecord{
			Path:     relPath,
			Hash:     hashStr,
			MTime:    mtime,
			Language: lang,
			IsTest:   isTest(relPath, lang),
			content:  content,
		})
	}

	return toProcess, nil
}

func (s *Scanner) getDiskFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relPath, relErr := filepath.Rel(s.rootDir, path)
		if relErr != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		if d.IsDir() {
			if relPath == "." {
				return nil
			}
			if s.IsIgnored(relPath, true) {
				return fs.SkipDir
			}
			return nil
		}
		if s.IsIgnored(relPath, false) {
			return nil
		}
		files = append(files, relPath)
		return nil
	})
	return files, err
}

// GetDiskFiles returns all disk files under the scanner root.
func (s *Scanner) GetDiskFiles() ([]string, error) {
	return s.getDiskFiles()
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

func (s *Scanner) loadIgnorePatterns() {
	patterns := []string{".git", "node_modules", ".venv", "venv", "dist", ".zen", ".zenmcp", "__pycache__", ".next", ".nuxt", ".output"}

	// Project-local .gitignore
	if data, err := os.ReadFile(filepath.Join(s.rootDir, ".gitignore")); err == nil {
		patterns = append(patterns, parseIgnoreLines(data)...)
	}

	// Project-local .codegraphignore (highest priority, added last)
	if data, err := os.ReadFile(filepath.Join(s.rootDir, ".codegraphignore")); err == nil {
		patterns = append(patterns, parseIgnoreLines(data)...)
	}

	s.ignore = gitignore.CompileIgnoreLines(patterns...)
}

// parseIgnoreLines trims and drops blank/comment lines, matching the TS
// implementation which maps each line through trim + Boolean filter before
// handing them to the ignore matcher.
func parseIgnoreLines(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

// IsIgnored checks if a path should be ignored.
func (s *Scanner) IsIgnored(relPath string, isDirectory bool) bool {
	if relPath == "" || s.ignore == nil {
		return false
	}
	normalizedPath := strings.ReplaceAll(relPath, "\\", "/")
	if isDirectory && !strings.HasSuffix(normalizedPath, "/") {
		normalizedPath += "/"
	}
	return s.ignore.MatchesPath(normalizedPath)
}

// IsSupported checks if a file extension is supported.
func (s *Scanner) IsSupported(file string) bool {
	lower := strings.ToLower(file)
	excluded := []string{".min.js", ".min.ts", ".min.mjs", ".bundle.js", ".bundle.ts", ".bundle.mjs"}
	for _, sfx := range excluded {
		if strings.HasSuffix(lower, sfx) {
			return false
		}
	}
	ext := filepath.Ext(file)
	if ext == "" {
		return false
	}
	if s.parser != nil {
		supported := s.parser.GetSupportedExtensions()
		for _, e := range supported {
			if strings.EqualFold(e, ext) {
				return true
			}
		}
	}
	return isSupported(file)
}

// GetFileDetails returns content, hash, mtime, language, and is_test for a file.
func (s *Scanner) GetFileDetails(relPath string) (content string, hash string, mtime int64, language string, isTest bool, err error) {
	fullPath := filepath.Join(s.rootDir, relPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", "", 0, "", false, err
	}
	if info.Size() > maxFileSize {
		return "", "", 0, "", false, fmt.Errorf("file exceeds 500KB limit")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", "", 0, "", false, err
	}
	content = string(data)
	hashBytes := sha256.Sum256(data)
	hash = hex.EncodeToString(hashBytes[:])
	mtime = info.ModTime().UnixMilli()
	language = s.detectLanguage(relPath)
	isTest = s.isTest(relPath, language)
	return content, hash, mtime, language, isTest, nil
}

func (s *Scanner) detectLanguage(relPath string) string {
	if s.parser != nil {
		ext := "." + strings.ToLower(filepath.Ext(relPath))
		lang := s.parser.GetExtensionLanguage(ext)
		if lang != "" {
			return lang
		}
	}
	return detectLanguage(relPath)
}

func (s *Scanner) isTest(relPath string, language string) bool {
	return isTest(relPath, language)
}

// LoadTsConfigAliases loads tsconfig.json path aliases.
func (s *Scanner) LoadTsConfigAliases() {
	s.aliasMap = make(map[string]string)
	s.aliasBaseUrl = "."
	tsconfigPath := filepath.Join(s.rootDir, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); err != nil {
		return
	}
	data, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	compilerOptions, ok := raw["compilerOptions"].(map[string]interface{})
	if !ok {
		return
	}
	paths, ok := compilerOptions["paths"].(map[string]interface{})
	if !ok {
		return
	}
	baseUrl, _ := compilerOptions["baseUrl"].(string)
	s.aliasBaseUrl = baseUrl
	if s.aliasBaseUrl == "" {
		s.aliasBaseUrl = "."
	}
	for alias, targetVal := range paths {
		targets, ok := targetVal.([]interface{})
		if !ok || len(targets) == 0 {
			continue
		}
		target, _ := targets[0].(string)
		prefix := strings.TrimSuffix(alias, "*")
		resolved := strings.TrimSuffix(target, "*")
		s.aliasMap[prefix] = resolved
	}
}

// ResolveAlias resolves a TS import specifier against tsconfig path aliases.
func (s *Scanner) ResolveAlias(specifier string) string {
	for prefix, target := range s.aliasMap {
		if strings.HasPrefix(specifier, prefix) {
			rest := strings.TrimPrefix(specifier, prefix)
			return filepath.Join(s.aliasBaseUrl, target+rest)
		}
	}
	return ""
}
