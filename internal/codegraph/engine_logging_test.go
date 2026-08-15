package codegraph

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns every
// line written to it. logfilter's Info/Debug emit paths write to os.Stdout
// (logfilter.go:146), so this exercises the real emission path end to end.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	t.Cleanup(func() {
		_ = w.Close()
		_ = r.Close()
		os.Stdout = old
		<-done
	})

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// TestIndexEmitsPhaseTimingLogs guards the indexing process-logging contract:
// Index must emit per-phase timings plus a slowest-file annotation so a
// performance profile can attribute the bottleneck without instrumenting the
// binary. The single file makes the slowest annotation deterministic.
func TestIndexEmitsPhaseTimingLogs(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "calc.go", "package p\n\nfunc Add(a int, b int) int {\n\treturn a + b\n}\n")

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	out := captureStdout(t, func() {
		if _, err := cg.Index(); err != nil {
			t.Fatalf("Index: %v", err)
		}
	})

	for _, want := range []string{
		"[CodeGraph] Scan: 1 file(s) to process in ",
		"[CodeGraph] Phase 1 (parse): 1 file(s), 1 node(s), 0 relation(s) in ",
		"; slowest: calc.go (",
		"[CodeGraph] parse calc.go: 1 node(s), 0 relation(s) in ",
		"[CodeGraph] Phase 2 (write nodes): 1 file(s) in ",
		"[CodeGraph] Phase 3 (edges): 0 relation(s) -> 0 edge(s) across 1 file(s) in ",
		"[CodeGraph] Cleanup deleted: 0 file(s) in ",
		"[CodeGraph] Manifests: ",
		"[CodeGraph] Index total: ",
		"(1 indexed, 0 deleted)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected log output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestIndexEmitsPhaseTimingLogsEmpty guards the no-op index path: an empty
// workspace must still log each phase with zero counts and must not emit a
// slowest-file annotation (there is no file to attribute).
func TestIndexEmitsPhaseTimingLogsEmpty(t *testing.T) {
	tmp := t.TempDir()

	cg, err := NewCodeGraph(tmp)
	if err != nil {
		t.Fatalf("NewCodeGraph: %v", err)
	}
	defer cg.Close()

	out := captureStdout(t, func() {
		if _, err := cg.Index(); err != nil {
			t.Fatalf("Index: %v", err)
		}
	})

	for _, want := range []string{
		"[CodeGraph] Scan: 0 file(s) to process in ",
		"[CodeGraph] Phase 1 (parse): 0 file(s), 0 node(s), 0 relation(s) in ",
		"[CodeGraph] Phase 2 (write nodes): 0 file(s) in ",
		"[CodeGraph] Phase 3 (edges): 0 relation(s) -> 0 edge(s) across 0 file(s) in ",
		"[CodeGraph] Cleanup deleted: 0 file(s) in ",
		"[CodeGraph] Index total: ",
		"(0 indexed, 0 deleted)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected log output to contain %q, got:\n%s", want, out)
		}
	}

	if strings.Contains(out, "; slowest:") {
		t.Errorf("expected no slowest annotation for empty index, got:\n%s", out)
	}
}
