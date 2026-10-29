package scenario

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads source.json for a scenario name under baseDir.
func Load(baseDir, name string) (*SourceFile, error) {
	path := SourcePath(baseDir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario %s: %w", path, err)
	}
	var sf SourceFile
	if err := json.Unmarshal(b, &sf); err != nil {
		return nil, fmt.Errorf("parse scenario %s: %w", path, err)
	}
	return &sf, nil
}
