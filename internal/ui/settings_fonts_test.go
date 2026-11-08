package ui

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestFontSettingsAndZoomSurviveLaunch(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	first := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	first.LoadSettings()
	first.settings.InterfaceFont, first.settings.InterfaceSize = "system-ui", 16
	first.settings.TerminalFont, first.settings.TerminalSize = "monospace", 19
	first.Run("app.zoom_in")
	tt := myui.NewTester(first.View, 1280, 820)
	tt.Frame()

	second := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	second.LoadSettings()
	if second.settings.InterfaceFont != "system-ui" || second.settings.InterfaceSize != 16 ||
		second.settings.TerminalFont != "monospace" || second.settings.TerminalSize != 19 || second.Zoom != first.Zoom {
		t.Fatalf("font and zoom choices did not survive: %+v, zoom %v", second.settings, second.Zoom)
	}
	if got := second.currentTerminalFont(); got.Family != "monospace" || got.Size != 19 {
		t.Fatalf("new terminal font = %+v", got)
	}
	second.Run("app.zoom_reset")
	var zoom float32
	tt = myui.NewTester(func(c *myui.Context) { second.View(c); zoom = c.Zoom() }, 1280, 820)
	tt.Frame()
	if zoom != 1 {
		t.Fatalf("reset left rendered zoom at %v", zoom)
	}
	third := NewApp(nil, ThemeByName("graphite"))
	third.LoadSettings()
	if third.Zoom != 1 {
		t.Fatalf("reset was not saved: %v", third.Zoom)
	}
}

func TestZoomResetUpdatesExistingView(t *testing.T) {
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	var zoom float32
	tt := myui.NewTester(func(c *myui.Context) { app.View(c); zoom = c.Zoom() }, 1280, 820)
	app.Run("app.zoom_in")
	tt.Frame()
	if zoom <= 1 {
		t.Fatalf("zoom in drew at %v", zoom)
	}
	app.Run("app.zoom_reset")
	tt.Frame()
	if zoom != 1 {
		t.Fatalf("reset left existing view at %v", zoom)
	}
}

func TestReadFontAndZoomSettings(t *testing.T) {
	for _, tc := range []struct {
		data string
		zoom float32
	}{
		{`{}`, 1}, {`{"zoom":0}`, 1}, {`{"zoom":-3}`, 1},
		{`{"zoom":0.2}`, 0.5}, {`{"zoom":9}`, 3}, {`{"zoom":1.25}`, 1.25},
	} {
		s := readSettings([]byte(tc.data), defaultSettings())
		if s.Zoom != tc.zoom {
			t.Errorf("%s: zoom = %v, want %v", tc.data, s.Zoom, tc.zoom)
		}
	}
	s := readSettings([]byte(`{"interface_font":" ","terminal_font":"","terminal_size":100}`), defaultSettings())
	if s.InterfaceFont != UIFont || s.TerminalFont != TerminalFont || s.TerminalSize != 40 {
		t.Fatalf("invalid font settings = %+v", s)
	}
}

func TestTerminalFontChoicesAreMonospaced(t *testing.T) {
	app := NewApp(nil, ThemeByName("graphite"))
	app.fontFamilies = []myui.FontFamily{{Name: UIFont}, {Name: TerminalFont, Monospaced: true}}
	if got := app.fontNames(true); !slices.Equal(got, []string{"monospace", TerminalFont}) {
		t.Fatalf("terminal choices = %v", got)
	}
	if got := app.fontNames(false); !slices.Contains(got, UIFont) || !slices.Contains(got, TerminalFont) {
		t.Fatalf("interface choices = %v", got)
	}
	RegisterFonts()
	app.fontFamilies = nil
	if got := app.fontNames(true); !slices.Contains(got, TerminalFont) {
		t.Fatalf("bundled terminal font missing from installed choices: %v", got)
	}
}

func TestFontControlsApplyAndSave(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	// Keep the fixture small while exercising the real dropdown, keyboard
	// navigation, numeric input and persistence used by Look.
	tt := myui.NewTester(func(c *myui.Context) {
		uiTheme := *c.Theme()
		uiTheme.Font, uiTheme.FontSize = app.interfaceFont(), app.fontSize(13)
		c.SetTheme(&uiTheme)
		myui.Column(c).Gap(20).Padding(20).Children(func() {
			myui.Row(c).Gap(10).Children(func() { app.settingsFont(c, &app.settings.InterfaceFont, &app.settings.InterfaceSize, false) })
			myui.Row(c).Gap(10).Children(func() { app.settingsFont(c, &app.settings.TerminalFont, &app.settings.TerminalSize, true) })
			app.stepper(c, "Zoom")
		})
		app.saveSettings()
	}, 700, 400)
	tt.Frame()
	for _, label := range []string{"Interface font family", "Terminal font family"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Key(0, myui.KeyHome)
		tt.Key(0, myui.KeyEnter)
		tt.Frame()
	}
	if app.settings.InterfaceFont != "system-ui" || app.settings.TerminalFont != "monospace" {
		t.Fatalf("family controls did not apply: %q, %q", app.settings.InterfaceFont, app.settings.TerminalFont)
	}
	for _, label := range []string{"Interface font size", "Terminal font size"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %s control", label)
		}
		tt.ClickAt(r.X+20, r.Y+r.H/2)
		tt.Key(myui.Cmd, myui.KeyA)
		tt.Type("18")
		tt.Key(0, myui.KeyEnter)
		tt.Frame()
	}
	if app.settings.InterfaceSize != 18 || app.settings.TerminalSize != 18 {
		t.Fatalf("size controls did not apply: %d/%d", app.settings.InterfaceSize, app.settings.TerminalSize)
	}
	if err := tt.Click("Zoom in"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.Zoom <= 1 {
		t.Fatal("zoom stepper did not apply")
	}
	data, err := os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved settingsFile
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.InterfaceFont != "system-ui" || saved.TerminalFont != "monospace" || saved.InterfaceSize != 18 || saved.TerminalSize != 18 || saved.Zoom != app.Zoom {
		t.Fatalf("controls were not saved: %+v", saved)
	}
}

func TestFontSizeUpdatesAttachedPanesAndPeek(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	views := make([]*terminal.Terminal, 3)
	for i := range views {
		term, err := terminal.New(terminal.Options{Conn: newPeekConn(), Font: app.currentTerminalFont(), Theme: terminalTheme(app.T)})
		if err != nil {
			t.Fatal(err)
		}
		defer term.Close()
		term.Feed([]byte("\x1b[?25lFont settings preserve terminal output\r\n"))
		views[i] = term
	}
	app.terms["visible"] = &paneTerm{term: views[0]}
	app.terms["cached"] = &paneTerm{term: views[1]}
	app.peek.term = views[2]
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Children(func() {
			for _, term := range views {
				terminal.View(c, term).Width(600).Height(160)
			}
		})
	}, 620, 500)
	tt.Frame()
	cols, rows := views[0].Size()
	app.settings.TerminalFont, app.settings.TerminalSize = "monospace", 24
	app.applyTerminalFont()
	tt.Frame()
	for i, term := range views {
		c, r := term.Size()
		if c >= cols || r >= rows {
			t.Errorf("view %d grid stayed %dx%d; before %dx%d", i, c, r, cols, rows)
		}
		if !strings.Contains(term.Text(), "Font settings") {
			t.Errorf("view %d lost its output", i)
		}
		select {
		case <-term.Done():
			t.Errorf("view %d was closed by font update", i)
		default:
		}
	}
	if out := os.Getenv("SETTINGS_TERMINAL_SHOT_OUT"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInterfaceFontChangesRenderedText(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()
	before, ok := tt.Find("Rove")
	if !ok {
		t.Fatal("brand not rendered")
	}
	app.settings.InterfaceFont = TerminalFont
	tt.Frame()
	family, ok := tt.Find("Rove")
	if !ok || family.W == before.W {
		t.Fatalf("font family did not change text metrics: before %+v, after %+v", before, family)
	}
	app.settings.InterfaceSize = 18
	tt.Frame()
	after, ok := tt.Find("Rove")
	if !ok || after.W <= family.W || after.H <= family.H {
		t.Fatalf("interface text did not grow: before %+v, after %+v", family, after)
	}
}

// SETTINGS_FONT_SHOT_OUT retains the actual Look page for visual review.
func TestSettingsFontShot(t *testing.T) {
	out := os.Getenv("SETTINGS_FONT_SHOT_OUT")
	if out == "" {
		t.Skip("set SETTINGS_FONT_SHOT_OUT")
	}
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.OpenOverlay("settings")
	app.settings.InterfaceFont, app.settings.InterfaceSize = "system-ui", 16
	app.settings.TerminalFont, app.settings.TerminalSize = "monospace", 18
	tt := myui.NewTester(app.View, 1280, 820)
	tt.SetScale(2)
	tt.Frame()
	tt.Scroll(1000, 650, 0, 650)
	tt.Frame()
	for _, shot := range []string{out, strings.TrimSuffix(out, filepath.Ext(out)) + "-picker.png"} {
		if shot != out {
			if err := tt.Click("Terminal font family"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
		}
		f, err := os.Create(shot)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", shot)
	}
}
