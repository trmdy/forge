package loop

import (
	"fmt"
	"os"
	"path/filepath"
)

func (r *Runner) writePromptFile(loopID, runID, content string) (string, error) {
	dir := filepath.Join(r.Config.Global.DataDir, "prompts", loopID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("run-%s.md", runID)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
