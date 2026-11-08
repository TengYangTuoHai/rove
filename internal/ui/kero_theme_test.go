package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestKeroThemesHaveBothAppearances(t *testing.T) {
	for _, name := range keroThemes {
		for _, dark := range []bool{false, true} {
			th := themeForAppearance(name, dark)
			if th.Dark != dark {
				t.Fatalf("%s dark=%v draws dark=%v", name, dark, th.Dark)
			}
			if th.TerminalForeground == th.Background || th.Cursor == th.Background {
				t.Fatalf("%s dark=%v has invisible terminal text or cursor", name, dark)
			}
			if name != "plain" && len(th.Backdrop.Stops) < 2 {
				t.Fatalf("%s has no wash", name)
			}
		}
	}
	if themeForAppearance("petal", true).Ansi == themeForAppearance("mist", true).Ansi {
		t.Fatal("different tones have the same ANSI colors")
	}
}

func TestThemeChoicesPersistAndFollowAppearance(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	app := NewApp(nil, ThemeByName("graphite"))
	app.LoadSettings()
	app.chooseTheme("petal", false)
	app.chooseTheme("dusk", true)
	app.saveSettings()

	next := NewApp(nil, ThemeByName("graphite"))
	next.LoadSettings()
	for _, dark := range []bool{false, true, false, true} {
		next.SetSystemAppearance(dark)
		want := "petal"
		if dark {
			want = "dusk"
		}
		if next.currentThemeName() != want || next.T.Dark != dark {
			t.Fatalf("dark=%v draws %s, want %s", dark, next.currentThemeName(), want)
		}
	}
	// Forced appearance ignores subsequent changes from the desktop.
	next.settings.Appearance = AppearanceLight
	next.applyTheme()
	next.SetSystemAppearance(false)
	next.SetSystemAppearance(true)
	if next.currentThemeName() != "petal" || next.T.Dark {
		t.Fatal("forced light followed the desktop")
	}
}

func TestGhosttyThemePreservesTerminalColors(t *testing.T) {
	for _, name := range []string{"Catppuccin Mocha", "Catppuccin Latte"} {
		original, err := terminal.GhosttyTheme(name)
		if err != nil {
			t.Fatal(err)
		}
		th := themeForAppearance(name, true)
		if got := terminalTheme(th); *got != *original {
			t.Fatalf("%s terminal colors changed: got %+v, want %+v", name, got, original)
		}
		if th.Foreground != original.Foreground || th.Surface != original.Background {
			t.Fatalf("%s interface does not use the terminal colors", name)
		}
	}
}

func TestInstalledGhosttyThemeKeepsSearchColors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "ghostty", "themes", "Custom test palette")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("background = #112233\nforeground = #ddeeff\nsearch-background = #ffaa00\nsearch-foreground = #001122\nsearch-selected-background = #aabbcc\nsearch-selected-foreground = #223344\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := readSettings([]byte(`{"dark_theme":"Custom test palette"}`), defaultSettings())
	_, th := s.themeFor(true)
	original, err := terminal.GhosttyTheme("Custom test palette")
	if err != nil {
		t.Fatal(err)
	}
	if *terminalTheme(th) != *original {
		t.Fatal("installed theme colors changed")
	}
	matches := ghosttyMatches("custom test")
	if len(matches) != 1 || matches[0] != "Custom test palette" {
		t.Fatalf("installed theme missing from search: %v", matches)
	}
}

func TestOpenAndPeekedTerminalsFollowTheme(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	var terms []*terminal.Terminal
	for range 2 {
		term, err := terminal.New(terminal.Options{Conn: newPeekConn(), Font: terminal.Font{Family: TerminalFont, Size: 14}, Theme: terminalTheme(app.T)})
		if err != nil {
			t.Fatal(err)
		}
		defer term.Close()
		terms = append(terms, term)
	}
	app.terms["pane"] = &paneTerm{term: terms[0]}
	app.peek.term = terms[1]
	app.settings.LightTheme, app.settings.DarkTheme = "petal", "Catppuccin Mocha"
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Row(c).Fill().Children(func() {
			for i, term := range terms {
				terminal.View(c.Key(i), term).Width(400).FillHeight()
			}
		})
	}, 800, 240)
	for _, dark := range []bool{false, true, false} {
		app.SetSystemAppearance(dark)
		tt.Frame()
		img := tt.Image()
		for _, x := range []int{100, 500} {
			r, g, b, _ := img.At(x, 100).RGBA()
			bg := app.T.Background
			if uint8(r>>8) != bg.R || uint8(g>>8) != bg.G || uint8(b>>8) != bg.B {
				t.Fatalf("dark=%v terminal at %d did not repaint to %+v", dark, x, bg)
			}
		}
		if dir := os.Getenv("THEME_SHOT_DIR"); dir != "" {
			name := "live-terminals-light.png"
			if dark {
				name = "live-terminals-dark.png"
			}
			saveThemeShot(t, tt, filepath.Join(dir, name))
		}
	}
}

func TestThemeSettingsKeepLegacyAndValidateEachChoice(t *testing.T) {
	s := readSettings([]byte(`{"theme":"ink"}`), defaultSettings())
	if name, _ := s.themeFor(true); name != "ink" {
		t.Fatal("legacy dark choice lost")
	}
	if name, _ := s.themeFor(false); name != "paper" {
		t.Fatal("legacy light fallback changed")
	}
	s = readSettings([]byte(`{"light_theme":"Catppuccin Latte","dark_theme":"no-such-theme","accent":""}`), defaultSettings())
	if s.LightTheme != "Catppuccin Latte" || s.DarkTheme != "graphite" || s.Accent != "" {
		t.Fatalf("sanitized settings = %+v", s)
	}
}

func TestThemePickerClicksAndGhosttySearch(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(760).Children(func() { app.settingsThemeCard(c) })
	}, 800, 1600)
	if err := tt.Click("Petal"); err != nil {
		t.Fatal(err)
	}
	if app.settings.LightTheme != "petal" || app.settings.DarkTheme != "" {
		t.Fatal("light tile did not change only the light theme")
	}
	app.settings.ThemePicker = 2
	app.settings.ThemeSearch = "catppuccin mocha"
	tt.Frame()
	if !tt.HasText("Catppuccin Mocha") || tt.HasText("Catppuccin Latte") {
		t.Fatal("Ghostty search did not filter results")
	}
	if err := tt.Click("Catppuccin Mocha"); err != nil {
		t.Fatal(err)
	}
	if app.settings.DarkTheme != "Catppuccin Mocha" || app.settings.LightTheme != "petal" || app.settings.ThemePicker != 0 {
		t.Fatal("Ghostty choice did not update the dark slot and close the picker")
	}
	app.settings.ThemePicker = 1
	app.settings.ThemeSearch = "catppuccin latte"
	tt.Frame()
	tt.Key(0, myui.KeyEnter)
	if app.settings.LightTheme != "Catppuccin Latte" || app.settings.ThemePicker != 0 {
		t.Fatal("Return did not choose the filtered theme")
	}
}

// TestThemeChangeKeepsMainWindowClickable follows the Settings flow with
// the full frame, including the theme's decorative background layers.
func TestThemeChangeKeepsMainWindowClickable(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(app.View, 1280, 900)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if app.overlay != "settings" {
		t.Fatal("Settings did not open")
	}
	if err := tt.Click("Petal, light windows"); err != nil {
		t.Fatal(err)
	}
	if app.currentThemeName() != "petal" {
		t.Fatal("the selected theme did not apply")
	}
	tt.Key(0, myui.KeyEscape)
	if app.overlay != "" {
		t.Fatal("Settings did not close")
	}
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if app.overlay != "settings" {
		t.Fatal("main window stopped responding to clicks after changing theme")
	}
}

// TestSavedThemeKeepsMainWindowClickable checks a fresh app loading the
// saved choice, including Neutral's fully transparent glow in light mode.
func TestSavedThemeKeepsMainWindowClickable(t *testing.T) {
	RegisterFonts()
	for _, choice := range []struct {
		name string
		dark bool
	}{
		{"petal", false},
		{"dusk", true},
		{"neutral", false},
		{"Catppuccin Latte", false},
		{"Catppuccin Mocha", true},
		{"plain", true},
	} {
		t.Run(choice.name, func(t *testing.T) {
			t.Setenv("ROVE_HOME", t.TempDir())
			first := NewApp(nil, ThemeByName("graphite"))
			first.LoadSettings()
			first.chooseTheme(choice.name, choice.dark)
			first.saveSettings()

			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.LoadSettings()
			app.SetSystemAppearance(choice.dark)
			if app.currentThemeName() != choice.name {
				t.Fatalf("loaded theme = %q, want %q", app.currentThemeName(), choice.name)
			}
			tt := myui.NewTester(app.View, 1280, 900)
			tt.Frame()
			// The separately drawn divider still worked in the frozen UI.
			listHeight := app.H - 46 - sidebarFooterHeight - 1
			start := 46 + listHeight*app.splitRatio
			before := app.splitRatio
			tt.Press(120, start)
			tt.Move(120, start+40)
			tt.Release(120, start+40)
			if app.splitRatio <= before {
				t.Fatal("spaces/agents divider did not drag")
			}
			if err := tt.Click("Settings"); err != nil {
				t.Fatal(err)
			}
			if app.overlay != "settings" {
				t.Fatal("saved theme prevents Settings from opening after restart")
			}
			tt.Key(0, myui.KeyEscape)
			if err := tt.Click("Files"); err != nil {
				t.Fatal(err)
			}
			if app.panel != "files" {
				t.Fatal("saved theme prevents the Files panel from opening")
			}
			if dir := os.Getenv("THEME_SHOT_DIR"); dir != "" {
				saveThemeShot(t, tt, filepath.Join(dir, choice.name+"-interactive.png"))
			}
		})
	}
}

func TestThemePickerInTheSettingsViewport(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.OpenOverlay("settings")
	tt := myui.NewTester(app.View, 1280, 900)
	tt.Scroll(900, 650, 0, 480)
	if err := tt.Click("Meadow, dark windows"); err != nil {
		t.Fatal(err)
	}
	if app.settings.DarkTheme != "meadow" {
		t.Fatal("dark tile was not reachable by scrolling")
	}
	app.settings.ThemePicker = 2
	app.settings.ThemeSearch = "catppuccin"
	tt.Frame()
	tt.Key(0, myui.KeyEscape)
	if app.settings.ThemePicker != 0 || app.overlay != "settings" {
		t.Fatal("Escape should close the theme picker and keep Settings open")
	}
}

// TestKeroThemeShots renders the actual app without changing the running core.
func TestKeroThemeShots(t *testing.T) {
	dir := os.Getenv("THEME_SHOT_DIR")
	if dir == "" {
		t.Skip("set THEME_SHOT_DIR")
	}
	RegisterFonts()
	for _, shot := range []struct {
		name, theme string
		dark        bool
	}{
		{"petal-light", "petal", false},
		{"dusk-dark", "dusk", true},
		{"ghostty-dark", "Catppuccin Mocha", true},
	} {
		app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
		app.settings.LightTheme, app.settings.DarkTheme = shot.theme, shot.theme
		app.SetSystemAppearance(shot.dark)
		app.OpenOverlay("settings")
		tt := myui.NewTester(app.View, 1280, 900)
		tt.SetScale(1)
		tt.Frame()
		saveThemeShot(t, tt, filepath.Join(dir, shot.name+".png"))
		tt.Scroll(900, 650, 0, 480)
		saveThemeShot(t, tt, filepath.Join(dir, shot.name+"-scrolled.png"))
		app.CloseOverlay()
		tt.Frame()
		saveThemeShot(t, tt, filepath.Join(dir, shot.name+"-window.png"))
	}
	app := NewApp(nil, ThemeByName("graphite"))
	app.settings.ThemePicker, app.settings.ThemeSearch = 2, "catppuccin"
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(760).Background(app.T.Surface).Children(func() { app.settingsThemeCard(c) })
	}, 800, 1400)
	saveThemeShot(t, tt, filepath.Join(dir, "picker.png"))
}

func saveThemeShot(t *testing.T, tt *myui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
