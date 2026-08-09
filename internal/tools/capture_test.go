package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetZenCapCache() {
	zenCapAddrCache = ""
	zenCapAddrLoadedAt = time.Time{}
}

func writeZenCapConfig(t *testing.T, addr string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "zen-cap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"api_address": addr})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetZenCapAPIAddressReadsConfig(t *testing.T) {
	resetZenCapCache()
	writeZenCapConfig(t, "192.168.1.50:5555")
	if got := getZenCapAPIAddress(); got != "192.168.1.50:5555" {
		t.Errorf("got %q, want configured address", got)
	}
}

func TestGetZenCapAPIAddressDefault(t *testing.T) {
	resetZenCapCache()
	t.Setenv("HOME", t.TempDir())
	if got := getZenCapAPIAddress(); got != "localhost:4444" {
		t.Errorf("got %q, want localhost:4444 default", got)
	}
}

// TestGetZenCapAPIAddressCaches pins F12: the address must not be re-read from
// disk on every capture call, even if the config file changes mid-TTL.
func TestGetZenCapAPIAddressCaches(t *testing.T) {
	resetZenCapCache()
	writeZenCapConfig(t, "127.0.0.1:1111")
	first := getZenCapAPIAddress()
	writeZenCapConfig(t, "127.0.0.1:2222")
	second := getZenCapAPIAddress()
	if first != "127.0.0.1:1111" || second != "127.0.0.1:1111" {
		t.Errorf("expected cached address, got first=%q second=%q", first, second)
	}
}

func TestGetZenCapAPIAddressRefreshesAfterTTL(t *testing.T) {
	resetZenCapCache()
	old := zenCapAddrCacheTTL
	zenCapAddrCacheTTL = -time.Second
	defer func() { zenCapAddrCacheTTL = old }()

	writeZenCapConfig(t, "a:1")
	getZenCapAPIAddress()
	writeZenCapConfig(t, "b:2")
	if got := getZenCapAPIAddress(); got != "b:2" {
		t.Errorf("expected refreshed address after TTL, got %q", got)
	}
}
