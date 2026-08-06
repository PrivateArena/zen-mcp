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

func TestStoreBasics(t *testing.T) {
	st := NewStore()
	if _, ok := st.Get("workspace-root"); ok {
		t.Fatal("empty store should miss")
	}
	st.Set("workspace-root", "/a")
	if v, ok := st.Get("workspace-root"); !ok || v != "/a" {
		t.Errorf("Get = %q,%v", v, ok)
	}
	if got := st.GetAll(); len(got) != 1 || got["workspace-root"] != "/a" {
		t.Errorf("GetAll = %v", got)
	}

	var seen []string
	unsub := st.OnChange("workspace-root", func(v string) { seen = append(seen, v) })
	st.Set("workspace-root", "/b")
	if len(seen) != 1 || seen[0] != "/b" {
		t.Errorf("OnChange fired %v", seen)
	}
	// same value -> no notify
	st.Set("workspace-root", "/b")
	if len(seen) != 1 {
		t.Errorf("same-value Set should not notify: %v", seen)
	}
	unsub()
	st.Set("workspace-root", "/c")
	if len(seen) != 1 {
		t.Errorf("unsubscribed watcher should not fire: %v", seen)
	}

	st.Clear()
	if got := st.GetAll(); len(got) != 0 {
		t.Errorf("after Clear GetAll = %v", got)
	}
}

func TestStoreOnChangeOtherKey(t *testing.T) {
	st := NewStore()
	var fired bool
	st.OnChange("a", func(string) { fired = true })
	st.Set("b", "x")
	if fired {
		t.Error("watcher for key a should not fire for key b")
	}
}

func TestResolveWorkspacePriority(t *testing.T) {
	cwd := cwd(t)
	cases := []struct {
		name        string
		input       string
		registration string
		store       func() *Store
		env         string
		want        string
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
			name:     "4-env-workspace-root",
			input:    "",
			store:    func() *Store { return NewStore() },
			env:      "/env-ws",
			want:     "/env-ws",
		},
		{
			name:     "5-cwd-fallback",
			input:    "",
			store:    func() *Store { return NewStore() },
			want:     cwd,
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
