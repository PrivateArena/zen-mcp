package tools

import (
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("firstNonEmpty() = %q, want %q", got, "a")
	}
	if got := firstNonEmpty("", "b"); got != "b" {
		t.Errorf("firstNonEmpty() = %q, want %q", got, "b")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty() = %q, want %q", got, "")
	}
}

func TestResolveWorkspaceFromDeps(t *testing.T) {
	if got := resolveWorkspaceFromDeps("/tmp/ws", ""); got != "/tmp/ws" {
		t.Errorf("resolveWorkspaceFromDeps() = %q, want %q", got, "/tmp/ws")
	}
	if got := resolveWorkspaceFromDeps("", "/tmp/ws"); got != "/tmp/ws" {
		t.Errorf("resolveWorkspaceFromDeps() = %q, want %q", got, "/tmp/ws")
	}
}

func TestDependenciesOrEmpty(t *testing.T) {
	got := dependenciesOrEmpty(map[string]any{})
	if got == nil {
		t.Errorf("dependenciesOrEmpty(empty) = nil, want []any{}")
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 0 {
		t.Errorf("dependenciesOrEmpty(empty) = %v, want []any{}", got)
	}
	got = dependenciesOrEmpty(map[string]any{"dependencies": []any{"a"}})
	arr, ok = got.([]any)
	if !ok || len(arr) != 1 {
		t.Errorf("dependenciesOrEmpty() = %v, want [a]", got)
	}
}

func TestFormatPercent(t *testing.T) {
	if got := formatPercent(0.95); got != "95%" {
		t.Errorf("formatPercent(0.95) = %q, want %q", got, "95%")
	}
	if got := formatPercent(0.0); got != "0%" {
		t.Errorf("formatPercent(0.0) = %q, want %q", got, "0%")
	}
}

func TestItoa(t *testing.T) {
	if got := itoa(0); got != "0" {
		t.Errorf("itoa(0) = %q, want %q", got, "0")
	}
	if got := itoa(123); got != "123" {
		t.Errorf("itoa(123) = %q, want %q", got, "123")
	}
	if got := itoa(-5); got != "-5" {
		t.Errorf("itoa(-5) = %q, want %q", got, "-5")
	}
}

func TestPlanManagerNextActionableTask(t *testing.T) {
	pm := &planManager{}
	plan := planData{
		Tasks: []task{
			{ID: 1, Status: "todo"},
			{ID: 2, Status: "in_progress"},
			{ID: 3, Status: "todo"},
		},
	}
	next := pm.nextActionableTask(plan)
	if next == nil || next.ID != 2 {
		t.Errorf("nextActionableTask() = %v, want ID=2", next)
	}

	plan.Tasks[1].Status = "done"
	next = pm.nextActionableTask(plan)
	if next == nil || next.ID != 1 {
		t.Errorf("nextActionableTask() = %v, want ID=1", next)
	}
}

func TestPlanManagerCreatePlan(t *testing.T) {
	pm := &planManager{}
	msg := pm.createPlan("P", "O", []string{"t1", "t2"})
	if msg != "Plan ready (2 tasks). Start → 1: t1" {
		t.Errorf("createPlan() = %q, want %q", msg, "Plan ready (2 tasks). Start → 1: t1")
	}
}

func TestPlanManagerAddTask(t *testing.T) {
	pm := &planManager{}
	plan := planData{ProjectName: "P", Objective: "O", Tasks: []task{{ID: 1, Title: "t1", Status: "todo"}}}
	_ = pm.savePlan(plan)

	msg, err := pm.addTask("t2")
	if err != nil {
		t.Fatalf("addTask() error = %v", err)
	}
	if msg != "Added task 2: t2" {
		t.Errorf("addTask() = %q, want %q", msg, "Added task 2: t2")
	}
}

func TestPlanManagerUpdateTask(t *testing.T) {
	pm := &planManager{}
	plan := planData{ProjectName: "P", Objective: "O", Tasks: []task{{ID: 1, Title: "t1", Status: "todo"}}}
	_ = pm.savePlan(plan)

	msg, err := pm.updateTask(1, "done", nil)
	if err != nil {
		t.Fatalf("updateTask() error = %v", err)
	}
	if msg != "Task 1 → done.\nAll tasks complete." {
		t.Errorf("updateTask() = %q, want %q", msg, "Task 1 → done.\nAll tasks complete.")
	}
}

func TestPlanManagerFinishTask(t *testing.T) {
	pm := &planManager{}
	plan := planData{ProjectName: "P", Objective: "O", Tasks: []task{
		{ID: 1, Title: "t1", Status: "todo"},
		{ID: 2, Title: "t2", Status: "done"},
	}}
	_ = pm.savePlan(plan)

	msg, err := pm.finishTask()
	if err != nil {
		t.Fatalf("finishTask() error = %v", err)
	}
	if msg != "Finished all tasks (1 marked done)." {
		t.Errorf("finishTask() = %q, want %q", msg, "Finished all tasks (1 marked done).")
	}
}
