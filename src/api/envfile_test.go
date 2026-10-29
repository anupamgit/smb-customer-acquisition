package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDotEnvFromModuleDirectory(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "src", "api")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(root, ".env")
	if err := os.WriteFile(envPath, []byte("FIND_DOT_ENV_TEST=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(moduleDir)
	got := findDotEnv()
	if got != envPath {
		t.Fatalf("findDotEnv() = %q, want %q", got, envPath)
	}
}
