package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestSettingsPersistBetweenLaunches keeps the theme and the appearance in
// the app's own file, and reads them back on the next launch.
func TestSettingsPersistBetweenLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	first.settings.Theme = "paper"
	first.settings.Appearance = AppearanceLight
	first.settings.AlertSound = false
	first.saveSettings()

	path := filepath.Join(dir, "rove", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no settings file: %v", err)
	}
	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if second.settings.Theme != "paper" {
		t.Errorf("theme = %q, want paper", second.settings.Theme)
	}
	if second.settings.Appearance != AppearanceLight {
		t.Errorf("appearance = %d, want light", second.settings.Appearance)
	}
	if second.settings.AlertSound {
		t.Error("alert sound came back on")
	}
	// The theme the file kept is the one the window then draws.
	second.applyTheme()
	if second.currentThemeName() != "paper" || second.T.Dark {
		t.Fatalf("second launch drew %q (dark %v), want paper", second.currentThemeName(), second.T.Dark)
	}
}

// TestLoadSettingsWritesNothingUntilAChange starts from a file the app
// wrote, and leaves it alone.
func TestLoadSettingsWritesNothingUntilAChange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	first.settings.Theme = "moss"
	first.saveSettings()

	path := filepath.Join(dir, "rove", "settings.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	second.saveSettings() // nothing changed

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an unchanged launch rewrote the settings")
	}
}

// TestReadSettingsKeepsDefaultsForMissingKeys lets a file written by
// another version keep the settings it does not name.
func TestReadSettingsKeepsDefaultsForMissingKeys(t *testing.T) {
	s := readSettings([]byte(`{"theme":"ink"}`), defaultSettings())
	if s.Theme != "ink" {
		t.Errorf("theme = %q, want ink", s.Theme)
	}
	if !s.Clipboard || !s.AlertDone || !s.AlertNeeds || !s.AlertSound || !s.Telemetry {
		t.Errorf("missing keys turned settings off: %+v", s)
	}
}

// TestReadSettingsIgnoresWhatItCannotDraw keeps a hand-edited file from
// making the window draw a theme or a size it cannot.
func TestReadSettingsIgnoresWhatItCannotDraw(t *testing.T) {
	data := []byte(`{"theme":"nope","appearance":9,"accent":"red",
		"minimum_contrast":0,"interface_size":999,"option_as_alt":7,
		"update_channel":-2}`)
	s := readSettings(data, defaultSettings())
	if s.Theme != "graphite" {
		t.Errorf("theme = %q, want graphite", s.Theme)
	}
	if s.Appearance != AppearanceSystem {
		t.Errorf("appearance = %d, want system", s.Appearance)
	}
	if s.Accent != defaultSettings().Accent {
		t.Errorf("accent = %q, want the default", s.Accent)
	}
	if s.MinContrast != 1 {
		t.Errorf("minimum contrast = %v, want 1", s.MinContrast)
	}
	if s.InterfaceSize != 40 {
		t.Errorf("interface size = %d, want 40", s.InterfaceSize)
	}
	if s.OptionAsAlt != 0 || s.UpdateChannel != 0 {
		t.Errorf("choices = %d/%d, want 0/0", s.OptionAsAlt, s.UpdateChannel)
	}
}

// TestSaveSettingsWithoutAFileDoesNothing keeps a window made without
// settings, as a preview's, from writing one.
func TestSaveSettingsWithoutAFileDoesNothing(t *testing.T) {
	app := NewApp(nil, ThemeByName("graphite"))
	app.settings.Theme = "paper"
	app.saveSettings()
	if app.savedSettings != "" {
		t.Fatal("a window without a settings file wrote one")
	}
}

// TestViewSavesAsAChangeHappens writes the settings from the frame that
// drew the change, as the Look section promises.
func TestViewSavesAsAChangeHappens(t *testing.T) {
	RegisterFonts()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "rove", "settings.json")

	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.LoadSettings()
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the first frame wrote no settings: %v", err)
	}

	app.settings.Theme = "moss"
	app.settings.Appearance = AppearanceDark
	tt.Frame()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file settingsFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.Theme != "moss" || file.Appearance != AppearanceDark {
		t.Fatalf("file = %+v, want moss and dark", file)
	}
}

// TestConfigPathIsTheAppsOwn names the app's own file, never herdr-app's.
func TestConfigPathIsTheAppsOwn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got, want := ConfigPath(), filepath.Join(dir, "rove", "settings.json"); got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}
