package scenario

import (
	"os"
	"path/filepath"
)

const DefaultRelativeDir = "data/scenarios"

// SourcePath returns .../data/scenarios/{name}/source.json under baseDir.
func SourcePath(baseDir, name string) string {
	return filepath.Join(baseDir, name, "source.json")
}

// FindDataDir walks up from cwd until baseDir exists or repo root is reached.
func FindDataDir(baseDir string) (string, bool) {
	if baseDir == "" {
		baseDir = DefaultRelativeDir
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, baseDir)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
