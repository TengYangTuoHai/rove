package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// findHC prefers the core shipped with this app, independent of the working
// directory and the desktop launcher's PATH. Explicit Connect paths bypass it.
func findHC() (string, error) {
	executable, _ := os.Executable()
	home, _ := os.UserHomeDir()
	return resolveHC(executable, home, os.Getenv("LOCALAPPDATA"), runtime.GOOS, exec.LookPath)
}

func resolveHC(executable, home, localAppData, goos string, lookPath func(string) (string, error)) (string, error) {
	name, canonical := "hc", "herdr-core"
	if goos == "windows" {
		name += ".exe"
		canonical += ".exe"
	}
	var candidates []string
	if executable != "" {
		dir := filepath.Dir(executable)
		resources := dir
		if goos == "darwin" && filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
			resources = filepath.Join(dir, "..", "Resources")
			// Keep the upstream core in its own bundle for macOS privacy access.
			candidates = append(candidates, filepath.Join(resources, "bin", "Herdr Core.app", "Contents", "MacOS", "herdr-core"))
		}
		candidates = append(candidates, filepath.Join(resources, "bin", name), filepath.Join(resources, name))
	}
	for _, path := range candidates {
		if executableFile(path, goos) {
			return filepath.Clean(path), nil
		}
	}
	for _, command := range []string{name, canonical} {
		if path, err := lookPath(command); err == nil {
			return path, nil
		}
	}
	candidates = nil
	if goos == "windows" && localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Programs", "herdr-core", canonical), filepath.Join(localAppData, "Programs", "herdr-core", name))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", canonical), filepath.Join(home, ".local", "bin", name))
	}
	for _, path := range candidates {
		if executableFile(path, goos) {
			return path, nil
		}
	}
	return "", fmt.Errorf("herdr-core (hc) is missing. Reinstall Rove with its bundled core, or install hc from https://github.com/herdrdev/herdr-core#install and restart Rove")
}

// paneEnvironment is the app's environment without the color policy of the
// shell that started it: a core this app starts gives its own environment to
// every terminal it opens, so a NO_COLOR from the launcher would leave all of
// them without colors. A pane's shell still sets these for what it runs.
func paneEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		switch key, _, _ := strings.Cut(entry, "="); key {
		case "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR":
			continue
		}
		out = append(out, entry)
	}
	return out
}

func hcEnvironment(binary string) []string {
	env := paneEnvironment(os.Environ())
	if !filepath.IsAbs(binary) {
		return env
	}
	dir := filepath.Dir(binary)
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(filepath.Dir(dir))) == "Herdr Core.app" {
		dir = filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}
	name := "hc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executableFile(filepath.Join(dir, name), runtime.GOOS) {
		// A newly started core passes this PATH to its terminals, so commands
		// such as hc machine add also work without a separate CLI install.
		env = append(env, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env
}

func executableFile(path, goos string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (goos == "windows" || info.Mode().Perm()&0o111 != 0)
}
