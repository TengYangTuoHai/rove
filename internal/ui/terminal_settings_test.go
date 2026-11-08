package ui

import (
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

type terminalSettingsConn struct {
	*peekConn
	writes chan struct{}
}

func (c *terminalSettingsConn) Write(p []byte) (int, error) {
	n, err := c.peekConn.Write(p)
	select {
	case c.writes <- struct{}{}:
	default:
	}
	return n, err
}

func (c *terminalSettingsConn) waitFor(t *testing.T, text string) string {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if got := c.typed(); strings.Contains(got, text) {
			return got
		}
		select {
		case <-c.writes:
		case <-deadline.C:
			t.Fatalf("terminal sent %q, want %q", c.typed(), text)
			return ""
		}
	}
}

func TestSavedClipboardPermissionReachesNewTerminals(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "allowed"}[allow], func(t *testing.T) {
			t.Setenv("ROVE_HOME", t.TempDir())
			first := NewApp(nil, ThemeByName("graphite"))
			first.LoadSettings()
			first.settings.Clipboard = allow
			first.saveSettings()
			app := NewApp(nil, ThemeByName("graphite"))
			app.LoadSettings()
			conn := &terminalSettingsConn{peekConn: newPeekConn(), writes: make(chan struct{}, 1)}
			opts := app.currentTerminalOptions()
			opts.Conn, opts.Theme = conn, terminalTheme(app.T)
			term, err := terminal.New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			tt := myui.NewTester(func(c *myui.Context) { terminal.View(c, term).Fill().Focus() }, 600, 200)
			tt.Frame()
			tt.SetClipboard("untouched")
			term.Feed([]byte("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("program write")) + "\a"))
			tt.Frame()
			want := "untouched"
			if allow {
				want = "program write"
			}
			if got := tt.Clipboard(); got != want {
				t.Fatalf("program clipboard write = %q, want %q", got, want)
			}
			// Program reads stay denied even when program writes are allowed.
			term.Feed([]byte("\x1b]52;c;?\x1b\\"))
			tt.Frame()
			if got := conn.waitFor(t, "\x1b]52;c;\x1b\\"); got != "\x1b]52;c;\x1b\\" {
				t.Fatalf("program read response = %q, want an empty clipboard", got)
			}
			// The permission only governs program writes, not manual copying
			// or pasting, which continue through the terminal's own bindings.
			term.Feed([]byte("manual copy"))
			tt.Frame()
			mods := myui.Ctrl | myui.Shift
			if runtime.GOOS == "darwin" {
				mods = myui.Cmd
			}
			tt.Key(mods, myui.KeyA)
			tt.Key(mods, myui.KeyC)
			tt.Frame()
			if got := strings.TrimSpace(tt.Clipboard()); got != "manual copy" {
				t.Fatalf("manual copy = %q", got)
			}
			tt.SetClipboard("manual paste")
			tt.Key(mods, myui.KeyV)
			tt.Frame()
			if got := conn.waitFor(t, "manual paste"); !strings.HasSuffix(got, "manual paste") {
				t.Fatalf("manual paste did not reach program: %q", got)
			}
		})
	}
}

func TestPaneSelectionCopiesToLocalClipboard(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "program writes denied", true: "program writes allowed"}[allow], func(t *testing.T) {
			RegisterFonts()
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.terminalPrefs.clipboard = allow
			conn := newPeekConn()
			opts := app.currentTerminalOptions()
			opts.Conn, opts.Theme = conn, terminalTheme(app.T)
			term, err := terminal.New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			pane := herdr.PaneInfo{Pane: "remote-pane", Terminal: "pc/t1", Focused: true}
			key := pane.Pane + "|" + pane.Terminal
			app.terms[key] = &paneTerm{term: term}
			tt := myui.NewTester(func(c *myui.Context) { app.terminalView(c, pane) }, 600, 200)
			term.Feed([]byte("PS C:\\Users\\user> Windows pane 中文"))
			tt.Frame()
			tt.SetClipboard("previous clipboard")
			// Drag across the first line, then release without pressing Copy.
			tt.Press(8, 12)
			tt.Move(450, 12)
			tt.Release(450, 12)
			tt.Frame()
			want := "PS C:\\Users\\user> Windows pane 中文"
			if out := os.Getenv("PANE_COPY_SHOT_OUT"); out != "" && allow {
				file, err := os.Create(out)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err := png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			if got := strings.TrimSpace(tt.Clipboard()); got != want {
				t.Errorf("selected text = %q, want %q in local clipboard", got, want)
			}
			// Explicit Copy must still work with the selection left in place.
			tt.SetClipboard("previous clipboard")
			mods := myui.Ctrl | myui.Shift
			if runtime.GOOS == "darwin" {
				mods = myui.Cmd
			}
			tt.Key(mods, myui.KeyC)
			if got := strings.TrimSpace(tt.Clipboard()); got != want {
				t.Errorf("copy shortcut = %q, want %q", got, want)
			}
			tt.SetClipboard("previous clipboard")
			tt.Command("copy")
			if got := strings.TrimSpace(tt.Clipboard()); got != want {
				t.Errorf("explicit copy = %q, want %q", got, want)
			}
		})
	}
}

func TestMinimumContrastUpdatesPanesAndPeekInPlace(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	views := make([]*terminal.Terminal, 3)
	for i := range views {
		opts := app.currentTerminalOptions()
		opts.Conn = newPeekConn()
		opts.Theme = &terminal.Theme{Foreground: myui.RGB(255, 255, 255), Background: myui.RGB(0, 0, 0)}
		term, err := terminal.New(opts)
		if err != nil {
			t.Fatal(err)
		}
		defer term.Close()
		term.Feed([]byte("\x1b[?25l\x1b[38;2;30;30;30mLow contrast terminal text"))
		views[i] = term
	}
	app.terms["visible"] = &paneTerm{term: views[0]}
	app.terms["cached"] = &paneTerm{term: views[1]}
	app.peek.term = views[2]
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Children(func() {
			for _, term := range views {
				terminal.View(c, term).Width(600).Height(140)
			}
		})
	}, 600, 420)
	tt.Frame()
	cols, rows := views[0].Size()
	brightness := func(im image.Image, band int) uint32 {
		var brightest uint32
		for y := band * 140; y < (band+1)*140; y++ {
			for x := 0; x < 550; x++ {
				r, _, _, _ := im.At(x, y).RGBA()
				brightest = max(brightest, r>>8)
			}
		}
		return brightest
	}
	before := [3]uint32{}
	for i := range before {
		before[i] = brightness(tt.Image(), i)
		if before[i] > 35 || before[i] < 15 {
			t.Fatalf("view %d baseline brightness = %d, want original gray text", i, before[i])
		}
	}
	writeShot := func(name string) {
		t.Helper()
		if dir := os.Getenv("TERMINAL_SETTINGS_SHOT_DIR"); dir != "" {
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeShot("contrast-1")
	app.settings.MinContrast = 7
	app.saveSettings()
	tt.Frame()
	for i, term := range views {
		if got := brightness(tt.Image(), i); got < 140 || got <= before[i] {
			t.Errorf("view %d brightness = %d, want lightened text at contrast 7", i, got)
		}
		if c, r := term.Size(); c != cols || r != rows {
			t.Errorf("view %d grid changed from %dx%d to %dx%d", i, cols, rows, c, r)
		}
		if !strings.Contains(term.Text(), "Low contrast terminal text") {
			t.Errorf("view %d lost its output", i)
		}
		select {
		case <-term.Done():
			t.Errorf("view %d closed during contrast update", i)
		default:
		}
	}
	writeShot("contrast-7")
	app.settings.MinContrast = 1
	app.saveSettings()
	tt.Frame()
	for i := range views {
		if got := brightness(tt.Image(), i); got != before[i] {
			t.Errorf("view %d reset brightness = %d, want %d", i, got, before[i])
		}
	}
}

func TestMinimumContrastIsAppliedAfterRelaunch(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	first.settings.MinContrast = 4.5
	first.saveSettings()
	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if got := second.currentTerminalOptions().MinimumContrast; got != 4.5 {
		t.Fatalf("new terminal minimum contrast = %v, want 4.5", got)
	}
}
