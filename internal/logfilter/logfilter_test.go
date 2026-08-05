package logfilter

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

func capture(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = w
	os.Stderr = w
	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
	}()
	fn()
	_ = w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	wg.Wait()
	return buf.String()
}

func TestLevelFiltering(t *testing.T) {
	Setup("info")
	out := capture(func() {
		Info("visible info")
		Warn("visible warn")
		Debug("hidden debug")
		Debugf("hidden debugf %d", 1)
	})
	for _, want := range []string{"visible info", "visible warn"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	for _, banned := range []string{"hidden debug"} {
		if bytes.Contains([]byte(out), []byte(banned)) {
			t.Errorf("should not print %q in %q", banned, out)
		}
	}
}

func TestLevelOffStillPrintsBypass(t *testing.T) {
	Setup("off")
	out := capture(func() {
		Info("normal suppressed")
		Info("[ZEN-CLI] agent output must show")
		Info("RESULT: success")
		Info("🛑 block message")
	})
	for _, want := range []string{"[ZEN-CLI] agent output must show", "RESULT: success", "🛑 block message"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("bypass message %q missing", want)
		}
	}
	if bytes.Contains([]byte(out), []byte("normal suppressed")) {
		t.Errorf("normal message should be suppressed at off")
	}
}

func TestPrefixOverridesLevel(t *testing.T) {
	Setup("warn")
	out := capture(func() {
		Info("[WARN] escalated to warn")
		Info("plain info stays hidden")
		Error("[DEBUG] forced down to debug")
	})
	if !bytes.Contains([]byte(out), []byte("escalated to warn")) {
		t.Errorf("[WARN] prefix should override method default to warn")
	}
	if bytes.Contains([]byte(out), []byte("plain info stays hidden")) {
		t.Errorf("plain info should be hidden at warn level")
	}
	if bytes.Contains([]byte(out), []byte("forced down to debug")) {
		t.Errorf("[DEBUG] prefix should demote below warn level")
	}
}

func TestSecurityBypassVariants(t *testing.T) {
	Setup("off")
	cases := []string{
		"======",
		"------",
		"Description: something",
		"An agent is attempting",
		"To allow this",
		"To block",
		"Error: Action failed",
		"ACTIVE WORKSPACES: x",
		"AVAILABLE SKILLS: x",
		"KNOWLEDGE BASE: x",
		"Commands: x",
		"STATUS: x",
	}
	out := capture(func() {
		for _, c := range cases {
			Info(c)
		}
	})
	for _, c := range cases {
		if !bytes.Contains([]byte(out), []byte(c)) {
			t.Errorf("bypass case %q missing", c)
		}
	}
}

func TestStdioRedirect(t *testing.T) {
	Setup("debug")
	dir := t.TempDir()
	path := dir + "/debug.log"
	if err := SetStdioFile(path); err != nil {
		t.Fatal(err)
	}
	defer func() {
		mu.Lock()
		stdioFile = nil
		mu.Unlock()
	}()
	Info("log line")
	Debug("err line")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("[LOG]")) || !bytes.Contains(data, []byte("log line")) {
		t.Errorf("missing LOG line: %s", data)
	}
	if !bytes.Contains(data, []byte("[ERR]")) || !bytes.Contains(data, []byte("err line")) {
		t.Errorf("missing ERR line: %s", data)
	}
}
