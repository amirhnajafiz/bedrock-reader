package files

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// GetFileNamesByWildcardMatch accepts a root and pattern, and returns the file
// names matching with the pattern.
func GetFileNamesByWildcardMatch(root string, pattern string) ([]string, error) {
	names := make([]string, 0)

	// walk the dir and look for matching patterns
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if match, err := filepath.Match(pattern, d.Name()); err == nil && match {
			names = append(names, d.Name())
		}

		return nil
	})

	return names, err
}

// GetFileSize returns the size of a file.
func GetFileSize(path string) (int64, error) {
	// open the log file
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("failed to open the tracing file: %v", err)
	}
	defer file.Close()

	// get the file stat
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to get file stat: %v", err)
	}

	return info.Size(), nil
}

// sort a target file by calling cort command.
func SortFile(filePath string) error {
	// run Linux sort command
	cmd := exec.Command("sort", filePath, "-o", filePath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to sort file: %v", err)
	}

	return nil
}
