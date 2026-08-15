package projectmemory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
)

// RegisterProjectInMap ports registerProjectInMap: records a project in the
// global map.json (only when it has a .zenmcp dir), stamping lastVisited.
// The registered key is moved to the top of the file, mirroring the TS
// insertion-order semantics that json.Marshal on a Go map would lose.
func RegisterProjectInMap(projectPath string, dependencies []string) {
	defer func() {
		if r := recover(); r != nil {
			logfilter.Info("[ProjectMemory] Failed to register project in map: " + stringifyPanic(r))
		}
	}()
	zenDir := filepath.Join(projectPath, ".zenmcp")
	if _, err := os.Stat(zenDir); err != nil {
		return
	}

	mapFile := mcpcfg.MapFilePath()
	entries := readOrderedMap(mapFile)

	entryObj := map[string]any{}
	for _, e := range entries {
		if e.key == projectPath {
			var existing map[string]any
			if json.Unmarshal(e.value, &existing) == nil {
				entryObj = existing
			}
			break
		}
	}
	entryObj["lastVisited"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if dependencies != nil {
		entryObj["dependencies"] = dependencies
	}
	entryRaw, err := json.MarshalIndent(entryObj, "", "  ")
	if err != nil {
		return
	}

	newEntries := make([]mapEntry, 0, len(entries)+1)
	newEntries = append(newEntries, mapEntry{key: projectPath, value: entryRaw})
	for _, e := range entries {
		if e.key != projectPath {
			newEntries = append(newEntries, e)
		}
	}

	data := marshalOrderedMap(newEntries)
	if err := os.WriteFile(mapFile, data, 0o644); err != nil {
		logfilter.Info("[ProjectMemory] Failed to register project in map: " + err.Error())
	}
}

type mapEntry struct {
	key   string
	value json.RawMessage
}

// readOrderedMap parses a JSON object preserving key order.
func readOrderedMap(mapFile string) []mapEntry {
	raw, err := os.ReadFile(mapFile)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	var entries []mapEntry
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return entries
		}
		key, ok := keyTok.(string)
		if !ok {
			return entries
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return entries
		}
		entries = append(entries, mapEntry{key: key, value: value})
	}
	return entries
}

// marshalOrderedMap writes an indented JSON object keeping entry order,
// re-indenting each value's lines to nest under its key.
func marshalOrderedMap(entries []mapEntry) []byte {
	var b strings.Builder
	b.WriteString("{\n")
	for i, e := range entries {
		keyJSON, _ := json.Marshal(e.key)
		b.WriteString("  ")
		b.Write(keyJSON)
		b.WriteString(": ")
		val := normalizeJSONValue(e.value)
		lines := strings.Split(strings.TrimRight(val, "\n"), "\n")
		for j, ln := range lines {
			if j > 0 {
				b.WriteString("  ")
			}
			b.WriteString(ln)
			if j < len(lines)-1 {
				b.WriteByte('\n')
			}
		}
		if i < len(entries)-1 {
			b.WriteString(",\n")
		} else {
			b.WriteByte('\n')
		}
	}
	b.WriteString("}")
	return []byte(b.String())
}

// normalizeJSONValue re-indents a raw value from any prior layout, preventing
// indentation from compounding across rewrite cycles.
func normalizeJSONValue(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		buf.Reset()
		if err := json.Compact(&buf, raw); err != nil {
			return string(raw)
		}
	}
	return buf.String()
}

// stringifyPanic is a helper function
func stringifyPanic(r any) string {
	if err, ok := r.(error); ok {
		return err.Error()
	}
	return "unknown error"
}
