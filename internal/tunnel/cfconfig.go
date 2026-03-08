package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chabinhwang/octunnel/internal/config"
)

// OctunnelConfigPath returns the octunnel-managed cloudflared config path:
// ~/.octunnel/cloudflared.yml
// This is completely separate from ~/.cloudflared/config.yml so we never
// interfere with the user's existing cloudflared setup.
func OctunnelConfigPath() string {
	return filepath.Join(config.Dir(), "cloudflared.yml")
}

// WriteCloudflaredConfig writes the octunnel-owned cloudflared config to
// ~/.octunnel/cloudflared.yml. This file is passed via --config flag when
// running `cloudflared tunnel run`.
func WriteCloudflaredConfig(tunnelID, credFile, hostname, serviceURL string) error {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tunnel: %s\n", tunnelID))
	b.WriteString(fmt.Sprintf("credentials-file: %s\n", credFile))
	b.WriteString("ingress:\n")
	b.WriteString(fmt.Sprintf("  - hostname: %s\n", hostname))
	b.WriteString(fmt.Sprintf("    service: %s\n", serviceURL))
	b.WriteString("  - service: http_status:404\n")

	dir := config.Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := OctunnelConfigPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("failed to write cloudflared config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to save cloudflared config: %w", err)
	}
	return nil
}
