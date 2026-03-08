package cmd

import (
	"bufio"
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

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Cloudflare and set default domain",
	Long: `Runs 'cloudflared tunnel login' (opens a browser), then asks for a
base domain to use for Named Tunnels. Results are saved to ~/.octunnel/config.json.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	lock, err := util.AcquireLock()
	if err != nil {
		return err
	}
	defer lock.Release()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	rec := recovery.CheckRecovery(cfg, "login")
	if rec.Message != "" {
		util.Log(util.TagRecover, "%s", rec.Message)
	}

	if err := cfg.StartOperation("login", ""); err != nil {
		return err
	}

	ctx := context.Background()

	// Conservative login policy: always perform a fresh login.
	// The recovery layer already nukes ambiguous cert state, so if we get
	// here the cert is either fully valid (both cert + domain exist, which
	// is handled by recovery as "already complete") or wiped.
	// We never silently reuse a partial cert.
	{
		// Remove any leftover cert to guarantee a clean slate
		if cfg.CertPemPath != "" {
			os.Remove(cfg.CertPemPath)
			cfg.CertPemPath = ""
		}
		home, _ := os.UserHomeDir()
		os.Remove(home + "/.cloudflared/cert.pem")

		util.Log(util.TagCloudflared, "running cloudflared tunnel login...")
		util.Log(util.TagOctunnel, "a browser window will open — please authorize the domain")

		lines, runErr := process.RunCommandParsing(ctx, util.TagCloudflared, "cloudflared", "tunnel", "login")

		certPath := ""
		for _, line := range lines {
			if m := process.CertPemRe.FindStringSubmatch(line); m != nil {
				certPath = m[1]
			}
		}

		if certPath == "" {
			// fallback: check default location
			home, _ := os.UserHomeDir()
			defaultCert := home + "/.cloudflared/cert.pem"
			if _, err := os.Stat(defaultCert); err == nil {
				certPath = defaultCert
				util.Log(util.TagRecover, "cert path not in output, found at default location: %s", certPath)
			}
		}

		if certPath == "" {
			msg := "could not find cert.pem path"
			if runErr != nil {
				msg = fmt.Sprintf("login failed: %v", runErr)
			}
			cfg.SetFailed(msg)
			return fmt.Errorf("%s. Please run 'cloudflared tunnel login' manually and retry", msg)
		}

		cfg.CertPemPath = certPath
		if err := cfg.UpdatePhase("cert_detected"); err != nil {
			return err
		}
		util.LogSuccess(util.TagCloudflared, "cert.pem: %s", certPath)
	}

	if err := cfg.UpdatePhase("login_completed"); err != nil {
		return err
	}

	// Phase: base domain input — always ask (conservative: no reuse of old domain)
	cfg.BaseDomain = ""
	{
		domain := promptInput("Enter your base domain (e.g., example.com): ")
		if domain == "" {
			cfg.SetFailed("no domain provided")
			return fmt.Errorf("base domain is required")
		}

		// Strip protocol and trailing slash
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimRight(domain, "/")

		fmt.Printf("\n  Base domain: %s\n\n", domain)
		if !promptYN("Is this correct? (Y/n): ", true) {
			cfg.SetFailed("domain not confirmed")
			return fmt.Errorf("domain not confirmed, please retry")
		}

		cfg.BaseDomain = domain
	}

	if err := cfg.UpdatePhase("base_domain_saved"); err != nil {
		return err
	}
	util.LogSuccess(util.TagOctunnel, "base domain saved: %s", cfg.BaseDomain)
	fmt.Println()
	fmt.Println("Next step:")
	fmt.Println("  octunnel auth    — create tunnel + connect DNS")

	return cfg.SetCompleted()
}

// ---------- prompt helpers ----------

func promptInput(prompt string) string {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func promptYN(prompt string, defaultYes bool) bool {
	if prompt != "" {
		fmt.Print(prompt)
	}
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "" {
			return defaultYes
		}
		return answer == "y" || answer == "yes"
	}
	return defaultYes
}
