package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewSpaceFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On Windows, os.UserHomeDir uses USERPROFILE instead of HOME.
	t.Setenv("USERPROFILE", home)
	project := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, folder, machine, want string
	}{
		{"local default", "", "", home},
		{"local explicit", project, "", project},
		{"local missing", filepath.Join(project, "missing"), "", home},
		{"local file", file, "", home},
		{"remote default", "", "remote", ""},
		{"remote explicit", "/remote/project", "remote", "/remote/project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newSpaceFolder(tc.folder, tc.machine); got != tc.want {
				t.Errorf("newSpaceFolder(%q, %q) = %q, want %q", tc.folder, tc.machine, got, tc.want)
			}
		})
	}
}
