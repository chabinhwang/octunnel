package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Lock is a simple PID-based file lock stored in ~/.octunnel/octunnel.lock.
type Lock struct {
	path string
}

// LockDir returns the directory for the lock file (~/.octunnel).
func LockDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".octunnel")
}

func lockPath() string {
	return filepath.Join(LockDir(), "octunnel.lock")
}

// AcquireLock creates the lock file with the current PID.
func AcquireLock() (*Lock, error) {
	path := lockPath()

	if data, err := os.ReadFile(path); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			if IsProcessAlive(pid) {
				return nil, fmt.Errorf("another octunnel instance is running (PID %d). If this is stale, remove %s", pid, path)
			}
		}
		Log(TagRecover, "removing stale lock file (PID %s)", pidStr)
		os.Remove(path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	pid := os.Getpid()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	return &Lock{path: path}, nil
}

// Release removes the lock file.
func (l *Lock) Release() {
	if l != nil {
		os.Remove(l.path)
	}
}

// IsProcessAlive checks whether the given PID is still running.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// IsProcessWithName checks that the PID is alive AND its command name
// matches the expected binary (best-effort).
func IsProcessWithName(pid int, name string) bool {
	if !IsProcessAlive(pid) {
		return false
	}
	out, err := ExecOutput("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	if err != nil {
		return IsProcessAlive(pid)
	}
	return strings.Contains(strings.TrimSpace(out), name)
}

// KillProcess sends SIGTERM to the process group (kills children too).
func KillProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	return nil
}
