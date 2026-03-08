package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/process"
	"github.com/chabinhwang/octunnel/internal/recovery"
	"github.com/chabinhwang/octunnel/internal/util"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "octunnel",
	Short: "Expose OpenCode server via Cloudflare Tunnel",
	Long: `octunnel is a CLI tool that starts an OpenCode server (opencode serve)
and exposes it to the internet through a Cloudflare Tunnel.

Running 'octunnel' without a subcommand starts a Quick Tunnel (no login required).
Use 'octunnel login' + 'octunnel auth' + 'octunnel run' for a fixed domain.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runQuickTunnel,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		util.LogError(util.TagOctunnel, "%v", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(switchCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(removeCmd)
}

// ---------- octunnel (quick tunnel) ----------

func runQuickTunnel(cmd *cobra.Command, args []string) error {
	// 1. Preflight
	util.Log(util.TagPreflight, "checking dependencies...")
	if err := util.CheckDependencies(); err != nil {
		return err
	}
	util.LogSuccess(util.TagPreflight, "all dependencies found")

	// 2. Lock
	lock, err := util.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()

	// 3. Load config
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 4. Recovery check
	rec := recovery.CheckRecovery(cfg, "quick")
	if rec.Message != "" {
		util.Log(util.TagRecover, "%s", rec.Message)
	}

	// 5. Context + signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	pm := process.NewProcessManager(cfg)

	// Ensure cleanup on exit
	defer func() {
		util.Log(util.TagOctunnel, "shutting down...")
		pm.Cleanup()
		cfg.SetInterrupted()
	}()

	// Handle signal in background
	go func() {
		<-sigCh
		util.Log(util.TagOctunnel, "received interrupt signal")
		cancel()
	}()

	// 6. Handle recovery actions
	if rec.Action == "reuse_processes" {
		util.LogSuccess(util.TagOctunnel, "reusing existing session")
		displayPublicURL(rec.PublicURL)
		return pm.Wait(ctx)
	}

	// 7. Start operation
	if err := cfg.StartOperation("quick", "quick"); err != nil {
		return err
	}

	// 8. Start opencode serve (or reuse)
	var localURL string
	if rec.ReuseOpencode && rec.LocalURL != "" {
		localURL = rec.LocalURL
		util.LogSuccess(util.TagOpencode, "reusing existing: %s", localURL)
	} else {
		util.Log(util.TagOpencode, "starting opencode serve...")
		localURL, err = pm.StartOpencode(ctx)
		if err != nil {
			cfg.SetFailed(fmt.Sprintf("opencode start failed: %v", err))
			return fmt.Errorf("failed to start opencode: %w", err)
		}
	}

	cfg.LocalURL = localURL
	if err := cfg.UpdatePhase("local_url_detected"); err != nil {
		return err
	}
	util.LogSuccess(util.TagOpencode, "listening on %s", localURL)

	// 9. Start quick tunnel
	util.Log(util.TagCloudflared, "starting quick tunnel...")
	if err := cfg.UpdatePhase("quick_tunnel_started"); err != nil {
		return err
	}

	publicURL, err := pm.StartCloudflaredQuick(ctx, localURL)
	if err != nil {
		cfg.SetFailed(fmt.Sprintf("quick tunnel failed: %v", err))
		return fmt.Errorf("quick tunnel failed: %w", err)
	}

	cfg.PublicURL = publicURL
	if err := cfg.UpdatePhase("quick_public_url_detected"); err != nil {
		return err
	}

	// 10. Display results
	displayPublicURL(publicURL)

	// 11. Wait for processes
	if err := cfg.SetCompleted(); err != nil {
		return err
	}
	return pm.Wait(ctx)
}

// displayPublicURL shows the URL, copies to clipboard, and prints QR.
func displayPublicURL(url string) {
	fmt.Println()
	util.LogSuccess(util.TagOctunnel, "Public URL: %s", url)
	fmt.Println()

	if err := util.CopyToClipboard(url); err != nil {
		util.LogWarn(util.TagOctunnel, "clipboard copy failed: %v", err)
	} else {
		util.LogSuccess(util.TagOctunnel, "URL copied to clipboard")
	}

	fmt.Println()
	util.PrintQR(url)
	fmt.Println()
}
