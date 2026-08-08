package gatekeeper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"zen-mcp/internal/logfilter"
	"zen-mcp/internal/mcpcfg"
	"zen-mcp/internal/shared"
)

var systemRootsRegex = regexp.MustCompile(`(?i)^/(?:home|media|mnt|usr|etc|var|root|bin|sbin|lib|lib64|opt|srv|sys|proc|dev|boot|tmp|Users|Windows|Program Files|ProgramData|Program Files \(x86\))`)
var driveLetterRegex = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
var extensionRegex = regexp.MustCompile(`\.[a-zA-Z0-9]+$`)
var commandTokenRegex = regexp.MustCompile("[\x60\\s|&;<>()'\"]+")

type Decision struct {
	Confirmed  bool
	Suggestion string
}

type PendingInfo struct {
	ID          string
	Description string
	CreatedAt   time.Time
}

type pendingConfirmation struct {
	id          string
	description string
	createdAt   time.Time
	targetPath  string
	resolve     chan Decision
}

type Gatekeeper struct {
	mu                 sync.RWMutex
	store              *shared.Store
	cwd                string
	pending            map[string]*pendingConfirmation
	pendingOrder       []string
	nextID             int
	cachedAllowedPaths []string
	lastLoadedPath     string
}

func New(store *shared.Store) *Gatekeeper {
	cwd, _ := os.Getwd()
	return &Gatekeeper{
		store:              store,
		cwd:                cwd,
		pending:            map[string]*pendingConfirmation{},
		nextID:             1,
		cachedAllowedPaths: nil,
		lastLoadedPath:     "",
	}
}

func (g *Gatekeeper) GetActiveWorkspaceRoot() string {
	if g.store != nil {
		if ws, ok := g.store.Get("workspace-root"); ok && ws != "" {
			return ws
		}
	}
	if env := os.Getenv("MCP_WORKSPACE_ROOT"); env != "" {
		return env
	}
	cwd, _ := os.Getwd()
	return cwd
}

func IsLikelyFilePath(token string) bool {
	isAbsolute := strings.HasPrefix(token, "/") || strings.HasPrefix(token, "\\") || driveLetterRegex.MatchString(token)
	hasTraversal := strings.Contains(token, "..")

	if !isAbsolute && !hasTraversal {
		return false
	}
	if hasTraversal {
		return true
	}
	return systemRootsRegex.MatchString(token) || driveLetterRegex.MatchString(token) || extensionRegex.MatchString(token)
}

func (g *Gatekeeper) resolvePath(input string) string {
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	return filepath.Join(g.cwd, input)
}

func (g *Gatekeeper) getAllowedPathsFilePath() string {
	var activeWorkspace string
	if g.store != nil {
		if ws, ok := g.store.Get("workspace-root"); ok && ws != "" {
			activeWorkspace = ws
		}
	}
	if activeWorkspace == "" {
		activeWorkspace = os.Getenv("MCP_WORKSPACE_ROOT")
	}
	if activeWorkspace == "" {
		cwd, _ := os.Getwd()
		activeWorkspace = cwd
	}
	if activeWorkspace != "" {
		return filepath.Join(activeWorkspace, ".zenmcp", "allowed-paths.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "allowed-paths.json")
}

func (g *Gatekeeper) LoadAllowedPaths() []string {
	filePath := g.getAllowedPathsFilePath()
	if filePath == "" {
		return []string{}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cachedAllowedPaths != nil && g.lastLoadedPath == filePath {
		return g.cachedAllowedPaths
	}

	if data, err := os.ReadFile(filePath); err == nil {
		var paths []string
		if jsonErr := json.Unmarshal(data, &paths); jsonErr == nil && paths != nil {
			g.cachedAllowedPaths = paths
			g.lastLoadedPath = filePath
			return paths
		}
		logfilter.Debugf("[Gatekeeper] Failed to read allowed paths: %v", err)
	}

	g.cachedAllowedPaths = []string{}
	g.lastLoadedPath = filePath
	return []string{}
}

func (g *Gatekeeper) SaveAllowedPaths(paths []string) {
	filePath := g.getAllowedPathsFilePath()
	if filePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		logfilter.Debugf("[Gatekeeper] Failed to save allowed paths: %v", err)
		return
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		logfilter.Debugf("[Gatekeeper] Failed to save allowed paths: %v", err)
		return
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		logfilter.Debugf("[Gatekeeper] Failed to save allowed paths: %v", err)
		return
	}
	g.mu.Lock()
	g.cachedAllowedPaths = paths
	g.lastLoadedPath = filePath
	g.mu.Unlock()
}

func (g *Gatekeeper) AddAllowedPath(path string) {
	if c := mcpcfg.Get(); c != nil && !c.GatekeeperRemember {
		return
	}
	normalized := g.resolvePath(path)
	current := g.LoadAllowedPaths()
	if !containsStr(current, normalized) {
		current = append(current, normalized)
		g.SaveAllowedPaths(current)
		logfilter.Debugf("[Gatekeeper] Added and remembered allowed path: %s", normalized)
	}
}

func normalizeForMatch(p string) string {
	return strings.ToLower(strings.TrimRight(p, "/"))
}

func matchesRule(normalizedTarget, normalizedRule string) bool {
	return normalizedTarget == normalizedRule ||
		strings.HasPrefix(normalizedTarget, normalizedRule+"/") ||
		strings.HasPrefix(normalizedTarget, normalizedRule+"\\")
}

func (g *Gatekeeper) IsPathAllowed(path string) bool {
	normalizedTarget := normalizeForMatch(g.resolvePath(path))

	cfg := mcpcfg.Get()
	staticPaths := []string{}
	if cfg != nil {
		staticPaths = cfg.GatekeeperRememberPaths
	}
	for _, rule := range staticPaths {
		if matchesRule(normalizedTarget, normalizeForMatch(g.resolvePath(rule))) {
			return true
		}
	}

	if cfg == nil || cfg.GatekeeperRemember {
		for _, rule := range g.LoadAllowedPaths() {
			if matchesRule(normalizedTarget, normalizeForMatch(g.resolvePath(rule))) {
				return true
			}
		}
	}
	return false
}

func (g *Gatekeeper) GetDangerousRoots() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/",
		home,
		"/home",
		"/Users",
		"/mnt",
		"/media",
		"/tmp",
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Downloads"),
	}
}

func (g *Gatekeeper) GetRecursivelyRestrictedRoots() []string {
	return []string{
		"/etc", "/var", "/usr", "/boot", "/sys", "/proc", "/dev",
		"/root", "/bin", "/sbin", "/lib", "/lib64", "/opt", "/srv",
	}
}

func (g *Gatekeeper) IsPathUnderRestrictedRoot(targetPath string) bool {
	normalizedTarget := normalizeForMatch(g.resolvePath(targetPath))

	for _, d := range g.GetDangerousRoots() {
		if normalizedTarget == normalizeForMatch(g.resolvePath(d)) {
			return true
		}
	}
	for _, d := range g.GetRecursivelyRestrictedRoots() {
		normalizedDangerous := normalizeForMatch(g.resolvePath(d))
		if normalizedTarget == normalizedDangerous || strings.HasPrefix(normalizedTarget, normalizedDangerous+"/") {
			return true
		}
	}
	return false
}

func (g *Gatekeeper) RequestUserConfirmation(description, targetPath string) Decision {
	cfg := mcpcfg.Get()
	if cfg == nil || !cfg.GatekeeperEnabled {
		return Decision{Confirmed: true}
	}
	if !cfg.GatekeeperInteractive {
		allowed := cfg.GatekeeperInteractiveAuto == "accept"
		status := "blocked"
		if allowed {
			status = "allowed"
		}
		logfilter.Debugf("\n⚠️  [SECURITY WARNING] Action %s (non-interactive mode): %s\n", status, description)
		return Decision{Confirmed: allowed}
	}

	g.mu.Lock()
	id := fmt.Sprintf("%d", g.nextID)
	g.nextID++
	g.mu.Unlock()

	logfilter.Debugf("\n======================================================")
	logfilter.Debugf("⚠️  [SECURITY WARNING] SUSPICIOUS SYSTEM-LEVEL ACTION DETECTED")
	logfilter.Debugf("Description: %s", description)
	logfilter.Debugf("------------------------------------------------------")
	if os.Getenv("MCP_TRANSPORT") == "stdio" {
		allowed := cfg.GatekeeperInteractiveAuto == "accept"
		status := "blocked"
		if allowed {
			status = "allowed"
		}
		logfilter.Debugf("Error: Action is %s in STDIO mode because interactive confirmation is not supported.", status)
		logfilter.Debugf("======================================================\n")
		return Decision{Confirmed: allowed}
	}
	logfilter.Debugf("An agent is attempting to execute this action.")
	logfilter.Debugf("To allow this, type in your terminal: accept %s", id)
	logfilter.Debugf("To block/reject, type in your terminal: reject %s [optional suggestion]", id)
	logfilter.Debugf("======================================================\n")

	ch := make(chan Decision, 1)
	g.mu.Lock()
	conf := &pendingConfirmation{id: id, description: description, createdAt: time.Now(), targetPath: targetPath, resolve: ch}
	g.pending[id] = conf
	g.pendingOrder = append(g.pendingOrder, id)
	g.mu.Unlock()

	timeoutMs := 60000
	if cfg.GatekeeperInteractiveTimeout > 0 {
		timeoutMs = cfg.GatekeeperInteractiveTimeout
	}
	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case d := <-ch:
		return d
	case <-timer.C:
		g.mu.Lock()
		delete(g.pending, id)
		g.removeFromOrder(id)
		g.mu.Unlock()
		allowed := cfg.GatekeeperInteractiveAuto == "accept"
		status := "rejected"
		if allowed {
			status = "accepted"
		}
		logfilter.Debugf("\n⚠️  [SECURITY] Confirmation [%s] timed out and was %s automatically.\n", id, status)
		return Decision{Confirmed: allowed}
	}
}

func (g *Gatekeeper) AcceptConfirmation(id string) bool {
	g.mu.Lock()
	var conf *pendingConfirmation
	if id != "" {
		conf = g.pending[id]
	} else if len(g.pendingOrder) > 0 {
		last := g.pendingOrder[len(g.pendingOrder)-1]
		conf = g.pending[last]
	}
	if conf == nil {
		g.mu.Unlock()
		return false
	}
	delete(g.pending, conf.id)
	g.removeFromOrder(conf.id)
	g.mu.Unlock()

	if conf.targetPath != "" {
		g.AddAllowedPath(conf.targetPath)
	}
	conf.resolve <- Decision{Confirmed: true}
	return true
}

func (g *Gatekeeper) RejectConfirmation(id string, suggestion string) bool {
	g.mu.Lock()
	var conf *pendingConfirmation
	if id != "" {
		conf = g.pending[id]
	} else if len(g.pendingOrder) > 0 {
		last := g.pendingOrder[len(g.pendingOrder)-1]
		conf = g.pending[last]
	}
	if conf == nil {
		g.mu.Unlock()
		return false
	}
	delete(g.pending, conf.id)
	g.removeFromOrder(conf.id)
	g.mu.Unlock()

	conf.resolve <- Decision{Confirmed: false, Suggestion: suggestion}
	return true
}

func (g *Gatekeeper) removeFromOrder(id string) {
	for i, pid := range g.pendingOrder {
		if pid == id {
			g.pendingOrder = append(g.pendingOrder[:i], g.pendingOrder[i+1:]...)
			return
		}
	}
}

func (g *Gatekeeper) GetPendingConfirmations() []PendingInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]PendingInfo, 0, len(g.pending))
	for _, conf := range g.pending {
		out = append(out, PendingInfo{ID: conf.id, Description: conf.description, CreatedAt: conf.createdAt})
	}
	return out
}

func (g *Gatekeeper) ValidatePathSafety(path, operationName string) error {
	cfg := mcpcfg.Get()
	if cfg != nil && !cfg.GatekeeperEnabled {
		return nil
	}
	normalizedPath := g.resolvePath(path)

	zenRunPrefix := filepath.Join(os.TempDir(), "zen-run-")
	if strings.HasPrefix(normalizedPath, zenRunPrefix) {
		return nil
	}

	isRestricted := g.IsPathUnderRestrictedRoot(normalizedPath)

	var activeWorkspace string
	activeWorkspace = g.GetActiveWorkspaceRoot()

	isOutside := false
	workspaceMsg := ""
	if activeWorkspace != "" {
		normalizedTarget := normalizeForMatch(normalizedPath)
		normalizedWorkspace := normalizeForMatch(g.resolvePath(activeWorkspace))
		isInside := normalizedTarget == normalizedWorkspace ||
			strings.HasPrefix(normalizedTarget, normalizedWorkspace+"/") ||
			strings.HasPrefix(normalizedTarget, normalizedWorkspace+"\\")
		isOutside = !isInside
		workspaceMsg = fmt.Sprintf("outside active workspace %q", activeWorkspace)
	} else {
		isOutside = isRestricted
		workspaceMsg = "in restricted zone"
	}

	if isRestricted || isOutside {
		if g.IsPathAllowed(normalizedPath) {
			logfilter.Debugf("ℹ️  [SECURITY] Auto-accepted remembered path: %q", normalizedPath)
			return nil
		}

		reason := fmt.Sprintf("resides under a restricted system/user directory (%q)", normalizedPath)
		if !isRestricted {
			reason = fmt.Sprintf("resides outside the active workspace root (%q is %s)", normalizedPath, workspaceMsg)
		}

		description := fmt.Sprintf("[%s] Path access to %q - Reason: %s", operationName, normalizedPath, reason)
		decision := g.RequestUserConfirmation(description, normalizedPath)
		if !decision.Confirmed {
			suggestion := ""
			if decision.Suggestion != "" {
				suggestion = " Suggestion: " + decision.Suggestion
			}
			return fmt.Errorf("[%s] Security block: Path access to %q was rejected or timed out. (%s)%s", operationName, normalizedPath, reason, suggestion)
		}
	}
	return nil
}

func (g *Gatekeeper) ValidateCommandPayload(command, execDir string) error {
	cfg := mcpcfg.Get()
	if cfg != nil && !cfg.GatekeeperEnabled {
		return nil
	}
	tokens := commandTokenRegex.Split(command, -1)
	zenRunPrefix := filepath.Join(os.TempDir(), "zen-run-")

	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") || strings.HasPrefix(token, "ftp://") {
			continue
		}
		switch token {
		case "/", "\\", "//", "/*", "*/":
			continue
		}
		if !IsLikelyFilePath(token) {
			continue
		}

		absoluteTargetPath := token
		if filepath.IsAbs(token) {
			absoluteTargetPath = filepath.Clean(token)
		} else {
			absoluteTargetPath = filepath.Join(execDir, token)
		}

		if strings.HasPrefix(absoluteTargetPath, zenRunPrefix) {
			continue
		}

		isRestricted := g.IsPathUnderRestrictedRoot(absoluteTargetPath)

		var activeWorkspace string
		activeWorkspace = g.GetActiveWorkspaceRoot()
		isOutside := false
		workspaceMsg := ""
		if activeWorkspace != "" {
			normalizedTarget := normalizeForMatch(absoluteTargetPath)
			normalizedWorkspace := normalizeForMatch(g.resolvePath(activeWorkspace))
			isInside := normalizedTarget == normalizedWorkspace ||
				strings.HasPrefix(normalizedTarget, normalizedWorkspace+"/") ||
				strings.HasPrefix(normalizedTarget, normalizedWorkspace+"\\")
			isOutside = !isInside
			workspaceMsg = fmt.Sprintf("outside active workspace %q", activeWorkspace)
		} else {
			isOutside = isRestricted
			workspaceMsg = "in restricted zone"
		}

		if isRestricted || isOutside {
			if g.IsPathAllowed(absoluteTargetPath) {
				logfilter.Debugf("ℹ️  [SECURITY] Auto-accepted remembered command path: %q", absoluteTargetPath)
				continue
			}

			reason := fmt.Sprintf("resides under a restricted system/user directory (%q)", absoluteTargetPath)
			if !isRestricted {
				reason = fmt.Sprintf("resides outside the active workspace root (%q is %s)", absoluteTargetPath, workspaceMsg)
			}

			description := fmt.Sprintf("[Shell Guard] Execution of command containing suspicious path %q (resolves to %q). Reason: %s", token, absoluteTargetPath, reason)
			decision := g.RequestUserConfirmation(description, absoluteTargetPath)
			if !decision.Confirmed {
				suggestion := ""
				if decision.Suggestion != "" {
					suggestion = " Suggestion: " + decision.Suggestion
				}
				return fmt.Errorf("[Shell Guard] Security block: Command/script execution blocked. Suspect path: %q (resolves to %q).%s", token, absoluteTargetPath, suggestion)
			}
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
