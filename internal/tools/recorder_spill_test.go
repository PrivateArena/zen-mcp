package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A recorder index larger than the spill threshold must land in the workspace's recorder
// directory, and keep enough of itself inline that an agent can still decide what to query next.
func TestRecorderReplyDocumentSpillsWithPreview(t *testing.T) {
	ws := t.TempDir()
	big := strings.Repeat(`{"seq":1,"url":"https://example.com/api"}`, chatSpillThresholdBytes()+10)

	out, replaced := recorderReplyDocument("recorder_stop", big, ws, Deps{})
	if !replaced {
		t.Fatal("an oversized index should have been spilled")
	}
	if !strings.Contains(out, filepath.Join(".zenmcp", "recorder", "recorder_stop-")) {
		t.Errorf("reply does not point at the recorder directory: %.160s", out)
	}
	if !strings.Contains(out, "Preview:") {
		t.Error("spilled index lost its inline preview; a bare filename is not actionable")
	}
	if len(out) >= len(big)/2 {
		t.Errorf("spilled reply is %.1f KB, barely smaller than the %.1f KB original",
			float64(len(out))/1024, float64(len(big))/1024)
	}

	dir := filepath.Join(ws, ".zenmcp", "recorder")
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one spilled file in %s: %v (%v)", dir, files, err)
	}
	if !strings.HasPrefix(files[0].Name(), "recorder_stop-") {
		t.Errorf("unexpected file name %q", files[0].Name())
	}
}

// A reply under the threshold must come back untouched - the spill is a safety net, not a
// reformatting step.
func TestRecorderReplyDocumentKeepsSmallRepliesInline(t *testing.T) {
	small := `{"success":true,"entries":3}`
	out, replaced := recorderReplyDocument("recorder_list", small, t.TempDir(), Deps{})
	if replaced || out != small {
		t.Errorf("small reply was altered: replaced=%v out=%.80s", replaced, out)
	}
}

// Spilled recorder indexes get their own directory: chat_output_path describes conversations, and
// a traffic capture belongs somewhere an agent can name.
func TestRecorderSpillDirectory(t *testing.T) {
	ws := t.TempDir()
	want := filepath.Join(ws, ".zenmcp", "recorder")
	if got := recorderDir(ws, Deps{}); got != want {
		t.Errorf("recorder dir = %q, want %q", got, want)
	}
	if chatOutputDir(ws, Deps{}) == recorderDir(ws, Deps{}) {
		t.Fatal("chat and recorder spill directories must differ")
	}
}
