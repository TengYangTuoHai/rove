package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// The install prompt must survive redraws and receive the answer, even though
// its terminal has no pane in the spaces document.
func TestMachineInstallTerminalSurvivesRedrawsAndTakesInput(t *testing.T) {
	RegisterFonts()
	t.Setenv("ROVE_HOME", t.TempDir())
	conn := newPeekConn()
	term, err := terminal.New(terminal.Options{
		Conn:  conn,
		Font:  terminal.Font{Family: TerminalFont, Size: 14},
		Theme: terminalTheme(ThemeByName("graphite")),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.mach = machineAddState{loaded: true, term: "tinstall", target: "windows", item: map[string]string{"windows": "terminal"}}
	key := "machine-add|tinstall"
	app.terms[key] = &paneTerm{key: key, pane: "machine-add", addr: "tinstall", term: term}
	app.OpenOverlay("machine-add")
	tt := myui.NewTester(app.View, 1000, 700)
	tt.SetFocused(true)
	for range 4 {
		tt.Frame()
	}
	if dir := os.Getenv("MACHINE_SHOT_DIR"); dir != "" {
		writeFullscreenShot(t, tt, filepath.Join(dir, "machine-install.png"))
	}
	app.mu.Lock()
	kept := app.terms[key] != nil && app.terms[key].term == term
	app.mu.Unlock()
	if !kept {
		t.Fatal("redrawing the add dialog discarded the install terminal")
	}
	tt.Key(0, myui.KeyEscape)
	if app.overlay != "machine-add" {
		t.Fatal("Escape dismissed the dialog instead of reaching its terminal")
	}
	tt.Type("y")
	tt.Key(0, myui.KeyEnter)
	tt.Frame()
	conn.flushInput(t, term)
	if got := conn.typed(); got != "\x1by\r" {
		t.Fatalf("install terminal received %q, want the answer and Enter", got)
	}
	app.CloseOverlay()
	app.collect()
	app.mu.Lock()
	kept = app.terms[key] != nil
	app.mu.Unlock()
	if kept {
		t.Fatal("closing the dialog retained its terminal attachment")
	}
}

func TestMachineInstallCanBeReopenedAfterDismissal(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.mach = machineAddState{loaded: true, term: "tinstall", target: "windows", item: map[string]string{"windows": "terminal"}}
	app.OpenOverlay("machine-add")
	app.CloseOverlay()
	app.OpenOverlay("machine-add")
	st := app.addMachineSnapshot()
	if st.term != "tinstall" || st.target != "windows" {
		t.Fatalf("reopening lost the terminal waiting for an install answer: %+v", st)
	}
}

// MACHINE_HC opts into a smoke test with a real core and PTY. SSH is a local
// fixture, so this never connects to or installs anything on a person's host.
func TestMachineInstallWithCore(t *testing.T) {
	bin := os.Getenv("MACHINE_HC")
	if bin == "" || runtime.GOOS == "windows" {
		t.Skip("set MACHINE_HC to a native hc binary on Unix")
	}
	bin, err := filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	RegisterFonts()
	root, err := os.MkdirTemp("/tmp", "wma-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for key, value := range map[string]string{
		"HC_SOCKET": filepath.Join(root, "local.sock"), "HC_SESSION": "local",
		"XDG_STATE_HOME": filepath.Join(root, "state"), "XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"ROVE_HOME":     filepath.Join(root, "view"),
		"MACHINE_PEER_SOCKET": filepath.Join(root, "peer.sock"), "MACHINE_TEST_BIN": bin,
	} {
		t.Setenv(key, value)
	}
	gate := filepath.Join(root, "ssh")
	if err := os.WriteFile(gate, []byte("#!/bin/sh\n[ \"$2\" = exit ] && exit 0\nexec env HC_SOCKET=\"$MACHINE_PEER_SOCKET\" HC_SESSION=peer \"$MACHINE_TEST_BIN\" connect\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HC_SSH", gate)
	t.Cleanup(func() {
		for _, socket := range []string{"local.sock", "peer.sock"} {
			cmd := exec.Command(bin, "server", "stop")
			cmd.Env = append(os.Environ(), "HC_SOCKET="+filepath.Join(root, socket))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("stop isolated core: %v: %s", err, out)
			}
		}
	})
	// A core started by a desktop launcher may not have hc on its PATH.
	start := exec.Command(bin, "server", "start")
	start.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start core with a desktop PATH: %v: %s", err, out)
	}
	s, err := herdr.Connect(bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Core.Close)
	promptArgv := []string{"sh", "-c", `printf 'windows has no hc. Copy this hc there? [Y/n] '; read answer; [ "$answer" = y ] || exit 1; exec "$@"`, "sh", bin, "machine", "add", "windows"}
	addr, err := s.RunOnMachine(promptArgv, "")
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(s, ThemeByName("graphite"))
	app.mach = machineAddState{loaded: true, hosts: []sshHost{{Alias: "windows"}}, query: "windows", term: addr, target: "windows", item: map[string]string{"windows": "terminal"}}
	app.OpenOverlay("machine-add")
	t.Cleanup(func() {
		app.CloseOverlay()
		app.collect()
	})
	tt := myui.NewTester(app.View, 1000, 700)
	tt.SetFocused(true)
	waitText := func(want string) *terminal.Terminal {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			app.mu.Lock()
			entry := app.terms["machine-add|"+addr]
			app.mu.Unlock()
			if entry != nil && strings.Contains(entry.term.Text(), want) {
				return entry.term
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("install terminal did not show %q", want)
		return nil
	}
	waitText("[Y/n]")
	tt.Type("n")
	tt.Key(0, myui.KeyEnter)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		tt.Frame()
		if record := s.Terminal(addr); record != nil && !record.Live() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if dir := os.Getenv("MACHINE_SHOT_DIR"); dir != "" {
		writeFullscreenShot(t, tt, filepath.Join(dir, "core-failed.png"))
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatalf("a failed add cannot return to the host list: %v", err)
	}
	if st := app.addMachineSnapshot(); st.term != "" || st.item["windows"] != "" {
		t.Fatalf("a failed add cannot be retried: %+v", st)
	}
	addr, err = s.RunOnMachine(promptArgv, "")
	if err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.mach.term, app.mach.target = addr, "windows"
	app.mach.item["windows"] = "terminal"
	app.mu.Unlock()
	waitText("[Y/n]")
	if dir := os.Getenv("MACHINE_SHOT_DIR"); dir != "" {
		writeFullscreenShot(t, tt, filepath.Join(dir, "core-prompt.png"))
	}
	app.CloseOverlay()
	tt.Frame()
	app.OpenOverlay("machine-add")
	waitText("[Y/n]")
	tt.Type("y")
	tt.Key(0, myui.KeyEnter)
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		tt.Frame()
		if m := s.MachineNamed("windows"); m != nil {
			if m.Connected() {
				break
			}
			// The add callback has refreshed the list; a newly registered
			// machine may still be finishing its first background connection.
			s.RefreshMachines()
		}
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	st := app.addMachineSnapshot()
	if st.term != "" || st.item["windows"] != "" {
		t.Fatalf("successful machine add still waits in its terminal: %+v", st)
	}
	if m := s.MachineNamed("windows"); m == nil || !m.Connected() {
		t.Fatalf("machine was not added: %+v", m)
	}
	if dir := os.Getenv("MACHINE_SHOT_DIR"); dir != "" {
		writeFullscreenShot(t, tt, filepath.Join(dir, "core-added.png"))
	}
	if _, err := s.AddMachineInTerminal("windows"); err != nil {
		t.Fatalf("start the add command when the daemon PATH has no hc: %v", err)
	}
}

// This opt-in check performs a real add, including the normal installer when
// offered. The target remains in the person's core after a successful check.
func TestMachineInstallRealHost(t *testing.T) {
	target := os.Getenv("MACHINE_REAL_TARGET")
	if target == "" {
		t.Skip("set MACHINE_REAL_TARGET only with permission to add that host")
	}
	RegisterFonts()
	t.Setenv("ROVE_HOME", t.TempDir())
	s, err := herdr.Connect(os.Getenv("MACHINE_REAL_HC"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Core.Close)
	t.Logf("core hello: %s; previous machine: %+v", s.Core.Hello, s.MachineNamed(target))
	app := NewApp(s, ThemeByName("graphite"))
	app.mach = machineAddState{loaded: true, hosts: []sshHost{{Alias: target}}, query: target}
	app.OpenOverlay("machine-add")
	t.Cleanup(func() {
		app.CloseOverlay()
		app.mu.Lock()
		entries := app.terms
		app.terms = map[string]*paneTerm{}
		app.starting = map[string]bool{}
		app.mu.Unlock()
		for _, entry := range entries {
			_ = entry.term.Close()
		}
	})
	tt := myui.NewTester(app.View, 1000, 700)
	tt.SetFocused(true)
	app.addMachine(target)
	deadline := time.Now().Add(3 * time.Minute)
	var output string
	reopened, answered := false, false
	shot := func(name string) {
		if dir := os.Getenv("MACHINE_SHOT_DIR"); dir != "" {
			writeFullscreenShot(t, tt, filepath.Join(dir, name))
		}
	}
	for time.Now().Before(deadline) {
		tt.Frame()
		st := app.addMachineSnapshot()
		if state := st.item[target]; state != "" && state != "adding" && state != "terminal" {
			shot("pc-error.png")
			t.Fatalf("adding %s: %s", target, state)
		}
		if st.term != "" {
			app.mu.Lock()
			entry := app.terms["machine-add|"+st.term]
			app.mu.Unlock()
			if entry != nil {
				output = entry.term.Text()
				if !answered && strings.Contains(output, "Copy this hc") && strings.Contains(output, "[Y/n]") {
					if !reopened {
						shot("pc-install-prompt.png")
						app.CloseOverlay()
						tt.Frame()
						app.OpenOverlay("machine-add")
						reopened = true
						continue
					}
					t.Logf("real install prompt after closing and reopening:\n%s", output)
					tt.Type("y")
					tt.Key(0, myui.KeyEnter)
					answered = true
				}
			}
			if record := s.Terminal(st.term); record != nil && !record.Live() && record.ExitCode != nil && *record.ExitCode != 0 {
				shot("pc-error.png")
				t.Fatalf("real add exited %d:\n%s", *record.ExitCode, output)
			}
		}
		if m := s.MachineNamed(target); m != nil && m.Connected() && st.item[target] == "" && st.term == "" {
			shot("pc-added.png")
			res, err := s.Exec(target, "", []string{"cmd", "/d", "/c", "ver"}, 15*time.Second)
			if err != nil || res.Code == nil || *res.Code != 0 {
				t.Fatalf("read-only Windows verification: %+v, %v", res, err)
			}
			t.Logf("connected: %+v; install offered/answered: %v/%v; terminal output:\n%s\nWindows verification: %s", m, reopened, answered, output, res.Stdout)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	shot("pc-timeout.png")
	t.Fatalf("real add did not finish; state: %+v; output:\n%s", app.addMachineSnapshot(), output)
}
