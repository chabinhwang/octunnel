package util

import (
	"bytes"
	"os/exec"
)

// ExecOutput runs a command and returns its combined stdout.
func ExecOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
