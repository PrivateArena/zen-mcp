package handlers

import "testing"

func TestParseExportCLIArgs(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantClean   bool
		wantShort   bool
	}{
		{name: "empty defaults to plain export", args: nil},
		{name: "no flags", args: []string{"--anything"}},
		{name: "short long form", args: []string{"--short"}, wantShort: true},
		{name: "short bare form", args: []string{"short"}, wantShort: true},
		{name: "clean long form", args: []string{"--clean"}, wantClean: true},
		{name: "clean bare form", args: []string{"clean"}, wantClean: true},
		{name: "short plus clean", args: []string{"--short", "--clean"}, wantClean: true, wantShort: true},
		{name: "mixed with unknown", args: []string{"extra", "--short", "clean"}, wantClean: true, wantShort: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clean, short := parseExportCLIArgs(tc.args)
			if clean != tc.wantClean {
				t.Errorf("clean = %v, want %v", clean, tc.wantClean)
			}
			if short != tc.wantShort {
				t.Errorf("short = %v, want %v", short, tc.wantShort)
			}
		})
	}
}
