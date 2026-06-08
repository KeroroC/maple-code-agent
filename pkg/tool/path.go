package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkspacePath resolves a user-supplied relative path against root and
// ensures the result stays within root after cleaning and symlink resolution.
// Absolute paths are rejected.
func WorkspacePath(root, input string) (string, error) {
	if filepath.IsAbs(input) {
		return "", fmt.Errorf("absolute path not allowed: %s", input)
	}
	cleaned := filepath.Clean(input)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes workspace: %s", input)
	}
	joined := filepath.Join(root, cleaned)
	resolved := joined
	// Resolve symlinks if the file exists.
	if info, err := os.Lstat(joined); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err = filepath.EvalSymlinks(joined)
			if err != nil {
				return "", fmt.Errorf("symlink resolution failed: %w", err)
			}
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve root: %w", err)
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
		return "", fmt.Errorf("path escapes workspace: %s", input)
	}
	return absResolved, nil
}
