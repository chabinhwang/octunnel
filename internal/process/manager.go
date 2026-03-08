package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chabinhwang/octunnel/internal/config"
	"github.com/chabinhwang/octunnel/internal/util"
)

// ProcessManager owns child processes (opencode serve, cloudflared)
// and handles their lifecycle.
type ProcessManager struct {
	Cfg         *config.Config
	opencode    *exec.Cmd
	cloudflared *exec.Cmd
	mu          sync.Mutex
}

func NewProcessManager(cfg *config.Config) *ProcessManager {
	return &ProcessManager{Cfg: cfg}
}

// ---------- opencode serve ----------

func (pm *ProcessManager) StartOpencode(ctx context.Context) (string, error) {
	if pm.Cfg.OpencodePID > 0 && util.IsProcessWithName(pm.Cfg.OpencodePID, "opencode") {
		util.Log(util.TagRecover, "opencode serve is already running (PID %d)", pm.Cfg.OpencodePID)
		url, err := DetectOpencodePort()
		if err == nil {
			return url, nil
		}
		util.Log(util.TagRecover, "could not detect port via lsof, restarting opencode")
		util.KillProcess(pm.Cfg.OpencodePID)
		time.Sleep(500 * time.Millisecond)
	}

	cmd := exec.CommandContext(ctx, "opencode", "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start opencode serve: %w", err)
	}

	pm.mu.Lock()
	pm.opencode = cmd
	pm.mu.Unlock()

	pm.Cfg.OpencodePID = cmd.Process.Pid
	_ = pm.Cfg.Save()

	util.Log(util.TagOpencode, "started (PID %d)", cmd.Process.Pid)

	urlCh := make(chan string, 2)
	go streamAndParse(stdout, util.TagOpencode, OpencodeListenRe, urlCh)
	go streamFilteredWarnOnly(stderr, util.TagOpencode, OpencodeListenRe, urlCh)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case url := <-urlCh:
		return url, nil
	case <-time.After(30 * time.Second):
		util.Log(util.TagOpencode, "timeout waiting for URL from stdout, trying lsof...")
		return DetectOpencodePort()
	}
}

// ---------- cloudflared quick tunnel ----------

func (pm *ProcessManager) StartCloudflaredQuick(ctx context.Context, localURL string) (string, error) {
	cmd := exec.CommandContext(ctx, "cloudflared", "tunnel", "--config", "", "--url", localURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start cloudflared quick tunnel: %w", err)
	}

	pm.mu.Lock()
	pm.cloudflared = cmd
	pm.mu.Unlock()

	pm.Cfg.CloudflaredPID = cmd.Process.Pid
	_ = pm.Cfg.Save()

	util.Log(util.TagCloudflared, "quick tunnel started (PID %d)", cmd.Process.Pid)

	urlCh := make(chan string, 2)
	go streamAndParse(stdout, util.TagCloudflared, QuickTunnelURLRe, urlCh)
	go streamAndParse(stderr, util.TagCloudflared, QuickTunnelURLRe, urlCh)

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case url := <-urlCh:
		return url, nil
	case <-time.After(60 * time.Second):
		return "", fmt.Errorf("timeout waiting for quick tunnel public URL")
	}
}

// ---------- cloudflared named tunnel ----------

func (pm *ProcessManager) StartCloudflaredNamed(ctx context.Context, tunnelName string, configPath string) error {
	cmd := exec.CommandContext(ctx, "cloudflared", "tunnel", "--config", configPath, "run", tunnelName)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloudflared tunnel run: %w", err)
	}

	pm.mu.Lock()
	pm.cloudflared = cmd
	pm.mu.Unlock()

	pm.Cfg.CloudflaredPID = cmd.Process.Pid
	_ = pm.Cfg.Save()

	util.Log(util.TagCloudflared, "named tunnel started (PID %d)", cmd.Process.Pid)

	go streamAndParse(stdout, util.TagCloudflared, nil, nil)
	go streamAndParse(stderr, util.TagCloudflared, nil, nil)

	return nil
}

// ---------- wait / cleanup ----------

type processExit struct {
	name string
	err  error
}

func (pm *ProcessManager) Wait(ctx context.Context) error {
	errc := make(chan processExit, 2)

	pm.mu.Lock()
	oc := pm.opencode
	cf := pm.cloudflared
	pm.mu.Unlock()

	if oc != nil {
		go func() { errc <- processExit{name: "opencode", err: oc.Wait()} }()
	}
	if cf != nil {
		go func() { errc <- processExit{name: "cloudflared", err: cf.Wait()} }()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case exit := <-errc:
		if exit.err != nil {
			return fmt.Errorf("%s exited: %v", exit.name, exit.err)
		}
		return fmt.Errorf("%s exited unexpectedly", exit.name)
	}
}

func (pm *ProcessManager) Cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, cmd := range []*exec.Cmd{pm.cloudflared, pm.opencode} {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		pid := cmd.Process.Pid

		descendants := findDescendants(pid)

		_ = syscall.Kill(-pid, syscall.SIGTERM)
		for _, dp := range descendants {
			_ = syscall.Kill(dp, syscall.SIGTERM)
		}

		done := make(chan struct{})
		go func(c *exec.Cmd) {
			c.Wait()
			close(done)
		}(cmd)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			for _, dp := range descendants {
				_ = syscall.Kill(dp, syscall.SIGKILL)
			}
		}

		time.Sleep(200 * time.Millisecond)
		for _, dp := range descendants {
			if util.IsProcessAlive(dp) {
				_ = syscall.Kill(dp, syscall.SIGKILL)
			}
		}
	}
}

func findDescendants(pid int) []int {
	out, err := util.ExecOutput("pgrep", "-P", fmt.Sprintf("%d", pid))
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var p int
		if _, err := fmt.Sscanf(line, "%d", &p); err == nil {
			pids = append(pids, p)
			pids = append(pids, findDescendants(p)...)
		}
	}
	return pids
}

// ---------- stream helpers ----------

var warnErrorRe = regexp.MustCompile(`(?i)\b(WARN|ERROR|FATAL|PANIC)\b`)

func streamAndParse(r io.Reader, tag util.Tag, pattern *regexp.Regexp, resultCh chan<- string) {
	streamFiltered(r, tag, pattern, resultCh, false)
}

func streamFilteredWarnOnly(r io.Reader, tag util.Tag, pattern *regexp.Regexp, resultCh chan<- string) {
	streamFiltered(r, tag, pattern, resultCh, true)
}

func streamFiltered(r io.Reader, tag util.Tag, pattern *regexp.Regexp, resultCh chan<- string, warnOnly bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !warnOnly || warnErrorRe.MatchString(line) {
			util.Log(tag, "%s", line)
		}
		if pattern != nil && resultCh != nil {
			if m := pattern.FindStringSubmatch(line); m != nil {
				select {
				case resultCh <- m[1]:
				default:
				}
			}
		}
	}
}
