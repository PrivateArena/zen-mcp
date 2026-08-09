package shared

import (
	"testing"
)

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
