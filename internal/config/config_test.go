package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withTempHome overrides HOME so Load/Save use a temp directory.
func withTempHome(t *testing.T) (dir string, cleanup func()) {
	t.Helper()
	dir = t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	return dir, func() { os.Setenv("HOME", orig) }
}

func TestLoadEmpty(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OperationStatus != "" {
		t.Errorf("expected empty status, got %q", cfg.OperationStatus)
	}
}

func TestSaveAndLoad(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	cfg := &Config{
		BaseDomain: "example.com",
		TunnelID:   "abc-123",
		TunnelName: "octunnel",
		Hostname:   "open.example.com",
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.BaseDomain != "example.com" {
		t.Errorf("BaseDomain: got %q, want %q", loaded.BaseDomain, "example.com")
	}
	if loaded.TunnelID != "abc-123" {
		t.Errorf("TunnelID: got %q, want %q", loaded.TunnelID, "abc-123")
	}
	if loaded.Hostname != "open.example.com" {
		t.Errorf("Hostname: got %q, want %q", loaded.Hostname, "open.example.com")
	}
	if loaded.UpdatedAt == "" {
		t.Error("UpdatedAt should be set after Save()")
	}
}

func TestSaveCreatesBackup(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	cfg := &Config{BaseDomain: "first.com"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("first Save() error: %v", err)
	}

	cfg.BaseDomain = "second.com"
	if err := cfg.Save(); err != nil {
		t.Fatalf("second Save() error: %v", err)
	}

	bakPath := filepath.Join(home, ".octunnel", "config.json.bak")
	data, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}

	var bak Config
	if err := json.Unmarshal(data, &bak); err != nil {
		t.Fatalf("failed to parse backup: %v", err)
	}
	if bak.BaseDomain != "first.com" {
		t.Errorf("backup BaseDomain: got %q, want %q", bak.BaseDomain, "first.com")
	}
}

func TestLoadCorruptedWithBackup(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	dir := filepath.Join(home, ".octunnel")
	os.MkdirAll(dir, 0755)

	// Write valid backup
	bak := Config{BaseDomain: "backup.com"}
	bakData, _ := json.Marshal(bak)
	os.WriteFile(filepath.Join(dir, "config.json.bak"), bakData, 0644)

	// Write corrupted main config
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should recover from backup, got error: %v", err)
	}
	if cfg.BaseDomain != "backup.com" {
		t.Errorf("BaseDomain: got %q, want %q", cfg.BaseDomain, "backup.com")
	}
}

func TestLoadCorruptedNoBackup(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	dir := filepath.Join(home, ".octunnel")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0644)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail with corrupted config and no backup")
	}
}

func TestOperationLifecycle(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	cfg := &Config{}

	// StartOperation
	if err := cfg.StartOperation("auth", "named"); err != nil {
		t.Fatalf("StartOperation error: %v", err)
	}
	if cfg.LastCommand != "auth" {
		t.Errorf("LastCommand: got %q, want %q", cfg.LastCommand, "auth")
	}
	if cfg.OperationStatus != StatusInProgress {
		t.Errorf("status: got %q, want %q", cfg.OperationStatus, StatusInProgress)
	}
	if cfg.StartedAt == "" {
		t.Error("StartedAt should be set")
	}

	// UpdatePhase
	if err := cfg.UpdatePhase("tunnel_saved"); err != nil {
		t.Fatalf("UpdatePhase error: %v", err)
	}
	if cfg.CurrentPhase != "tunnel_saved" {
		t.Errorf("CurrentPhase: got %q, want %q", cfg.CurrentPhase, "tunnel_saved")
	}
	if cfg.LastSuccessfulPhase != "tunnel_saved" {
		t.Errorf("LastSuccessfulPhase: got %q, want %q", cfg.LastSuccessfulPhase, "tunnel_saved")
	}

	// SetCompleted
	if err := cfg.SetCompleted(); err != nil {
		t.Fatalf("SetCompleted error: %v", err)
	}
	if cfg.OperationStatus != StatusCompleted {
		t.Errorf("status: got %q, want %q", cfg.OperationStatus, StatusCompleted)
	}

	// Verify persisted
	loaded, _ := Load()
	if loaded.OperationStatus != StatusCompleted {
		t.Errorf("persisted status: got %q, want %q", loaded.OperationStatus, StatusCompleted)
	}
}

func TestSetFailed(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	cfg := &Config{}
	cfg.StartOperation("login", "")

	if err := cfg.SetFailed("something broke"); err != nil {
		t.Fatalf("SetFailed error: %v", err)
	}
	if cfg.OperationStatus != StatusFailed {
		t.Errorf("status: got %q, want %q", cfg.OperationStatus, StatusFailed)
	}
	if cfg.LastError != "something broke" {
		t.Errorf("LastError: got %q, want %q", cfg.LastError, "something broke")
	}
}

func TestSetInterrupted(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	cfg := &Config{}
	cfg.StartOperation("run", "quick")

	if err := cfg.SetInterrupted(); err != nil {
		t.Fatalf("SetInterrupted error: %v", err)
	}
	if cfg.OperationStatus != StatusInterrupted {
		t.Errorf("status: got %q, want %q", cfg.OperationStatus, StatusInterrupted)
	}
}

func TestClearRuntime(t *testing.T) {
	cfg := &Config{
		LastCommand:         "run",
		OperationStatus:     StatusInProgress,
		CurrentPhase:        "quick_tunnel_started",
		StartedAt:           "2025-01-01T00:00:00Z",
		LastError:           "some error",
		OpencodePID:         1234,
		CloudflaredPID:      5678,
		LocalURL:            "http://127.0.0.1:3000",
		PublicURL:           "https://random.trycloudflare.com",
		Mode:                "quick",
		LastSuccessfulPhase: "opencode_started",
		// These should survive ClearRuntime
		BaseDomain: "example.com",
		TunnelID:   "abc-123",
	}

	cfg.ClearRuntime()

	if cfg.OperationStatus != StatusIdle {
		t.Errorf("status: got %q, want %q", cfg.OperationStatus, StatusIdle)
	}
	if cfg.OpencodePID != 0 {
		t.Errorf("OpencodePID should be 0, got %d", cfg.OpencodePID)
	}
	if cfg.PublicURL != "" {
		t.Errorf("PublicURL should be empty, got %q", cfg.PublicURL)
	}
	// Persistent fields should survive
	if cfg.BaseDomain != "example.com" {
		t.Errorf("BaseDomain should survive ClearRuntime, got %q", cfg.BaseDomain)
	}
	if cfg.TunnelID != "abc-123" {
		t.Errorf("TunnelID should survive ClearRuntime, got %q", cfg.TunnelID)
	}
}

func TestStateQueries(t *testing.T) {
	cfg := &Config{}

	if cfg.HasTunnel() {
		t.Error("empty config should not HasTunnel")
	}
	if cfg.HasHostname() {
		t.Error("empty config should not HasHostname")
	}

	cfg.TunnelID = "abc"
	if cfg.HasTunnel() {
		t.Error("TunnelID alone should not satisfy HasTunnel")
	}

	cfg.TunnelName = "octunnel"
	if !cfg.HasTunnel() {
		t.Error("TunnelID + TunnelName should satisfy HasTunnel")
	}

	cfg.Hostname = "open.example.com"
	if !cfg.HasHostname() {
		t.Error("should HasHostname when set")
	}
}
