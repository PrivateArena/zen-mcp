package handlers

import "testing"

func TestParseExportCLIArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantClean bool
		wantShort bool
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

func TestExportShortFlag(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantOn  bool
		wantSet bool
	}{
		{name: "no flag", args: nil},
		{name: "unknown args", args: []string{"--anything"}},
		{name: "short long form", args: []string{"--short"}, wantOn: true, wantSet: true},
		{name: "short bare form", args: []string{"short"}, wantOn: true, wantSet: true},
		{name: "long long form", args: []string{"--long"}, wantSet: true},
		{name: "long bare form", args: []string{"long"}, wantSet: true},
		{name: "last one wins", args: []string{"--short", "--long"}, wantSet: true},
		{name: "last one wins short", args: []string{"--long", "short"}, wantOn: true, wantSet: true},
		{name: "short plus clean", args: []string{"--short", "--clean"}, wantOn: true, wantSet: true},
		{name: "mixed with unknown", args: []string{"extra", "--short"}, wantOn: true, wantSet: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			on, set := exportShortFlag(tc.args)
			if on != tc.wantOn || set != tc.wantSet {
				t.Errorf("exportShortFlag(%v) = (%v,%v), want (%v,%v)", tc.args, on, set, tc.wantOn, tc.wantSet)
			}
		})
	}
}
