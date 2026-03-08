package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/process"
	"github.com/chabinhwang/octunnel/internal/recovery"
	"github.com/chabinhwang/octunnel/internal/util"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch configuration",
}

var switchDomainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Switch to a different Cloudflare domain",
	Long: `Re-login to Cloudflare with a new domain, then route DNS for the
existing tunnel to the new domain. The old cert.pem is backed up during the process.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runSwitchDomain,
}

func init() {
	switchCmd.AddCommand(switchDomainCmd)
}

func runSwitchDomain(cmd *cobra.Command, args []string) error {
	lock, err := util.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.HasTunnel() {
		return fmt.Errorf("no tunnel configured. Run 'octunnel login' + 'octunnel auth' first")
	}

	rec := recovery.CheckRecovery(cfg, "switch-domain")
	if rec.Message != "" {
		util.Log(util.TagRecover, "%s", rec.Message)
	}

	if err := cfg.StartOperation("switch-domain", "named"); err != nil {
		return err
	}

	ctx := context.Background()

	// ---------- Phase: backup old cert ----------
	oldCertPath := cfg.CertPemPath
	certBackup := ""

	if rec.Action != "resume" {
		if oldCertPath != "" {
			if _, err := os.Stat(oldCertPath); err == nil {
				certBackup = oldCertPath + ".switch-bak"
				data, readErr := os.ReadFile(oldCertPath)
				if readErr != nil {
					cfg.SetFailed("failed to read old cert.pem")
					return fmt.Errorf("failed to read old cert: %w", readErr)
				}
				if writeErr := os.WriteFile(certBackup, data, 0644); writeErr != nil {
					cfg.SetFailed("failed to backup old cert.pem")
					return fmt.Errorf("failed to backup old cert: %w", writeErr)
				}
				cfg.ConfigBackupPath = certBackup
				_ = cfg.Save()
				util.Log(util.TagOctunnel, "old cert backed up to %s", certBackup)
			}
		}

		if err := cfg.UpdatePhase("cert_backed_up"); err != nil {
			return err
		}

		// ---------- Phase: re-login ----------
		util.Log(util.TagCloudflared, "running cloudflared tunnel login...")
		util.Log(util.TagOctunnel, "a browser window will open — select the NEW domain")

		lines, _ := process.RunCommandParsing(ctx, util.TagCloudflared, "cloudflared", "tunnel", "login")

		newCertPath := ""
		for _, line := range lines {
			if m := process.CertPemRe.FindStringSubmatch(line); m != nil {
				newCertPath = m[1]
			}
		}

		if newCertPath == "" {
			home, _ := os.UserHomeDir()
			defaultCert := home + "/.cloudflared/cert.pem"
			if _, err := os.Stat(defaultCert); err == nil {
				newCertPath = defaultCert
			}
		}

		if newCertPath == "" {
			// Restore old cert
			if certBackup != "" {
				restoreData, _ := os.ReadFile(certBackup)
				if restoreData != nil && oldCertPath != "" {
					os.WriteFile(oldCertPath, restoreData, 0644)
					util.Log(util.TagRecover, "restored old cert.pem from backup")
				}
			}
			cfg.SetFailed("new login failed, old cert restored")
			return fmt.Errorf("login failed: could not find new cert.pem")
		}

		cfg.CertPemPath = newCertPath
		if err := cfg.UpdatePhase("new_cert_detected"); err != nil {
			return err
		}
		util.LogSuccess(util.TagCloudflared, "new cert.pem: %s", newCertPath)
	}

	// ---------- Phase: new domain ----------
	if cfg.BaseDomain == "" || rec.ResumePhase == "new_cert_detected" || rec.ResumePhase == "cert_backed_up" {
		domain := promptInput("Enter your new base domain (e.g., example.com): ")
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimRight(domain, "/")
		domain = strings.ToLower(domain)

		if err := validateDomain(domain); err != nil {
			cfg.SetFailed(err.Error())
			return err
		}

		fmt.Printf("\n  New base domain: %s\n\n", domain)
		if !promptYN("Is this correct? (Y/n): ", true) {
			cfg.SetFailed("domain not confirmed")
			return fmt.Errorf("domain not confirmed")
		}

		cfg.BaseDomain = domain
		if err := cfg.UpdatePhase("base_domain_saved"); err != nil {
			return err
		}
	}

	// ---------- Phase: DNS route ----------
	subdomain := strings.ToLower(promptInput(fmt.Sprintf("Enter subdomain prefix (e.g., 'open' → open.%s): ", cfg.BaseDomain)))
	if err := validateSubdomain(subdomain); err != nil {
		cfg.SetFailed(err.Error())
		return err
	}

	hostname := subdomain + "." + cfg.BaseDomain

	fmt.Println()
	util.LogWarn(util.TagOctunnel, "this will route DNS for: %s", hostname)
	util.LogWarn(util.TagOctunnel, "existing CNAME records may be overwritten")
	fmt.Println()

	if !promptYN("Continue? (y/N): ", false) {
		cfg.SetFailed("DNS route not confirmed")
		return fmt.Errorf("DNS route canceled")
	}

	util.Log(util.TagCloudflared, "routing DNS: %s → %s", hostname, cfg.TunnelName)

	lines, runErr := process.RunCommandParsing(ctx, util.TagCloudflared,
		"cloudflared", "tunnel", "--config", "", "route", "dns", cfg.TunnelName, hostname)

	success := false
	if runErr == nil {
		success = true
	} else {
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "added cname") || strings.Contains(lower, "already exists") {
				success = true
				break
			}
		}
	}

	if !success {
		cfg.SetFailed(fmt.Sprintf("DNS route failed: %v", runErr))
		return fmt.Errorf("failed to route DNS for %s: %v", hostname, runErr)
	}

	cfg.Hostname = hostname
	if err := cfg.UpdatePhase("hostname_saved"); err != nil {
		return err
	}

	// ---------- Cleanup old backup ----------
	if certBackup != "" {
		os.Remove(certBackup)
		cfg.ConfigBackupPath = ""
	}

	util.LogSuccess(util.TagOctunnel, "domain switched successfully!")
	util.LogSuccess(util.TagOctunnel, "new hostname: %s", hostname)
	util.Log(util.TagOctunnel, "run 'octunnel run' to start the tunnel with the new domain")

	return cfg.SetCompleted()
}
