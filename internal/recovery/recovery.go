package recovery

import (
	"fmt"
	"os"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/process"
	"github.com/chabinhwang/octunnel/internal/util"
)

// RecoveryAction describes what should happen when a previous operation was
// interrupted or left partial state.
type RecoveryAction struct {
	Action      string // "fresh_start", "resume", "reuse_processes"
	Message     string
	ResumePhase string

	ReuseOpencode    bool
	ReuseCloudflared bool
	LocalURL         string
	PublicURL        string
}

// CheckRecovery inspects the current Config and decides whether the given
// command should start fresh, resume, or reuse running processes.
func CheckRecovery(cfg *config.Config, command string) *RecoveryAction {
	if cfg.OperationStatus == "" || cfg.OperationStatus == config.StatusIdle || cfg.OperationStatus == config.StatusCompleted {
		return &RecoveryAction{Action: "fresh_start"}
	}

	if cfg.LastCommand != command {
		cfg.ClearRuntime()
		_ = cfg.Save()
		return &RecoveryAction{
			Action:  "fresh_start",
			Message: fmt.Sprintf("Previous '%s' operation state cleared", cfg.LastCommand),
		}
	}

	switch command {
	case "quick":
		return checkQuickRecovery(cfg)
	case "login":
		return checkLoginRecovery(cfg)
	case "auth":
		return checkAuthRecovery(cfg)
	case "run":
		return checkRunRecovery(cfg)
	case "switch-domain":
		return checkSwitchRecovery(cfg)
	default:
		return &RecoveryAction{Action: "fresh_start"}
	}
}

func checkQuickRecovery(cfg *config.Config) *RecoveryAction {
	ocAlive := cfg.OpencodePID > 0 && util.IsProcessWithName(cfg.OpencodePID, "opencode")
	cfAlive := cfg.CloudflaredPID > 0 && util.IsProcessWithName(cfg.CloudflaredPID, "cloudflared")

	if ocAlive && cfAlive {
		return &RecoveryAction{
			Action:           "reuse_processes",
			Message:          "Previous session is still running",
			ReuseOpencode:    true,
			ReuseCloudflared: true,
			LocalURL:         cfg.LocalURL,
			PublicURL:        cfg.PublicURL,
		}
	}

	if ocAlive && !cfAlive {
		url, err := process.DetectOpencodePort()
		if err != nil {
			util.KillProcess(cfg.OpencodePID)
			return &RecoveryAction{Action: "fresh_start", Message: "opencode running but port detection failed, restarting"}
		}
		return &RecoveryAction{
			Action:        "resume",
			Message:       "opencode is still running, restarting cloudflared only",
			ResumePhase:   "local_url_detected",
			ReuseOpencode: true,
			LocalURL:      url,
		}
	}

	if !ocAlive && cfAlive {
		util.Log(util.TagRecover, "cleaning up orphaned cloudflared (PID %d)", cfg.CloudflaredPID)
		util.KillProcess(cfg.CloudflaredPID)
		return &RecoveryAction{Action: "fresh_start", Message: "cleaned up orphaned cloudflared process"}
	}

	return &RecoveryAction{Action: "fresh_start", Message: "previous session ended, starting fresh"}
}

func checkLoginRecovery(cfg *config.Config) *RecoveryAction {
	certOK := false
	if cfg.CertPemPath != "" {
		if _, err := os.Stat(cfg.CertPemPath); err == nil {
			certOK = true
		}
	}

	if certOK && cfg.BaseDomain != "" {
		return &RecoveryAction{Action: "fresh_start", Message: "login is already complete"}
	}

	if cfg.CertPemPath != "" {
		util.Log(util.TagRecover, "login state is ambiguous — removing existing cert.pem and restarting login")
		os.Remove(cfg.CertPemPath)
		cfg.CertPemPath = ""
	}
	home, _ := os.UserHomeDir()
	defaultCert := home + "/.cloudflared/cert.pem"
	if _, err := os.Stat(defaultCert); err == nil {
		util.Log(util.TagRecover, "removing default cert.pem at %s", defaultCert)
		os.Remove(defaultCert)
	}
	cfg.BaseDomain = ""
	_ = cfg.Save()

	return &RecoveryAction{Action: "fresh_start", Message: "login state unclear — performing fresh login"}
}

func checkAuthRecovery(cfg *config.Config) *RecoveryAction {
	if cfg.HasTunnel() {
		if cfg.HasHostname() {
			return &RecoveryAction{Action: "fresh_start", Message: "auth is already complete"}
		}
		return &RecoveryAction{
			Action:      "resume",
			Message:     "tunnel already created, resuming from DNS route",
			ResumePhase: "tunnel_saved",
		}
	}
	return &RecoveryAction{Action: "fresh_start"}
}

func checkRunRecovery(cfg *config.Config) *RecoveryAction {
	ocAlive := cfg.OpencodePID > 0 && util.IsProcessWithName(cfg.OpencodePID, "opencode")
	cfAlive := cfg.CloudflaredPID > 0 && util.IsProcessWithName(cfg.CloudflaredPID, "cloudflared")

	if ocAlive && cfAlive {
		return &RecoveryAction{
			Action:           "reuse_processes",
			Message:          "Previous named tunnel session is still running",
			ReuseOpencode:    true,
			ReuseCloudflared: true,
			LocalURL:         cfg.LocalURL,
			PublicURL:        cfg.PublicURL,
		}
	}

	if ocAlive && !cfAlive {
		url, err := process.DetectOpencodePort()
		if err != nil {
			util.KillProcess(cfg.OpencodePID)
			return &RecoveryAction{Action: "fresh_start", Message: "opencode running but port detection failed, restarting"}
		}
		return &RecoveryAction{
			Action:        "resume",
			Message:       "opencode still running, restarting cloudflared tunnel run",
			ResumePhase:   "cloudflared_config_written",
			ReuseOpencode: true,
			LocalURL:      url,
		}
	}

	if !ocAlive && cfAlive {
		util.Log(util.TagRecover, "cleaning up orphaned cloudflared (PID %d)", cfg.CloudflaredPID)
		util.KillProcess(cfg.CloudflaredPID)
	}

	return &RecoveryAction{Action: "fresh_start", Message: "previous run session ended, starting fresh"}
}

func checkSwitchRecovery(cfg *config.Config) *RecoveryAction {
	if cfg.ConfigBackupPath != "" {
		if _, err := os.Stat(cfg.ConfigBackupPath); err == nil {
			if cfg.CertPemPath != "" {
				if _, err := os.Stat(cfg.CertPemPath); err == nil {
					if cfg.BaseDomain == "" {
						return &RecoveryAction{
							Action:      "resume",
							Message:     "switch in progress: new login done, resuming from domain input",
							ResumePhase: "new_cert_detected",
						}
					}
					if !cfg.HasHostname() {
						return &RecoveryAction{
							Action:      "resume",
							Message:     "switch in progress: domain set, resuming from DNS route",
							ResumePhase: "base_domain_saved",
						}
					}
				}
			}
		}
	}
	return &RecoveryAction{Action: "fresh_start"}
}
