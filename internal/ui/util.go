package ui

import (
	"os"
	"strings"

	"rove/internal/gitcmd"
)

// readDir lists a folder, folders first, without dotfiles.
func readDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && e.Name() != ".gitignore" {
			continue
		}
		out = append(out, e)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j].IsDir() && !out[j-1].IsDir() {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	}
	return out, nil
}

// gitStatus lists what changed in a folder.
func gitStatus(dir string) ([]string, error) {
	out, err := gitcmd.Output(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
