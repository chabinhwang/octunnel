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
	"github.com/chabinhwang/octunnel/internal/tunnel"
	"github.com/chabinhwang/octunnel/internal/util"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Named Tunnel with a fixed domain",
	Long: `Starts opencode serve, writes ~/.octunnel/cloudflared.yml, and runs
'cloudflared tunnel run --config ~/.octunnel/cloudflared.yml' for the configured Named Tunnel.
Requires 'octunnel login' and 'octunnel auth' first.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runNamedTunnel,
}

func runNamedTunnel(cmd *cobra.Command, args []string) error {
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

	// 3. Load config + validate
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsNamedReady() {
		missing := []string{}
		if !cfg.IsLoggedIn() {
			missing = append(missing, "login (run 'octunnel login')")
		}
		if !cfg.HasTunnel() {
			missing = append(missing, "tunnel (run 'octunnel auth')")
		}
		if !cfg.HasHostname() {
			missing = append(missing, "hostname (run 'octunnel auth')")
		}
		return fmt.Errorf("named tunnel not ready. Missing: %v", missing)
	}

	// 4. Recovery
	rec := recovery.CheckRecovery(cfg, "run")
	if rec.Message != "" {
		util.Log(util.TagRecover, "%s", rec.Message)
	}

	// 5. Context + signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	pm := process.NewProcessManager(cfg)

	defer func() {
		util.Log(util.TagOctunnel, "shutting down...")
		pm.Cleanup()
		cfg.SetInterrupted()
	}()

	go func() {
		<-sigCh
		util.Log(util.TagOctunnel, "received interrupt signal")
		cancel()
	}()

	// 6. Reuse check
	if rec.Action == "reuse_processes" {
		util.LogSuccess(util.TagOctunnel, "reusing existing named tunnel session")
		publicURL := "https://" + cfg.Hostname
		displayPublicURL(publicURL)
		return pm.Wait(ctx)
	}

	// 7. Start operation
	if err := cfg.StartOperation("run", "named"); err != nil {
		return err
	}

	// 8. Start opencode serve
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

	// 9. Write octunnel-owned cloudflared config (~/.octunnel/cloudflared.yml)
	util.Log(util.TagCloudflared, "writing cloudflared config...")

	err = tunnel.WriteCloudflaredConfig(cfg.TunnelID, cfg.CredentialsFilePath, cfg.Hostname, localURL)
	if err != nil {
		cfg.SetFailed(fmt.Sprintf("config write failed: %v", err))
		return fmt.Errorf("failed to write cloudflared config: %w", err)
	}

	cfgPath := tunnel.OctunnelConfigPath()
	if err := cfg.UpdatePhase("cloudflared_config_written"); err != nil {
		return err
	}
	util.LogSuccess(util.TagCloudflared, "config written: %s", cfgPath)

	// 10. Start named tunnel
	util.Log(util.TagCloudflared, "starting named tunnel: %s", cfg.TunnelName)

	if err := pm.StartCloudflaredNamed(ctx, cfg.TunnelName, cfgPath); err != nil {
		cfg.SetFailed(fmt.Sprintf("named tunnel start failed: %v", err))
		return fmt.Errorf("failed to start named tunnel: %w", err)
	}

	if err := cfg.UpdatePhase("named_tunnel_started"); err != nil {
		return err
	}

	// 11. Display results (hostname is fixed)
	publicURL := "https://" + cfg.Hostname
	cfg.PublicURL = publicURL
	_ = cfg.Save()

	displayPublicURL(publicURL)

	// 12. Wait
	if err := cfg.SetCompleted(); err != nil {
		return err
	}
	return pm.Wait(ctx)
}
