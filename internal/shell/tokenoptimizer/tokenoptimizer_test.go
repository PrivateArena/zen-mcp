package tokenoptimizer

import (
	"testing"
)

func TestCountTokens(t *testing.T) {
	if got := CountTokens(""); got != 0 {
		t.Errorf("CountTokens('') = %d, want 0", got)
	}
	if got := CountTokens("abcd"); got != 1 {
		t.Errorf("CountTokens('abcd') = %d, want 1", got)
	}
	if got := CountTokens("abcdefgh"); got != 2 {
		t.Errorf("CountTokens('abcdefgh') = %d, want 2", got)
	}
}

func TestGetSavings(t *testing.T) {
	if got := GetSavings("", ""); got != 0 {
		t.Errorf("GetSavings('', '') = %d, want 0", got)
	}
	if got := GetSavings("abcdefgh", "abcd"); got != 50 {
		t.Errorf("GetSavings('abcdefgh', 'abcd') = %d, want 50", got)
	}
}

func TestCompactGitStatus(t *testing.T) {
	input := `M file1.go
 M file2.go
D  file3.go
?? file4.go
A  file5.go
M\tfile6.go
`
	got := compactGitStatus(input)
	if !contains(got, "Modified") || !contains(got, "Deleted") || !contains(got, "Untracked") || !contains(got, "Staged") {
		t.Errorf("compactGitStatus() missing sections: %s", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCompactGrep(t *testing.T) {
	input := "file1.go:10:foo\nfile1.go:20:bar\nfile2.go:5:baz\n"
	got := compactGrep(input, Options{})
	if !contains(got, "file1.go") || !contains(got, "file2.go") {
		t.Errorf("compactGrep() missing files: %s", got)
	}
}

func TestDeduplicateWithThreshold(t *testing.T) {
	input := "a\nb\na\na\nb\nc\n"
	got := deduplicateWithThreshold(input, 2)
	if !contains(got, "a (×3)") {
		t.Errorf("deduplicateWithThreshold() = %q, want ×3 for a", got)
	}
}

func TestApplyBlacklist(t *testing.T) {
	bl := []BlacklistEntry{{Match: "secret", DropOutput: true}}
	got := ApplyBlacklist("echo secret", "output", bl)
	if got == nil || *got == "output" {
		t.Errorf("ApplyBlacklist() = %v, want suppressed", got)
	}
}

func TestOptimizeOutputGitStatus(t *testing.T) {
	input := "M file1.go\n M file2.go\n"
	got := OptimizeOutput("git status", input, Options{}, Config{})
	if !contains(got, "Modified") {
		t.Errorf("OptimizeOutput(git status) = %q, want Modified section", got)
	}
}

func TestOptimizeOutputChained(t *testing.T) {
	input := "line1\nline1\nline1\n"
	got := OptimizeOutput("echo test && echo test", input, Options{}, Config{DeduplicateThreshold: 2})
	if !contains(got, "×3") {
		t.Errorf("OptimizeOutput(chained) = %q, want ×3", got)
	}
}

func TestApplyTokenProfilesMissingFile(t *testing.T) {
	res := ApplyTokenProfiles("test", "out", "", Options{}, Config{ProfilesPath: "/nonexistent/path.json"})
	if res.Applied {
		t.Errorf("ApplyTokenProfiles() applied = true, want false for missing file")
	}
}
