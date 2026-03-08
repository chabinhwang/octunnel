package util

import (
	"fmt"
	"os/exec"
)

// CheckDependencies ensures that opencode, cloudflared and lsof are available.
func CheckDependencies() error {
	deps := []struct {
		name string
		help string
	}{
		{"opencode", "Install: npm install -g opencode\n  For the latest install method, see: https://github.com/anomalyco/opencode"},
		{"cloudflared", "Install: brew install cloudflared (macOS)\n  For the latest install method, see: https://github.com/cloudflare/cloudflared"},
		{"lsof", "lsof should be available on macOS/Linux by default"},
	}
	for _, d := range deps {
		if _, err := exec.LookPath(d.name); err != nil {
			return fmt.Errorf("%s is not installed or not in PATH. %s", d.name, d.help)
		}
	}
	return nil
}
