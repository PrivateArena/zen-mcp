package shared

import (
	"os"
	"testing"
)

func cwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveWorkspacePriority(t *testing.T) {
	cwd := cwd(t)
	cases := []struct {
		name         string
		input        string
		registration string
		store        func() *Store
		env          string
		want         string
	}{
		{
			name:  "1-explicit-input-wins",
			input: "/explicit",
			want:  "/explicit",
		},
		{
			name:         "2-registration-next",
			input:        "",
			registration: "/reg-ws",
			want:         "/reg-ws",
		},
		{
			name:  "3-shared-workspace-root",
			input: "",
			store: func() *Store {
				st := NewStore()
				st.Set("workspace-root", "/shared-ws")
				return st
			},
			want: "/shared-ws",
		},
		{
			name:  "4-env-workspace-root",
			input: "",
			store: func() *Store { return NewStore() },
			env:   "/env-ws",
			want:  "/env-ws",
		},
		{
			name:  "5-cwd-fallback",
			input: "",
			store: func() *Store { return NewStore() },
			want:  cwd,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MCP_WORKSPACE_ROOT", tc.env)
			var st *Store
			if tc.store != nil {
				st = tc.store()
			}
			got := ResolveWorkspace(tc.input, tc.registration, st)
			if got != tc.want {
				t.Errorf("ResolveWorkspace = %q, want %q", got, tc.want)
			}
		})
	}
}
