package logopen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditorUsesConfiguredCommand(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "custom-editor")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CODE", "ignored-editor")

	cmd, ok := Editor("custom-editor --reuse", "/tmp/log.txt")
	if !ok {
		t.Fatal("expected configured editor command")
	}
	want := []string{executable, "--reuse", "/tmp/log.txt"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
}

func TestEditorUsesCodeEnvironmentWithoutWaiting(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "code")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CODE", "code")

	cmd, ok := Editor("", "/tmp/log.txt")
	if !ok {
		t.Fatal("expected $CODE editor command")
	}
	want := []string{executable, "/tmp/log.txt"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
}
