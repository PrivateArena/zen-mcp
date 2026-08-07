package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"zen-mcp/internal/mcpcfg"
)

type pathCandidate struct {
	path  string
	score int
}

type candidateEntry struct {
	score  int
	exists bool
}

type PathResolver struct {
	aliasMap   map[string]string
	candidates map[string]candidateEntry
	cwd        string
}

func NewPathResolver(aliasMap map[string]string, cwd string) *PathResolver {
	return &PathResolver{aliasMap: aliasMap, candidates: map[string]candidateEntry{}, cwd: cwd}
}

func LoadAliasMap() map[string]string {
	data, err := os.ReadFile(mcpcfg.MapFilePath())
	if err != nil {
		return map[string]string{}
	}
	aliasMap := map[string]string{}
	for _, fullPath := range orderedMapKeys(data) {
		aliasMap[fullPath] = fullPath
		base := filepath.Base(fullPath)
		if base != "" {
			aliasMap[base] = fullPath
		}
	}
	return aliasMap
}

func orderedMapKeys(data []byte) []string {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			break
		}
		if k, ok := key.(string); ok {
			keys = append(keys, k)
		}
		var v any
		if err := dec.Decode(&v); err != nil {
			break
		}
	}
	return keys
}

func (p *PathResolver) Resolve(input string) (string, bool) {
	if exact, ok := p.aliasMap[input]; ok && exists(exact) {
		return exact, true
	}

	isAbs := filepath.IsAbs(input)
	resolvedFromCwd := input
	if !isAbs {
		resolvedFromCwd = filepath.Join(p.cwd, input)
	}

	if isAbs && exists(resolvedFromCwd) {
		return resolvedFromCwd, true
	}

	base := filepath.Base(input)
	if basMatch, ok := p.aliasMap[base]; ok && exists(basMatch) {
		return basMatch, true
	}

	lowered := strings.ToLower(input)
	tokens := p.tokenize(lowered)
	var scored []pathCandidate
	for fullPath := range p.aliasMap {
		b := strings.ToLower(filepath.Base(fullPath))
		switch {
		case b == lowered:
			scored = append(scored, pathCandidate{path: fullPath, score: 100})
		case strings.HasPrefix(b, lowered):
			scored = append(scored, pathCandidate{path: fullPath, score: 80})
		case strings.Contains(b, lowered):
			scored = append(scored, pathCandidate{path: fullPath, score: 60})
		case len(tokens) > 0:
			bTokens := p.tokenize(b)
			matchCount := 0
			for _, t := range tokens {
				for _, bt := range bTokens {
					if t == bt {
						matchCount++
						break
					}
				}
			}
			if matchCount > 0 {
				scored = append(scored, pathCandidate{path: fullPath, score: 20 + matchCount*10})
			}
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	for _, c := range scored {
		if c.score > 0 && exists(c.path) {
			return c.path, true
		}
	}

	if exists(resolvedFromCwd) {
		return resolvedFromCwd, true
	}
	return "", false
}

func (p *PathResolver) tokenize(input string) []string {
	parts := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		switch r {
		case '-', '_', '/', '\\':
			return true
		}
		return false
	})
	return parts
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
