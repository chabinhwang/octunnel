package cmd

import (
	"fmt"
	"os"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/util"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Completely uninstall octunnel data and configuration",
	Long: `Removes the entire ~/.octunnel directory and all its contents.
This is a full uninstall of octunnel's local data. The octunnel binary itself
is not deleted — remove it manually if needed.

Existing Cloudflare resources (tunnels, CNAME records) are NOT deleted.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	dir := config.Dir()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Println("Nothing to remove — ~/.octunnel does not exist.")
		return nil
	}

	cfg, _ := config.Load()

	fmt.Println()
	util.LogWarn(util.TagOctunnel, "This will permanently DELETE the entire ~/.octunnel directory:")
	fmt.Println()

	// List what will be deleted
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		fmt.Printf("  %s/%s\n", dir, e.Name())
	}
	if cfg != nil && cfg.TunnelName != "" {
		fmt.Printf("  Cloudflare tunnel '%s' (id: %s)\n", cfg.TunnelName, cfg.TunnelID)
	}
	fmt.Println()

	if cfg != nil && cfg.Hostname != "" {
		util.LogWarn(util.TagOctunnel, "DNS CNAME for '%s' will NOT be deleted automatically.", cfg.Hostname)
		util.LogWarn(util.TagOctunnel, "Remove it manually: https://dash.cloudflare.com → DNS")
		fmt.Println()
	}

	util.LogWarn(util.TagOctunnel, "This action cannot be undone.")
	input := promptInput("Type 'remove' to confirm: ")
	if input != "remove" {
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

	// Remove entire directory
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove %s: %w", dir, err)
	}

	util.LogSuccess(util.TagOctunnel, "removed %s", dir)
	fmt.Println()
	fmt.Println("octunnel data has been completely removed.")
	fmt.Println("To also remove the binary: rm", os.Args[0])

	return nil
}
