// Package logopen resolves external commands used to view downloaded logs.
package logopen

import (
	"os"
	"os/exec"
	"strings"
)

// Pager returns the configured terminal pager for path.
func Pager(path string) (*exec.Cmd, bool) {
	if cmd, ok := command(os.Getenv("PAGER"), path); ok {
		return cmd, true
	}
	if executable, err := exec.LookPath("bat"); err == nil {
		return exec.Command(executable, "--paging=always", path), true
	}
	if executable, err := exec.LookPath("less"); err == nil {
		return exec.Command(executable, "-R", path), true
	}
	return nil, false
}

// Editor returns the configured log editor. An app setting takes precedence,
// followed by $CODE and then the code executable.
func Editor(configured, path string) (*exec.Cmd, bool) {
	if strings.TrimSpace(configured) != "" {
		return command(configured, path)
	}
	if code := os.Getenv("CODE"); strings.TrimSpace(code) != "" {
		return command(code, path)
	}
	return command("code", path)
}

func command(value, path string) (*exec.Cmd, bool) {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil, false
	}
	executable, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, false
	}
	args := append([]string(nil), parts[1:]...)
	args = append(args, path)
	return exec.Command(executable, args...), true
}
