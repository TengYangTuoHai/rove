package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCollectInstallers(t *testing.T) {
	script, err := filepath.Abs("collect-installers.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, versions := range []struct{ app, deb string }{
		{"0.1.0", "0.1.0"},
		{"0.2.0-rc.1.dev.42.gabcdef1", "0.2.0~rc.1.dev.42.gabcdef1"},
		{"0.2.0-rc-1", "0.2.0~rc.1"},
	} {
		version := versions.app
		for _, target := range []struct {
			os    string
			files []string
		}{
			{"darwin", []string{
				"darwin-arm64/rove-" + version + "-arm64.dmg",
				"darwin-amd64/rove-" + version + "-amd64.dmg",
			}},
			{"windows", []string{"windows-amd64/rove-setup-" + version + "-amd64.exe"}},
			{"linux", []string{
				"linux-arm64/rove_" + versions.deb + "_arm64.deb",
				"linux-amd64/rove_" + versions.deb + "_amd64.deb",
			}},
		} {
			t.Run(version+"/"+target.os, func(t *testing.T) {
				for _, missing := range append([]string{""}, target.files...) {
					t.Run("missing="+missing, func(t *testing.T) {
						dir := t.TempDir()
						var want []string
						for _, file := range target.files {
							path := filepath.Join(dir, "build", file)
							if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
								t.Fatal(err)
							}
							if file != missing {
								if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
									t.Fatal(err)
								}
							}
							want = append(want, filepath.Base(file))
							// Other build products must never become release assets.
							for _, extra := range []string{"Rove.exe", "PKGBUILD", "install.sh", "rove-old.dmg", "herdr.tar.gz"} {
								if err := os.WriteFile(filepath.Join(filepath.Dir(path), extra), []byte("not an installer"), 0o644); err != nil {
									t.Fatal(err)
								}
							}
						}
						cmd := exec.Command("bash", script, target.os, version)
						cmd.Dir = dir
						output, err := cmd.CombinedOutput()
						if missing != "" {
							if err == nil || !strings.Contains(string(output), "Missing or empty installer: build/"+missing) {
								t.Fatalf("missing installer %s: %v\n%s", missing, err, output)
							}
							if err := os.WriteFile(filepath.Join(dir, "build", missing), nil, 0o644); err != nil {
								t.Fatal(err)
							}
							cmd = exec.Command("bash", script, target.os, version)
							cmd.Dir = dir
							output, err = cmd.CombinedOutput()
							if err == nil || !strings.Contains(string(output), "Missing or empty installer: build/"+missing) {
								t.Fatalf("empty installer %s: %v\n%s", missing, err, output)
							}
							return
						}
						if err != nil {
							t.Fatalf("collect installers: %v\n%s", err, output)
						}
						entries, err := os.ReadDir(filepath.Join(dir, "artifacts"))
						if err != nil {
							t.Fatal(err)
						}
						var got []string
						for _, entry := range entries {
							got = append(got, entry.Name())
						}
						slices.Sort(want)
						if !slices.Equal(got, want) {
							t.Fatalf("assets = %v, want %v", got, want)
						}
						for _, file := range target.files {
							data, err := os.ReadFile(filepath.Join(dir, "artifacts", filepath.Base(file)))
							if err != nil || string(data) != file {
								t.Fatalf("installer %s was not copied intact: %q, %v", file, data, err)
							}
						}
					})
				}
			})
		}
	}
}
