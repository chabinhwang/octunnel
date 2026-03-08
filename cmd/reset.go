package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/process"
	"github.com/chabinhwang/octunnel/internal/util"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset all octunnel configuration and start fresh",
	Long: `Deletes all octunnel state, the Cloudflare tunnel, and local config files.
CNAME DNS records are NOT deleted — remove them manually from the Cloudflare dashboard.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runReset,
}

func runReset(cmd *cobra.Command, args []string) error {
	cfg, _ := config.Load()

	fmt.Println()
	util.LogWarn(util.TagOctunnel, "This will DELETE all octunnel configuration:")
	fmt.Println()
	fmt.Println("  - ~/.octunnel/config.json (login, tunnel, hostname)")
	fmt.Println("  - ~/.octunnel/cloudflared.yml (tunnel config)")
	fmt.Println("  - ~/.octunnel/octunnel.lock")
	if cfg != nil && cfg.TunnelName != "" {
		fmt.Printf("  - Cloudflare tunnel '%s' (id: %s)\n", cfg.TunnelName, cfg.TunnelID)
	}
	fmt.Println()

	if cfg != nil && cfg.Hostname != "" {
		util.LogWarn(util.TagOctunnel, "DNS CNAME record for '%s' will NOT be deleted automatically.", cfg.Hostname)
		util.LogWarn(util.TagOctunnel, "You must manually remove it from the Cloudflare dashboard:")
		fmt.Println("  https://dash.cloudflare.com → DNS → Delete CNAME for", cfg.Hostname)
		fmt.Println()
	}

	util.LogWarn(util.TagOctunnel, "This action cannot be undone.")
	input := promptInput("Type 'reset' to confirm: ")
	if input != "reset" {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println()

	// Kill any running processes
	if cfg != nil {
		killOctunnelProcesses(cfg)
	}

	// Delete Cloudflare tunnel
	if cfg != nil && cfg.TunnelName != "" {
		deleteTunnel(cfg.TunnelName)
	}

	// Delete octunnel files
	dir := config.Dir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			path := dir + "/" + e.Name()
			if err := os.Remove(path); err == nil {
				util.Log(util.TagOctunnel, "deleted %s", path)
			}
		}
	}

	fmt.Println()
	util.LogSuccess(util.TagOctunnel, "reset complete")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. octunnel login    — re-login to Cloudflare")
	fmt.Println("  2. octunnel auth     — create tunnel + DNS")
	fmt.Println("  3. octunnel run      — start named tunnel")
	fmt.Println()
	fmt.Println("Or just run 'octunnel' for a quick tunnel (no login needed)")

	return nil
}

// killOctunnelProcesses stops any running opencode/cloudflared from a previous session.
func killOctunnelProcesses(cfg *config.Config) {
	if cfg.OpencodePID > 0 && util.IsProcessAlive(cfg.OpencodePID) {
		util.Log(util.TagOctunnel, "stopping opencode (PID %d)...", cfg.OpencodePID)
		util.KillProcess(cfg.OpencodePID)
	}
	if cfg.CloudflaredPID > 0 && util.IsProcessAlive(cfg.CloudflaredPID) {
		util.Log(util.TagOctunnel, "stopping cloudflared (PID %d)...", cfg.CloudflaredPID)
		util.KillProcess(cfg.CloudflaredPID)
	}
}

// deleteTunnel attempts to delete a Cloudflare tunnel via cloudflared CLI.
func deleteTunnel(tunnelName string) {
	util.Log(util.TagCloudflared, "deleting tunnel '%s'...", tunnelName)

	ctx := context.Background()
	lines, err := process.RunCommandParsing(ctx, util.TagCloudflared,
		"cloudflared", "tunnel", "--config", "", "delete", tunnelName)

	if err != nil {
		// Tunnel might have active connections — cleanup first, then retry
		needsCleanup := false
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "active connections") || strings.Contains(lower, "cleanup") {
				needsCleanup = true
				break
			}
		}

		if needsCleanup {
			util.LogWarn(util.TagCloudflared, "tunnel has active connections, cleaning up first...")
			process.RunCommandParsing(ctx, util.TagCloudflared,
				"cloudflared", "tunnel", "--config", "", "cleanup", tunnelName)
			_, retryErr := process.RunCommandParsing(ctx, util.TagCloudflared,
				"cloudflared", "tunnel", "--config", "", "delete", tunnelName)
			if retryErr == nil {
				util.LogSuccess(util.TagCloudflared, "tunnel '%s' deleted", tunnelName)
				return
			}
		}

		util.LogWarn(util.TagCloudflared, "failed to delete tunnel: %v", err)
		util.LogWarn(util.TagCloudflared, "delete manually: cloudflared tunnel --config \"\" delete %s", tunnelName)
	} else {
		util.LogSuccess(util.TagCloudflared, "tunnel '%s' deleted", tunnelName)
	}
}
