package telemetry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/jang/zen-mcp/internal/mcpcfg"
)

var (
	mu sync.Mutex
	db *sql.DB
)

func getDb() (*sql.DB, error) {
	mu.Lock()
	defer mu.Unlock()
	if db != nil {
		return db, nil
	}
	dir := mcpcfg.TelemetryDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "telemetry.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(3000)&_pragma=journal_mode(WAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS tool_calls (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      tool TEXT NOT NULL,
      action TEXT,
      success INTEGER NOT NULL DEFAULT 1,
      error_message TEXT,
      timestamp TEXT NOT NULL DEFAULT (datetime('now'))
    )`); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_telemetry_tool ON tool_calls(tool)`); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_telemetry_ts ON tool_calls(timestamp)`); err != nil {
		_ = conn.Close()
		return nil, err
	}
	db = conn
	return db, nil
}

func LogToolCall(tool string, action string, success bool, errorMessage string) error {
	if c := mcpcfg.Get(); c != nil && !c.TelemetryEnabled {
		return nil
	}
	d, err := getDb()
	if err != nil {
		return err
	}
	s := 0
	if success {
		s = 1
	}
	var actionArg any
	if action != "" {
		actionArg = action
	}
	var errArg any
	if errorMessage != "" {
		errArg = errorMessage
	}
	_, err = d.Exec(`INSERT INTO tool_calls (tool, action, success, error_message, timestamp) VALUES (?, ?, ?, ?, datetime('now'))`,
		tool, actionArg, s, errArg)
	return err
}

func QueryTelemetry(args []string) string {
	defer func() {
		if r := recover(); r != nil {
			// never break app for telemetry
		}
	}()
	d, err := getDb()
	if err != nil {
		return fmt.Sprintf("[TELEMETRY] Error: %v", err)
	}
	cmd := "summary"
	if len(args) > 0 && args[0] != "" {
		cmd = args[0]
	}
	switch cmd {
	case "tools":
		rows, err := d.Query(`SELECT tool, COUNT(*) as calls, ROUND(100.0 * SUM(success) / COUNT(*), 1) as rate FROM tool_calls GROUP BY tool ORDER BY calls DESC LIMIT 20`)
		if err != nil {
			return fmt.Sprintf("[TELEMETRY] Error: %v", err)
		}
		defer rows.Close()
		var out strings.Builder
		out.WriteString("[TELEMETRY] Top tools:\n")
		for rows.Next() {
			var tool string
			var calls int
			var rate float64
			if err := rows.Scan(&tool, &calls, &rate); err != nil {
				continue
			}
			fmt.Fprintf(&out, "  %s: %d calls (%.1f%% success)\n", tool, calls, rate)
		}
		return strings.TrimRight(out.String(), "\n")
	case "actions":
		rows, err := d.Query(`SELECT tool, action, COUNT(*) as calls, ROUND(100.0 * SUM(success) / COUNT(*), 1) as rate FROM tool_calls WHERE action IS NOT NULL GROUP BY tool, action ORDER BY calls DESC LIMIT 20`)
		if err != nil {
			return fmt.Sprintf("[TELEMETRY] Error: %v", err)
		}
		defer rows.Close()
		var out strings.Builder
		out.WriteString("[TELEMETRY] Top actions:\n")
		for rows.Next() {
			var tool, action string
			var calls int
			var rate float64
			if err := rows.Scan(&tool, &action, &calls, &rate); err != nil {
				continue
			}
			fmt.Fprintf(&out, "  %s.%s: %d calls (%.1f%% success)\n", tool, action, calls, rate)
		}
		return strings.TrimRight(out.String(), "\n")
		case "failures":
			if len(args) > 1 && args[1] == "--top" {
				rows, err := d.Query(`SELECT tool, action, error_message, COUNT(*) as count FROM tool_calls WHERE success = 0 AND error_message IS NOT NULL GROUP BY tool, action, error_message ORDER BY count DESC LIMIT 20`)
				if err != nil {
					return fmt.Sprintf("[TELEMETRY] Error: %v", err)
				}
				defer rows.Close()
				var out strings.Builder
				out.WriteString("[TELEMETRY] Top failure reasons:\n")
				i := 1
				for rows.Next() {
					var tool string
					var action sql.NullString
					var errMsg string
					var count int
					if err := rows.Scan(&tool, &action, &errMsg, &count); err != nil {
						continue
					}
					label := tool
					if action.Valid && action.String != "" {
						label = tool + "." + action.String
					}
					fmt.Fprintf(&out, "  %d. %s: \"%s\" (%dx)\n", i, label, errMsg, count)
					i++
				}
				return strings.TrimRight(out.String(), "\n")
			}
			rows, err := d.Query(`SELECT tool, action, error_message, timestamp FROM tool_calls WHERE success = 0 ORDER BY id DESC LIMIT 20`)
			if err != nil {
				return fmt.Sprintf("[TELEMETRY] Error: %v", err)
			}
			defer rows.Close()
			var out strings.Builder
			out.WriteString("[TELEMETRY] Recent failures:\n")
			for rows.Next() {
				var tool string
				var action sql.NullString
				var errMsg sql.NullString
				var ts string
				if err := rows.Scan(&tool, &action, &errMsg, &ts); err != nil {
					continue
				}
				label := tool
				if action.Valid && action.String != "" {
					label = tool + "." + action.String
				}
				msg := ""
				if errMsg.Valid {
					msg = errMsg.String
				}
				fmt.Fprintf(&out, "  %s %s: %s\n", ts, label, msg)
			}
			return strings.TrimRight(out.String(), "\n")
	case "reset":
		if _, err := d.Exec(`DELETE FROM tool_calls`); err != nil {
			return fmt.Sprintf("[TELEMETRY] Error: %v", err)
		}
		if _, err := d.Exec(`VACUUM`); err != nil {
			return fmt.Sprintf("[TELEMETRY] Error: %v", err)
		}
		return "[TELEMETRY] Cleared all telemetry data."
	case "enable":
		if c := mcpcfg.Get(); c != nil {
			c.TelemetryEnabled = true
		}
		return "[TELEMETRY] Enabled."
	case "disable":
		if c := mcpcfg.Get(); c != nil {
			c.TelemetryEnabled = false
		}
		return "[TELEMETRY] Disabled."
	case "summary":
		return telemetrySummary(d)
	}
	return telemetrySummary(d)
}

func telemetrySummary(d *sql.DB) string {
	enabled := true
	if c := mcpcfg.Get(); c != nil {
		enabled = c.TelemetryEnabled
	}
	var total int
	var rate float64
	var failures int
	_ = d.QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&total)
	if total > 0 {
		_ = d.QueryRow(`SELECT ROUND(100.0 * SUM(success) / COUNT(*), 1) FROM tool_calls`).Scan(&rate)
	}
	_ = d.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE success = 0`).Scan(&failures)

	var out strings.Builder
	status := "OFF"
	if enabled {
		status = "ON"
	}
	fmt.Fprintf(&out, "[TELEMETRY] Status: %s\n", status)
	fmt.Fprintf(&out, "  Total calls: %d\n", total)
	fmt.Fprintf(&out, "  Success rate: %s%%\n", formatRate(rate))
	fmt.Fprintf(&out, "  Failures: %d\n", failures)

	if total > 0 {
		out.WriteString("  Most used tools:\n")
		if rows, err := d.Query(`SELECT tool, COUNT(*) as c FROM tool_calls GROUP BY tool ORDER BY c DESC LIMIT 5`); err == nil {
			for rows.Next() {
				var tool string
				var c int
				if err := rows.Scan(&tool, &c); err == nil {
					fmt.Fprintf(&out, "    %s: %d calls\n", tool, c)
				}
			}
			rows.Close()
		}
		out.WriteString("  Most used actions:\n")
		if rows, err := d.Query(`SELECT tool, action, COUNT(*) as c FROM tool_calls WHERE action IS NOT NULL GROUP BY tool, action ORDER BY c DESC LIMIT 20`); err == nil {
			for rows.Next() {
				var tool, action string
				var c int
				if err := rows.Scan(&tool, &action, &c); err == nil {
					fmt.Fprintf(&out, "    %s.%s: %d calls\n", tool, action, c)
				}
			}
			rows.Close()
		}
	}
	if failures > 0 {
		out.WriteString("  Top failures:\n")
		if rows, err := d.Query(`SELECT tool, action, error_message, COUNT(*) as c FROM tool_calls WHERE success = 0 AND error_message IS NOT NULL GROUP BY tool, action, error_message ORDER BY c DESC LIMIT 5`); err == nil {
			for rows.Next() {
				var tool string
				var action sql.NullString
				var errMsg string
				var c int
				if err := rows.Scan(&tool, &action, &errMsg, &c); err == nil {
					label := tool
					if action.Valid && action.String != "" {
						label = tool + "." + action.String
					}
					fmt.Fprintf(&out, "    %s: \"%s\" (%dx)\n", label, errMsg, c)
				}
			}
			rows.Close()
		}
	} else {
		out.WriteString("  Top failures: N/A")
	}
	return out.String()
}

func formatRate(r float64) string {
	return strconv.FormatFloat(r, 'f', 1, 64)
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	return err
}
