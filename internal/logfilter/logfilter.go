package logfilter

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sevDebug = 0
	sevInfo  = 1
	sevWarn  = 2
	sevError = 3
	sevOff   = 4
)

var levelSeverity = map[string]int{
	"debug": sevDebug,
	"info":  sevInfo,
	"warn":  sevWarn,
	"error": sevError,
	"off":   sevOff,
}

var currentSeverity atomic.Int32

var (
	mu        sync.Mutex
	stdioFile *os.File
)

func severity(level string) int {
	if v, ok := levelSeverity[strings.ToLower(level)]; ok {
		return v
	}
	return sevDebug
}

func Setup(level string) {
	currentSeverity.Store(int32(severity(level)))
}

func SetStdioFile(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	mu.Lock()
	if stdioFile != nil {
		_ = stdioFile.Close()
	}
	stdioFile = f
	mu.Unlock()
	return nil
}

func shouldBypass(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "[ZEN-CLI]") ||
		strings.HasPrefix(trimmed, "⚠️  [SECURITY") ||
		strings.Contains(trimmed, "[SECURITY]") ||
		strings.HasPrefix(trimmed, "======") ||
		strings.HasPrefix(trimmed, "------") ||
		strings.HasPrefix(trimmed, "Description:") ||
		strings.HasPrefix(trimmed, "An agent is attempting") ||
		strings.HasPrefix(trimmed, "To allow this") ||
		strings.HasPrefix(trimmed, "To block") ||
		strings.HasPrefix(trimmed, "Error: Action") ||
		strings.HasPrefix(trimmed, "🛑") ||
		strings.HasPrefix(trimmed, "-") ||
		strings.HasPrefix(trimmed, "RESULT:") ||
		strings.HasPrefix(trimmed, "STATUS:") ||
		strings.HasPrefix(trimmed, "ACTIVE WORKSPACES:") ||
		strings.HasPrefix(trimmed, "AVAILABLE SKILLS:") ||
		strings.HasPrefix(trimmed, "KNOWLEDGE BASE:") ||
		strings.HasPrefix(trimmed, "Commands:")
}

func getMessageLevel(methodDefault, trimmed string) string {
	switch {
	case strings.HasPrefix(trimmed, "[DEBUG]"):
		return "debug"
	case strings.HasPrefix(trimmed, "[INFO]"):
		return "info"
	case strings.HasPrefix(trimmed, "[WARN]"), strings.HasPrefix(trimmed, "[WARNING]"):
		return "warn"
	case strings.HasPrefix(trimmed, "[ERROR]"):
		return "error"
	}
	return methodDefault
}

func Debug(args ...any) { emit("debug", args...) }
func Info(args ...any)  { emit("info", args...) }
func Warn(args ...any)  { emit("warn", args...) }
func Error(args ...any) { emit("error", args...) }

func Debugf(format string, args ...any) { emit("debug", fmt.Sprintf(format, args...)) }
func Infof(format string, args ...any)  { emit("info", fmt.Sprintf(format, args...)) }
func Warnf(format string, args ...any)  { emit("warn", fmt.Sprintf(format, args...)) }
func Errorf(format string, args ...any) { emit("error", fmt.Sprintf(format, args...)) }

func emit(methodDefault string, args ...any) {
	now := time.Now()
	ts := fmt.Sprintf("[%02d:%02d:%02d.%03d]", now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1e6)
	var trimmed string
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			trimmed = strings.TrimSpace(s)
		}
	}

	if shouldBypass(trimmed) {
		write(methodDefault, ts, args)
		return
	}

	msgLevel := getMessageLevel(methodDefault, trimmed)
	if severity(msgLevel) >= int(currentSeverity.Load()) {
		write(methodDefault, ts, args)
	}
}

func write(methodDefault, ts string, args []any) {
	line := ts + " " + join(args)

	mu.Lock()
	file := stdioFile
	mu.Unlock()
	if file != nil {
		prefix := "[LOG]"
		if methodDefault == "debug" || methodDefault == "error" {
			prefix = "[ERR]"
		}
		_, _ = fmt.Fprintf(file, "%s %s\n", prefix, line)
		return
	}

	out := os.Stderr
	if methodDefault != "error" {
		out = os.Stdout
	}
	_, _ = fmt.Fprintln(out, line)
}

func join(args []any) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch v := a.(type) {
		case string:
			b.WriteString(v)
		case error:
			b.WriteString(v.Error())
		case fmt.Stringer:
			b.WriteString(v.String())
		default:
			b.WriteString(fmt.Sprintf("%v", v))
		}
	}
	return b.String()
}
