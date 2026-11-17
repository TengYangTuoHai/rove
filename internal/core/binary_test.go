package core

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveHC(t *testing.T) {
	for _, tc := range []struct {
		name, goos, executable, candidate string
	}{
		{"macOS bundle", "darwin", "Herdr.app/Contents/MacOS/Herdr", "Herdr.app/Contents/Resources/bin/hc"},
		{"macOS core app", "darwin", "Herdr.app/Contents/MacOS/Herdr", "Herdr.app/Contents/Resources/bin/Herdr Core.app/Contents/MacOS/herdr-core"},
		{"Windows bundle", "windows", "Herdr/Herdr.exe", "Herdr/bin/hc.exe"},
		{"Linux bundle", "linux", "Herdr/herdr", "Herdr/bin/hc"},
		{"portable adjacent core", "windows", "Herdr/Herdr.exe", "Herdr/hc.exe"},
		{"Unix installed core", "linux", "Herdr/herdr", "home/.local/bin/herdr-core"},
		{"legacy installed hc", "darwin", "Herdr/herdr", "home/.local/bin/hc"},
		{"Windows installed core", "windows", "Herdr/Herdr.exe", "local/Programs/herdr-core/herdr-core.exe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.goos != "windows" {
				t.Skip("Windows filesystems do not preserve Unix executable permissions")
			}
			root := t.TempDir()
			candidate := filepath.Join(root, tc.candidate)
			if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(candidate, []byte("core"), 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := resolveHC(filepath.Join(root, tc.executable), filepath.Join(root, "home"), filepath.Join(root, "local"), tc.goos, func(string) (string, error) {
				return "", errors.New("empty PATH")
			})
			if err != nil || got != candidate {
				t.Fatalf("resolveHC = %q, %v; want %q", got, err, candidate)
			}
		})
	}
}

func TestResolveHCPrefersBundleToPATH(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "hc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bundled := filepath.Join(root, "bin", name)
	if err := os.WriteFile(bundled, []byte("core"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveHC(filepath.Join(root, "Herdr"), "", "", runtime.GOOS, func(string) (string, error) {
		t.Fatal("PATH was searched despite bundled hc")
		return "", nil
	})
	if err != nil || got != bundled {
		t.Fatalf("resolveHC = %q, %v", got, err)
	}
}

func TestResolveHCSkipsDirectoriesAndNonExecutableFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filesystems do not preserve Unix executable permissions")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin", "hc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hc"), []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveHC(filepath.Join(root, "Herdr"), "", "", "linux", func(name string) (string, error) {
		if name == "herdr-core" {
			return "/installed/herdr-core", nil
		}
		return "", errors.New("not found")
	})
	if err != nil || got != "/installed/herdr-core" {
		t.Fatalf("resolveHC = %q, %v", got, err)
	}
}

func TestHCEnvironmentIncludesBundledCLI(t *testing.T) {
	for _, nested := range []bool{false, true} {
		root := t.TempDir()
		alias := "hc"
		if runtime.GOOS == "windows" {
			alias += ".exe"
		}
		if err := os.WriteFile(filepath.Join(root, alias), []byte("core"), 0o755); err != nil {
			t.Fatal(err)
		}
		binary := filepath.Join(root, alias)
		if nested {
			binary = filepath.Join(root, "Herdr Core.app", "Contents", "MacOS", "herdr-core")
		}
		t.Setenv("PATH", "/original/path")
		t.Setenv("HC_SESSION", "keep-session")
		var path, session string
		for _, value := range hcEnvironment(binary) {
			key, value, _ := strings.Cut(value, "=")
			if strings.EqualFold(key, "PATH") {
				path = value
			}
			if key == "HC_SESSION" {
				session = value
			}
		}
		want := root + string(os.PathListSeparator) + "/original/path"
		if path != want || session != "keep-session" {
			t.Fatalf("core env PATH=%q, HC_SESSION=%q; want PATH=%q", path, session, want)
		}
		if os.Getenv("PATH") != "/original/path" {
			t.Fatal("changed the app's environment")
		}
	}
}

func TestResolveHCMissing(t *testing.T) {
	_, err := resolveHC(filepath.Join(t.TempDir(), "Herdr"), "", "", "windows", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err == nil || !strings.Contains(err.Error(), "https://github.com/herdrdev/herdr-core#install") {
		t.Fatalf("missing core has no installation instructions: %v", err)
	}
}

func TestCoreEnvironmentDropsTheLaunchersColorPolicy(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR", "0")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("FORCE_COLOR", "3")
	t.Setenv("NO_COLOR_LOG", "kept")
	t.Setenv("ROVE_UNRELATED", "kept")
	binary := filepath.Join(t.TempDir(), "hc")
	if err := os.WriteFile(binary, []byte("core"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := hcEnvironment(binary)
	has := func(key string) bool {
		for _, entry := range env {
			if k, _, _ := strings.Cut(entry, "="); k == key {
				return true
			}
		}
		return false
	}
	for _, key := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR"} {
		if has(key) {
			t.Errorf("core env keeps %s: a terminal the core opens inherits the launcher's color policy", key)
		}
	}
	for _, key := range []string{"NO_COLOR_LOG", "ROVE_UNRELATED"} {
		if !has(key) {
			t.Errorf("core env lost %s", key)
		}
	}
	if os.Getenv("NO_COLOR") != "1" {
		t.Error("changed the app's environment")
	}
}
