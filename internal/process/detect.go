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

	"github.com/chabinhwang/octunnel/internal/util"
)

// Patterns used to parse child process output.
var (
	OpencodeListenRe = regexp.MustCompile(`listening on (https?://[^\s]+)`)
	QuickTunnelURLRe = regexp.MustCompile(`(https://[a-zA-Z0-9-]+\.trycloudflare\.com)`)
	CertPemRe        = regexp.MustCompile(`saved to:\s*(\S+cert\.pem)`)
	TunnelCreatedRe  = regexp.MustCompile(`Created tunnel (\S+) with id ([a-f0-9-]+)`)
	TunnelCredFileRe = regexp.MustCompile(`Tunnel credentials written to (\S+\.json)`)
	TunnelExistsRe   = regexp.MustCompile(`tunnel with name already exists`)
)

// DetectOpencodePort uses lsof to find the LISTEN port of a running opencode process.
func DetectOpencodePort() (string, error) {
	out, err := util.ExecOutput("lsof", "-i", "-P", "-n")
	if err != nil {
		return "", fmt.Errorf("lsof failed: %w", err)
	}

	re := regexp.MustCompile(`opencode\s+\d+\s+\S+\s+\S+\s+IPv[46]\s+\S+\s+\S+\s+TCP\s+(\S+)\s+\(LISTEN\)`)
	for _, line := range strings.Split(out, "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			addr := m[1]
			return "http://" + addr, nil
		}
	}
	return "", fmt.Errorf("no opencode LISTEN port found via lsof")
}

// RunCommandParsing runs a command, streams its output with the given tag,
// and returns all lines for further parsing.
func RunCommandParsing(ctx context.Context, tag util.Tag, name string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var lines []string
	var mu sync.Mutex

	collect := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := sc.Text()
			util.Log(tag, "%s", line)
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); collect(stdout) }()
	go func() { defer wg.Done(); collect(stderr) }()
	wg.Wait()

	err = cmd.Wait()
	return lines, err
}
