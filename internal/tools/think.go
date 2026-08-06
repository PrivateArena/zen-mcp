package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"

	"github.com/jang/zen-mcp/internal/logfilter"
	"github.com/jang/zen-mcp/internal/projectmemory"
	"github.com/jang/zen-mcp/internal/toolresponse"
)

func defThink(workspace string, _ Deps) ToolDef {
	return ToolDef{
		Name:        "think",
		Description: "Reasoning + planning for agents. Actions: sequential_thinking, create_plan, update_task, get_plan, add_task, finish_task.",
		Schema: jsonSchema(map[string]any{
			"action":            strEnumProp("Action", []string{"create_plan", "add_task", "update_task", "get_plan", "sequential_thinking", "finish_task"}),
			"project_name":      strProp("[create_plan] Project name"),
			"objective":         strProp("[create_plan] Session goal"),
			"tasks":             arrayStringProp("[create_plan] Task titles"),
			"id":                numProp("[update_task] Task ID"),
			"title":             strProp("[add_task] Task title"),
			"status":            strEnumProp("[update_task] New status", []string{"todo", "in_progress", "blocked", "done", "failed"}),
			"notes":             strProp("[update_task] Key findings or reason"),
			"thought":           strProp("[sequential_thinking] Current step [REQUIRED]"),
			"thoughtNumber":     intProp("[sequential_thinking] Step #, starting 1 [REQUIRED]"),
			"totalThoughts":     intProp("[sequential_thinking] Total steps; defaults to thoughtNumber"),
			"nextThoughtNeeded": boolProp("[sequential_thinking] Another thought needed; defaults true"),
			"isRevision":        boolProp("[sequential_thinking] Revising earlier thought"),
			"revisesThought":    intProp("[sequential_thinking] Thought being revised"),
			"branchFromThought": intProp("[sequential_thinking] Branch splits from thought #"),
			"branchId":          strProp("[sequential_thinking] Branch ID"),
		}, []string{"action"}),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleThinkAction(ctx, workspace, req), nil
		},
	}
}

func arrayStringProp(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": desc,
	}
}

func intProp(desc string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": desc,
	}
}

type thoughtData struct {
	Thought           string
	ThoughtNumber     int
	TotalThoughts     int
	IsRevision        bool
	RevisesThought    *int
	BranchFromThought *int
	BranchID          string
	NextThoughtNeeded bool
}

type sequentialThinkingServer struct {
	mu             sync.Mutex
	thoughtHistory []thoughtData
	branches       map[string][]thoughtData
}

func (s *sequentialThinkingServer) processThought(input thoughtData) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.ThoughtNumber > input.TotalThoughts {
		input.TotalThoughts = input.ThoughtNumber
	}
	s.thoughtHistory = append(s.thoughtHistory, input)

	if input.BranchFromThought != nil && input.BranchID != "" {
		if s.branches[input.BranchID] == nil {
			s.branches[input.BranchID] = []thoughtData{}
		}
		s.branches[input.BranchID] = append(s.branches[input.BranchID], input)
	}

	s.formatThought(input)

	branchKeys := []string{}
	for k := range s.branches {
		branchKeys = append(branchKeys, k)
	}
	sort.Strings(branchKeys)
	result := map[string]any{"next": input.NextThoughtNeeded}
	if len(branchKeys) > 0 {
		result["branches"] = branchKeys
	}
	return result
}

func (s *sequentialThinkingServer) formatThought(input thoughtData) {
	prefix := "💭 Thought"
	context := ""
	if input.IsRevision {
		prefix = "🔄 Revision"
		context = fmt.Sprintf(" (revising thought %d)", derefInt(input.RevisesThought))
	} else if input.BranchFromThought != nil {
		prefix = "🌿 Branch"
		context = fmt.Sprintf(" (from thought %d, ID: %s)", *input.BranchFromThought, input.BranchID)
	}

	header := fmt.Sprintf("%s %d/%d%s", prefix, input.ThoughtNumber, input.TotalThoughts, context)
	inner := input.Thought
	borderLen := len([]rune(header))
	if rl := len([]rune(inner)); rl > borderLen {
		borderLen = rl
	}
	borderLen += 4
	border := strings.Repeat("─", borderLen)
	pad := borderLen - 2

	var b strings.Builder
	fmt.Fprintf(&b, "\n┌%s┐\n", border)
	fmt.Fprintf(&b, "│ %s%s │\n", header, strings.Repeat(" ", pad-len([]rune(header))))
	fmt.Fprintf(&b, "├%s┤\n", border)
	fmt.Fprintf(&b, "│ %s%s │\n", inner, strings.Repeat(" ", pad-len([]rune(inner))))
	fmt.Fprintf(&b, "└%s┘", border)
	logfilter.Info(b.String())
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

var thinkingServer = &sequentialThinkingServer{branches: map[string][]thoughtData{}}

// ---- PlanManager ----

type taskStatus string

const (
	statusTodo       taskStatus = "todo"
	statusInProgress taskStatus = "in_progress"
	statusBlocked    taskStatus = "blocked"
	statusDone       taskStatus = "done"
	statusFailed     taskStatus = "failed"
)

type task struct {
	ID     int        `json:"id"`
	Title  string     `json:"title"`
	Status taskStatus `json:"status"`
	Notes  *string    `json:"notes,omitempty"`
}

type planData struct {
	ProjectName string `json:"projectName"`
	Objective   string `json:"objective"`
	Tasks       []task `json:"tasks"`
}

type planManager struct {
	workspace string
}

func (p *planManager) getPlanFile() (string, error) {
	root := p.workspace
	if root == "" {
		cwd, _ := os.Getwd()
		root = cwd
	}
	dataDir := filepath.Join(root, ".zenmcp")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return "", err
		}
	}
	return filepath.Join(dataDir, "plan.json"), nil
}

func (p *planManager) loadPlan() *planData {
	file, err := p.getPlanFile()
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var plan planData
	if json.Unmarshal(raw, &plan) != nil {
		return nil
	}
	return &plan
}

func (p *planManager) savePlan(plan planData) error {
	file, err := p.getPlanFile()
	if err != nil {
		return err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	p.printTaskBoard(plan)
	return nil
}

func (p *planManager) printTaskBoard(plan planData) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n 📋 %s ", plan.ProjectName)
	fmt.Fprintf(&b, "\n 🎯 %s", plan.Objective)
	b.WriteString("\n ─────────────────────────────────────────")
	for _, t := range plan.Tasks {
		icon := map[taskStatus]string{statusTodo: "[ ]", statusInProgress: "[▶]", statusBlocked: "[!]", statusDone: "[✔]", statusFailed: "[✘]"}[t.Status]
		fmt.Fprintf(&b, "\n %s %d: %s", icon, t.ID, t.Title)
		if t.Notes != nil && *t.Notes != "" {
			fmt.Fprintf(&b, "\n       ↳ %s", *t.Notes)
		}
	}
	b.WriteString("\n ─────────────────────────────────────────\n")
	logfilter.Info(b.String())
}

func (p *planManager) compactBoard(plan planData) string {
	header := "[" + plan.ProjectName + "] " + plan.Objective
	rows := make([]string, 0, len(plan.Tasks))
	for _, t := range plan.Tasks {
		row := fmt.Sprintf("%d:%s %s", t.ID, t.Status, t.Title)
		if t.Notes != nil && *t.Notes != "" {
			row += " (" + *t.Notes + ")"
		}
		rows = append(rows, row)
	}
	return strings.Join(append([]string{header}, rows...), "\n")
}

func (p *planManager) nextActionableTask(plan planData) *task {
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == statusInProgress {
			return &plan.Tasks[i]
		}
	}
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == statusTodo {
			return &plan.Tasks[i]
		}
	}
	return nil
}

func (p *planManager) createPlan(projectName, objective string, taskTitles []string) string {
	tasks := make([]task, 0, len(taskTitles))
	for i, title := range taskTitles {
		tasks = append(tasks, task{ID: i + 1, Title: title, Status: statusTodo})
	}
	_ = p.savePlan(planData{ProjectName: projectName, Objective: objective, Tasks: tasks})

	var first string
	if len(tasks) > 0 {
		first = fmt.Sprintf(" → %d: %s", tasks[0].ID, tasks[0].Title)
	}
	return fmt.Sprintf("Plan ready (%d tasks). Start%s", len(tasks), first)
}

func (p *planManager) addTask(title string) (string, error) {
	plan := p.loadPlan()
	if plan == nil {
		return "", errors.New("No plan. Use create_plan first.")
	}
	id := 1
	if len(plan.Tasks) > 0 {
		maxID := plan.Tasks[0].ID
		for _, t := range plan.Tasks {
			if t.ID > maxID {
				maxID = t.ID
			}
		}
		id = maxID + 1
	}
	plan.Tasks = append(plan.Tasks, task{ID: id, Title: title, Status: statusTodo})
	_ = p.savePlan(*plan)
	return fmt.Sprintf("Added task %d: %s", id, title), nil
}

func (p *planManager) updateTask(id int, status taskStatus, notes *string) (string, error) {
	plan := p.loadPlan()
	if plan == nil {
		return "", errors.New("No plan.")
	}
	var target *task
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == id {
			target = &plan.Tasks[i]
			break
		}
	}
	if target == nil {
		ids := make([]string, 0, len(plan.Tasks))
		for _, t := range plan.Tasks {
			ids = append(ids, itoa(t.ID))
		}
		return "", fmt.Errorf("Task %d not found. Valid IDs: %s", id, strings.Join(ids, ","))
	}
	target.Status = status
	if notes != nil {
		target.Notes = notes
	}
	_ = p.savePlan(*plan)

	next := p.nextActionableTask(*plan)
	suffix := "\nAll tasks complete."
	if next != nil {
		suffix = fmt.Sprintf("\nNext → %d:%s %s", next.ID, next.Status, next.Title)
	}
	return fmt.Sprintf("Task %d → %s.%s", id, status, suffix), nil
}

func (p *planManager) getPlan() (string, error) {
	plan := p.loadPlan()
	if plan == nil {
		return "No plan. Use create_plan.", nil
	}
	p.printTaskBoard(*plan)
	return p.compactBoard(*plan), nil
}

func (p *planManager) finishTask() (string, error) {
	plan := p.loadPlan()
	if plan == nil {
		return "", errors.New("No plan.")
	}
	updatedCount := 0
	for i := range plan.Tasks {
		if plan.Tasks[i].Status != statusDone {
			plan.Tasks[i].Status = statusDone
			updatedCount++
		}
	}
	if updatedCount > 0 {
		_ = p.savePlan(*plan)
	}
	return fmt.Sprintf("Finished all tasks (%d marked done).", updatedCount), nil
}

func HandleThinkAction(ctx context.Context, workspace string, req mcp.CallToolRequest) *mcp.CallToolResult {
	start := time.Now()
	args := req.GetArguments()
	action, _ := args["action"].(string)

	var result *mcp.CallToolResult
	pm := &planManager{workspace: workspace}
	fail := func(msg string) {
		result = toolresponse.WrapErrorWithContext(ctx, "think", errors.New(msg), start)
	}

	switch action {
	case "create_plan":
		missing := missingKeys(args, []string{"project_name", "objective", "tasks"})
		if missing != "" {
			fail("create_plan missing: " + missing)
			break
		}
		titles := toStringSlice(args["tasks"])
		result = toolresponse.WrapSuccess(ctx, "think",
			pm.createPlan(args["project_name"].(string), args["objective"].(string), titles), start)
	case "add_task":
		title, _ := args["title"].(string)
		if title == "" {
			fail("add_task missing: title")
			break
		}
		msg, err := pm.addTask(title)
		if err != nil {
			fail(err.Error())
			break
		}
		result = toolresponse.WrapSuccess(ctx, "think", msg, start)
	case "update_task":
		missing := missingKeys(args, []string{"id", "status"})
		if missing != "" {
			fail("update_task missing: " + missing)
			break
		}
		id := toInt(args["id"])
		status := taskStatus(args["status"].(string))
		var notes *string
		if n, ok := args["notes"].(string); ok {
			notes = &n
		}
		msg, err := pm.updateTask(id, status, notes)
		if err != nil {
			fail(err.Error())
			break
		}
		result = toolresponse.WrapSuccess(ctx, "think", msg, start)
	case "get_plan":
		msg, _ := pm.getPlan()
		result = toolresponse.WrapSuccess(ctx, "think", msg, start)
	case "finish_task":
		msg, err := pm.finishTask()
		if err != nil {
			fail(err.Error())
			break
		}
		result = toolresponse.WrapSuccess(ctx, "think", msg, start)
	case "sequential_thinking":
		missing := missingKeys(args, []string{"thought", "thoughtNumber"})
		if missing != "" {
			fail("sequential_thinking missing: " + missing)
			break
		}
		input := thoughtData{
			Thought:           args["thought"].(string),
			ThoughtNumber:     toInt(args["thoughtNumber"]),
			TotalThoughts:     toInt(args["totalThoughts"]),
			NextThoughtNeeded: true,
			IsRevision:        toBool(args["isRevision"]),
			BranchID:          toString(args["branchId"]),
		}
		if v, ok := args["totalThoughts"]; !ok || v == nil {
			input.TotalThoughts = input.ThoughtNumber
		}
		if v, ok := args["nextThoughtNeeded"].(bool); ok {
			input.NextThoughtNeeded = v
		}
		if v, ok := toIntPtr(args["revisesThought"]); ok {
			input.RevisesThought = v
		}
		if v, ok := toIntPtr(args["branchFromThought"]); ok {
			input.BranchFromThought = v
		}
		res := thinkingServer.processThought(input)
		raw, _ := json.Marshal(res)
		result = toolresponse.WrapSuccess(ctx, "think", string(raw), start)
	default:
		fail("Unknown action.")
	}

	logSessionEvent(workspace, "think", "think: "+action, thinkLogContent(action, args, result))
	return result
}

func thinkLogContent(action string, args map[string]any, result *mcp.CallToolResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Action: %s\n", action)
	if thought, ok := args["thought"].(string); ok && thought != "" {
		tn := toInt(args["thoughtNumber"])
		tt := toInt(args["totalThoughts"])
		if tt == 0 {
			tt = tn
		}
		fmt.Fprintf(&b, "Thought #%d/%d: %s\n", tn, tt, thought)
	} else {
		raw, _ := json.Marshal(args)
		fmt.Fprintf(&b, "Params: %s\n", string(raw))
	}
	if result != nil && len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			fmt.Fprintf(&b, "Result:\n%s", tc.Text)
		}
	}
	return b.String()
}

func logSessionEvent(workspace, typ, title, content string) {
	if workspace == "" {
		return
	}
	dbPath := filepath.Join(workspace, ".zenmcp", "context.db")
	projectmemory.LogProjectEvent(dbPath, typ, title, content, map[string]any{"session_token": workspace})
}

// ---- helpers ----

func missingKeys(args map[string]any, keys []string) string {
	var missing []string
	for _, k := range keys {
		if v, ok := args[k]; !ok || v == nil {
			missing = append(missing, k)
		}
	}
	return strings.Join(missing, ", ")
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func toIntPtr(v any) (*int, bool) {
	if v == nil {
		return nil, false
	}
	if n, ok := v.(float64); ok {
		i := int(n)
		return &i, true
	}
	return nil, false
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}
