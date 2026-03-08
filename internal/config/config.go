package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chabinhwang/octunnel/internal/util"
)

const (
	StatusIdle        = "idle"
	StatusInProgress  = "in_progress"
	StatusInterrupted = "interrupted"
	StatusFailed      = "failed"
	StatusCompleted   = "completed"
)

// Config is the single persistent state store for octunnel.
type Config struct {
	CertPemPath string `json:"certPemPath,omitempty"`
	BaseDomain  string `json:"baseDomain,omitempty"`

	TunnelID            string `json:"tunnelId,omitempty"`
	TunnelName          string `json:"tunnelName,omitempty"`
	CredentialsFilePath string `json:"credentialsFilePath,omitempty"`
	Hostname            string `json:"hostname,omitempty"`

	LastCommand         string `json:"lastCommand,omitempty"`
	OperationStatus     string `json:"operationStatus,omitempty"`
	CurrentPhase        string `json:"currentPhase,omitempty"`
	StartedAt           string `json:"startedAt,omitempty"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	LastError           string `json:"lastError,omitempty"`
	OpencodePID         int    `json:"opencodePid,omitempty"`
	CloudflaredPID      int    `json:"cloudflaredPid,omitempty"`
	LocalURL            string `json:"localUrl,omitempty"`
	PublicURL           string `json:"publicUrl,omitempty"`
	Mode                string `json:"mode,omitempty"`
	ConfigBackupPath    string `json:"configBackupPath,omitempty"`
	LastSuccessfulPhase string `json:"lastSuccessfulPhase,omitempty"`
}

// Dir returns ~/.octunnel.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".octunnel")
}

func configPath() string {
	return filepath.Join(Dir(), "config.json")
}

// Load reads the state file. Returns an empty Config if the file does not exist.
func Load() (*Config, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	if len(data) == 0 {
		return &Config{}, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		backupPath := path + ".bak"
		if bd, be := os.ReadFile(backupPath); be == nil {
			if json.Unmarshal(bd, &cfg) == nil {
				util.Log(util.TagRecover, "config.json was corrupted, restored from backup")
				_ = cfg.Save()
				return &cfg, nil
			}
		}
		return nil, fmt.Errorf("failed to parse config (and no valid backup): %w", err)
	}
	return &cfg, nil
}

// Save writes Config atomically: tmp → rename, with .bak of previous.
func (c *Config) Save() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path := configPath()
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	if cur, re := os.ReadFile(path); re == nil {
		_ = os.WriteFile(path+".bak", cur, 0644)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to save config (atomic rename): %w", err)
	}
	return nil
}

// ---------- Operation lifecycle ----------

func (c *Config) StartOperation(command, mode string) error {
	c.LastCommand = command
	c.OperationStatus = StatusInProgress
	c.CurrentPhase = ""
	c.Mode = mode
	c.StartedAt = time.Now().UTC().Format(time.RFC3339)
	c.LastError = ""
	return c.Save()
}

func (c *Config) UpdatePhase(phase string) error {
	c.CurrentPhase = phase
	c.LastSuccessfulPhase = phase
	c.OperationStatus = StatusInProgress
	return c.Save()
}

func (c *Config) SetCompleted() error {
	c.OperationStatus = StatusCompleted
	c.LastError = ""
	return c.Save()
}

func (c *Config) SetFailed(errMsg string) error {
	c.OperationStatus = StatusFailed
	c.LastError = errMsg
	return c.Save()
}

func (c *Config) SetInterrupted() error {
	c.OperationStatus = StatusInterrupted
	return c.Save()
}

func (c *Config) ClearRuntime() {
	c.LastCommand = ""
	c.OperationStatus = StatusIdle
	c.CurrentPhase = ""
	c.StartedAt = ""
	c.LastError = ""
	c.OpencodePID = 0
	c.CloudflaredPID = 0
	c.LocalURL = ""
	c.PublicURL = ""
	c.Mode = ""
	c.LastSuccessfulPhase = ""
}

// ---------- State queries ----------

func (c *Config) IsLoggedIn() bool {
	if c.CertPemPath == "" || c.BaseDomain == "" {
		return false
	}
	_, err := os.Stat(c.CertPemPath)
	return err == nil
}

func (c *Config) HasTunnel() bool  { return c.TunnelID != "" && c.TunnelName != "" }
func (c *Config) HasHostname() bool { return c.Hostname != "" }

func (c *Config) IsNamedReady() bool {
	return c.IsLoggedIn() && c.HasTunnel() && c.HasHostname()
}
