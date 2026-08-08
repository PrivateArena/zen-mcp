package codegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetSkeletonAfterIndex(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

func Add(a int, b int) int {
	return a + b
}

func mul(x, y int) int {
	return x * y
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	got, err := cg.GetSkeleton("calc.go")
	if err != nil {
		t.Fatalf("GetSkeleton error: %v", err)
	}

	if strings.Contains(got, "has no indexed symbols") {
		t.Fatalf("expected indexed symbols, got:\n%s", got)
	}
	if !strings.Contains(got, "Add") {
		t.Fatalf("expected skeleton to contain Add, got:\n%s", got)
	}
	if !strings.Contains(got, "mul") {
		t.Fatalf("expected skeleton to contain mul, got:\n%s", got)
	}
	if !strings.Contains(got, "lines") {
		t.Fatalf("expected skeleton to contain line numbers, got:\n%s", got)
	}
}

func TestGetSkeletonMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	got, err := cg.GetSkeleton("nonexistent.go")
	if err != nil {
		t.Fatalf("GetSkeleton error: %v", err)
	}

	if !strings.Contains(got, "File not found in index") {
		t.Fatalf("expected not-found message, got: %s", got)
	}
}

func TestGetFileByPathReturnsID(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package bar
func Hello() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "hello.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	fr := cg.storage.GetFileByPath("hello.go")
	if fr == nil {
		t.Fatalf("GetFileByPath returned nil for indexed file")
	}
	if fr.ID == 0 {
		t.Fatalf("GetFileByPath returned ID=0; expected non-zero ID")
	}
	if fr.Path != "hello.go" {
		t.Fatalf("GetFileByPath returned path=%q; want %q", fr.Path, "hello.go")
	}
	if fr.Language != "go" {
		t.Fatalf("GetFileByPath returned language=%q; want %q", fr.Language, "go")
	}
}

func TestDeadcodeDoesNotFlagUsedStruct(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

type AgentChatParams struct {
	Provider string
	Message  string
}

func DelegateToWebAgent(params AgentChatParams) string {
	return params.Provider + ":" + params.Message
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "agent.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "AgentChatParams" {
			t.Fatalf("struct used as parameter should not be deadcode, got: %s %s at %s:%d", s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeFlagsUnusedFunction(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

func Used() {}
func Unused() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "calc.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	foundUnused := false
	foundUsed := false
	for _, s := range symbols {
		if s.Name == "Unused" {
			foundUnused = true
		}
		if s.Name == "Used" {
			foundUsed = true
		}
	}
	if !foundUnused {
		t.Fatalf("expected Unused to be deadcode")
	}
	if foundUsed {
		t.Fatalf("Used should not be deadcode")
	}
}

func TestDeadcodeDoesNotFlagStructUsedAsField(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

type Inner struct {
	Value int
}

type Outer struct {
	Inner Inner
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "Inner" || s.Name == "Outer" {
			t.Fatalf("%s used as field type should not be deadcode, got: %s %s at %s:%d", s.Name, s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeDoesNotFlagInterfaceUsedInSignature(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

type Storage interface {
	Get(key string) string
}

func GetData(s Storage) string {
	return s.Get("key")
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "storage.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "Storage" {
			t.Fatalf("interface used as parameter should not be deadcode, got: %s %s at %s:%d", s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeDoesNotFlagTypeUsedAcrossFiles(t *testing.T) {
	tmpDir := t.TempDir()

	typeSrc := `package foo

type ChatParams struct {
	Provider string
}
`
	usageSrc := `package foo

func Send(params ChatParams) {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "types.go"), []byte(typeSrc), 0644); err != nil {
		t.Fatalf("write type fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "send.go"), []byte(usageSrc), 0644); err != nil {
		t.Fatalf("write usage fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "ChatParams" {
			t.Fatalf("type used across files should not be deadcode, got: %s %s at %s:%d", s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeDoesNotFlagCrossFileCalls(t *testing.T) {
	tmpDir := t.TempDir()

	storageSrc := `package foo

type Storage struct{}

func (s *Storage) InsertNodes(nodes []NodeRecord) (int64, error) {
	return 0, nil
}

func (s *Storage) InsertEdges(edges []EdgeRecord) error {
	return nil
}
`
	engineSrc := `package foo

func Index(storage *Storage) {
	var nodes []NodeRecord
	storage.InsertNodes(nodes)
	storage.InsertEdges(nil)
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "storage.go"), []byte(storageSrc), 0644); err != nil {
		t.Fatalf("write storage fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "engine.go"), []byte(engineSrc), 0644); err != nil {
		t.Fatalf("write engine fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "InsertNodes" || s.Name == "InsertEdges" {
			t.Fatalf("cross-file called method should not be deadcode, got: %s %s at %s:%d", s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeDoesNotFlagArgumentReferences(t *testing.T) {
	tmpDir := t.TempDir()

	src := `package foo

func brainExtract() error {
	return nil
}

func init() {
	Register("be", brainExtract)
}

func Register(name string, h Handler) {}
type Handler func(args []string) error
`
	if err := os.WriteFile(filepath.Join(tmpDir, "handlers.go"), []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	symbols, err := cg.Deadcode()
	if err != nil {
		t.Fatalf("Deadcode: %v", err)
	}

	for _, s := range symbols {
		if s.Name == "brainExtract" {
			t.Fatalf("function passed as argument should not be deadcode, got: %s %s at %s:%d", s.Type, s.Name, s.Path, s.StartLine)
		}
	}
}

func TestDeadcodeDoesNotFlagSideEffectImportedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	mainSrc := `package main

import (
	_ "foo/handlers"
)

func main() {}
`
	handlerSrc := `package handlers

func Init() {
	Register("help", func() {})
}

func Register(name string, h Handler) {}
type Handler func()
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainSrc), 0644); err != nil {
		t.Fatalf("write main fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "handlers.go"), []byte(handlerSrc), 0644); err != nil {
		t.Fatalf("write handlers fixture: %v", err)
	}

	cg, err := NewCodeGraph(tmpDir)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	if _, err := cg.Index(); err != nil {
		t.Fatalf("Index: %v", err)
	}

	result, err := cg.FindDeadCode("", 200)
	if err != nil {
		t.Fatalf("FindDeadCode: %v", err)
	}

	for _, of := range result.OrphanFiles {
		if of.Path == "handlers.go" {
			t.Fatalf("side-effect imported file should not be orphaned, got: %s", of.Path)
		}
	}
}
