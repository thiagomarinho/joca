package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var bin string

// TestMain builds the binary once before all e2e tests run.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "joca-e2e-bin")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	bin = filepath.Join(tmp, "joca")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".." // repo root
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		panic("build failed: " + string(out))
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// run executes the binary with a controlled HOME and returns combined output and exit code.
func run(t *testing.T, home string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	return string(out), 0
}

func TestVersion(t *testing.T) {
	home := t.TempDir()
	out, code := run(t, home, "version")
	if code != 0 {
		t.Fatalf("version exited %d: %s", code, out)
	}
	if !strings.Contains(out, "joca") {
		t.Errorf("unexpected version output: %s", out)
	}
}

func TestHelp_listsSubcommands(t *testing.T) {
	home := t.TempDir()
	out, code := run(t, home, "--help")
	if code != 0 {
		t.Fatalf("--help exited %d: %s", code, out)
	}
	if !strings.Contains(out, "version") {
		t.Errorf("--help output missing 'version': %s", out)
	}
}

func TestUnknownCommand_exitsNonZero(t *testing.T) {
	home := t.TempDir()
	_, code := run(t, home, "notacommand")
	if code == 0 {
		t.Fatal("unknown command should exit non-zero")
	}
}

func TestAdd_github(t *testing.T) {
	home := t.TempDir()
	out, code := run(t, home, "add", "github", "thiagomarinho/joca", "--name", "joca")
	if code != 0 {
		t.Fatalf("add exited %d: %s", code, out)
	}
	if !strings.Contains(out, "joca") {
		t.Errorf("unexpected add output: %s", out)
	}
	// Config file should now exist
	cfgPath := home + "/.joca/config.yaml"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if !strings.Contains(string(data), "thiagomarinho") {
		t.Errorf("config missing owner: %s", string(data))
	}
}

func TestAdd_duplicate_exitsNonZero(t *testing.T) {
	home := t.TempDir()
	run(t, home, "add", "github", "thiagomarinho/joca", "--name", "joca")
	_, code := run(t, home, "add", "github", "thiagomarinho/joca", "--name", "joca")
	if code == 0 {
		t.Fatal("duplicate add should exit non-zero")
	}
}

func TestAdd_unknownProvider_exitsNonZero(t *testing.T) {
	home := t.TempDir()
	_, code := run(t, home, "add", "bitbucket", "thiagomarinho/joca")
	if code == 0 {
		t.Fatal("unknown provider should exit non-zero")
	}
}

func TestHelp_showsAddAndTui(t *testing.T) {
	home := t.TempDir()
	out, code := run(t, home, "--help")
	if code != 0 {
		t.Fatalf("--help exited %d: %s", code, out)
	}
	for _, sub := range []string{"add", "tui", "version"} {
		if !strings.Contains(out, sub) {
			t.Errorf("--help output missing %q: %s", sub, out)
		}
	}
}
