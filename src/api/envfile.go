package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnvFiles loads the project .env file. Convention: keep .env at the
// repository root (parent of src/api). When you run the app or tests from
// src/api, findDotEnv walks up from the current working directory until it
// finds .env. Existing environment variables are not overwritten.
func loadDotEnvFiles() {
	if path := findDotEnv(); path != "" {
		_ = loadDotEnv(path)
	}
}

func findDotEnv() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}
