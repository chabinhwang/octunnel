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

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Create a Named Tunnel and connect DNS",
	Long: `Creates a Named Tunnel via 'cloudflared tunnel create' and routes a
DNS hostname to it. Requires 'octunnel login' first.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runAuth,
}

func runAuth(cmd *cobra.Command, args []string) error {
	lock, err := util.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run 'octunnel login' first")
	}

	rec := recovery.CheckRecovery(cfg, "auth")
	if rec.Message != "" {
		util.Log(util.TagRecover, "%s", rec.Message)
	}

	if err := cfg.StartOperation("auth", "named"); err != nil {
		return err
	}

	ctx := context.Background()

	// ---------- Phase: tunnel creation ----------
	if !cfg.HasTunnel() {
		util.Log(util.TagCloudflared, "creating Named Tunnel...")

		tunnelName, tunnelID, credFile, err := createTunnelWithRetry(ctx, cfg)
		if err != nil {
			cfg.SetFailed(fmt.Sprintf("tunnel creation failed: %v", err))
			return err
		}

		cfg.TunnelName = tunnelName
		cfg.TunnelID = tunnelID
		cfg.CredentialsFilePath = credFile

		if err := cfg.UpdatePhase("tunnel_saved"); err != nil {
			return err
		}

		util.LogSuccess(util.TagCloudflared, "tunnel created: %s (id: %s)", tunnelName, tunnelID)
	} else {
		util.LogSuccess(util.TagCloudflared, "tunnel already exists: %s (id: %s)", cfg.TunnelName, cfg.TunnelID)
	}

	// ---------- Phase: DNS route ----------
	if cfg.HasHostname() {
		util.Log(util.TagOctunnel, "current hostname: %s", cfg.Hostname)
		util.Log(util.TagOctunnel, "re-running auth will change the subdomain for this tunnel")
		fmt.Println()
	}

	subdomain := strings.ToLower(promptInput(fmt.Sprintf("Enter subdomain prefix (e.g., 'open' → open.%s): ", cfg.BaseDomain)))
	if err := validateSubdomain(subdomain); err != nil {
		cfg.SetFailed(err.Error())
		return err
	}

	hostname := subdomain + "." + cfg.BaseDomain

	fmt.Println()
	util.LogWarn(util.TagOctunnel, "this subdomain may already have a DNS record.")
	util.LogWarn(util.TagOctunnel, "continuing will overwrite any existing CNAME for: %s", hostname)
	fmt.Println()

	if !promptYN("Continue? (y/N): ", false) {
		cfg.SetFailed("DNS route not confirmed")
		return fmt.Errorf("DNS route canceled by user")
	}

	util.Log(util.TagCloudflared, "routing DNS: %s → %s", hostname, cfg.TunnelName)

	lines, runErr := process.RunCommandParsing(ctx, util.TagCloudflared,
		"cloudflared", "tunnel", "--config", "", "route", "dns", cfg.TunnelName, hostname)

	// Check for success or already-exists
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
		return fmt.Errorf("failed to route DNS for %s: %w", hostname, runErr)
	}

	cfg.Hostname = hostname
	if err := cfg.UpdatePhase("hostname_saved"); err != nil {
		return err
	}
	util.LogSuccess(util.TagCloudflared, "DNS routed: %s", hostname)

	return cfg.SetCompleted()
}

// createTunnelWithRetry tries tunnel names: octunnel, octunnel1, octunnel2, ...
func createTunnelWithRetry(ctx context.Context, cfg *config.Config) (name, id, credFile string, err error) {
	baseName := "octunnel"

	for i := 0; i < 20; i++ {
		tryName := baseName
		if i > 0 {
			tryName = fmt.Sprintf("%s%d", baseName, i)
		}

		util.Log(util.TagCloudflared, "trying tunnel name: %s", tryName)

		lines, runErr := process.RunCommandParsing(ctx, util.TagCloudflared,
			"cloudflared", "tunnel", "--config", "", "create", tryName)

		// Check for name-already-exists error
		exists := false
		for _, line := range lines {
			if process.TunnelExistsRe.MatchString(line) {
				exists = true
				break
			}
		}

		if exists {
			util.LogWarn(util.TagCloudflared, "tunnel name '%s' already exists, trying next...", tryName)
			continue
		}

		if runErr != nil {
			return "", "", "", fmt.Errorf("tunnel create failed: %w", runErr)
		}

		// Parse tunnel id and credentials file from output
		for _, line := range lines {
			if m := process.TunnelCreatedRe.FindStringSubmatch(line); m != nil {
				name = m[1]
				id = m[2]
			}
			if m := process.TunnelCredFileRe.FindStringSubmatch(line); m != nil {
				credFile = m[1]
			}
		}

		if id == "" {
			// Try to recover: look for credentials files
			home, _ := os.UserHomeDir()
			entries, _ := os.ReadDir(home + "/.cloudflared")
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".json") && e.Name() != "cert.pem" && len(e.Name()) > 10 {
					candidate := home + "/.cloudflared/" + e.Name()
					potentialID := strings.TrimSuffix(e.Name(), ".json")
					if len(potentialID) == 36 { // UUID length
						credFile = candidate
						id = potentialID
						name = tryName
						break
					}
				}
			}
		}

		if id == "" || name == "" {
			return "", "", "", fmt.Errorf("could not parse tunnel id from output")
		}

		return name, id, credFile, nil
	}

	return "", "", "", fmt.Errorf("all tunnel names octunnel..octunnel19 are taken")
}
