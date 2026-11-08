package ui

import (
	"os"
	"path/filepath"
	"testing"

	myui "github.com/egoist/mygo/ui"
	"rove/internal/herdr"
)

func TestLanguagePreferencePersistence(t *testing.T) {
	if got := defaultSettings().Language; got != "system" {
		t.Fatalf("default language=%q", got)
	}
	s := readSettings([]byte(`{"language":"ja"}`), defaultSettings())
	data, err := settingsJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSettings(data, defaultSettings()).Language; got != "ja" {
		t.Fatalf("round trip=%q", got)
	}
	if got := readSettings([]byte(`{"language":"fr"}`), defaultSettings()).Language; got != "system" {
		t.Fatalf("invalid preference=%q", got)
	}
	if got := readSettings([]byte(`{"theme":"ink"}`), defaultSettings()).Language; got != "system" {
		t.Fatalf("legacy settings=%q", got)
	}
	t.Setenv("ROVE_HOME", t.TempDir())
	if err := os.WriteFile(ConfigPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp(nil, ThemeByName("graphite"))
	a.LoadSettings()
	if got := a.tr("Settings"); got != "設定" {
		t.Fatalf("loaded locale=%q", got)
	}
}

func TestSystemLanguageRestoresAfterOverride(t *testing.T) {
	a := NewApp(nil, ThemeByName("graphite"))
	a.systemLanguage = "ja"
	a.setLanguage("system")
	if got := a.tr("Settings"); got != "設定" {
		t.Fatal(got)
	}
	a.setLanguage("en")
	if got := a.tr("Settings"); got != "Settings" {
		t.Fatal(got)
	}
	a.setLanguage("system")
	if got := a.tr("Settings"); got != "設定" {
		t.Fatal(got)
	}
}

func TestLocalizationPreservesUserTitles(t *testing.T) {
	RegisterFonts()
	a := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	a.setLanguage("ja")
	agent := &herdr.Agent{Title: "Settings", Status: "blocked"}
	tt := myui.NewTester(func(c *myui.Context) { myui.Column(c).Width(268).Children(func() { a.agentRow(c, agent) }) }, 300, 100)
	tt.Frame()
	if !tt.HasText("Settings") || tt.HasText("設定") {
		t.Fatal("user-assigned agent title was translated")
	}
	if !tt.HasText("対応が必要") {
		t.Fatal("agent status was not localized")
	}
}

func TestLocalizedSearchKeepsActions(t *testing.T) {
	a := NewApp(nil, ThemeByName("graphite"))
	a.setLanguage("ja")
	for _, q := range []string{"設定", "settings"} {
		a.palette.Query = q
		found := false
		for _, row := range a.paletteRows() {
			if row.Action == "app.settings" {
				found = true
				if row.Title != "設定" {
					t.Errorf("localized title=%q", row.Title)
				}
			}
		}
		if !found {
			t.Errorf("no settings command for %q", q)
		}
	}
	for _, q := range []string{"言語", "language"} {
		a.settings.Search = q
		found := false
		for _, row := range a.settingsResults() {
			if row.Title == "言語" && row.Section == "look" {
				found = true
			}
		}
		if !found {
			t.Errorf("no language setting for %q", q)
		}
	}
}

func TestLanguageSwitchRendersImmediately(t *testing.T) {
	RegisterFonts()
	a := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	a.setLanguage("en")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.OpenOverlay("settings")
	tt := myui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if !tt.HasText("Language") {
		t.Fatal("language control missing")
	}
	tt.Click("日本語")
	tt.Frame()
	if !tt.HasText("設定") || !tt.HasText("言語") {
		t.Fatal("Japanese settings did not render")
	}
	if a.settings.Language != "ja" {
		t.Fatalf("language=%q", a.settings.Language)
	}
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := readSettings(data, defaultSettings()).Language; got != "ja" {
		t.Fatalf("saved language=%q", got)
	}
	tt.Click("中文")
	tt.Frame()
	if !tt.HasText("设置") || !tt.HasText("语言") {
		t.Fatal("Chinese settings did not render")
	}
	a.setLanguage("en")
	tt.Frame()
	if !tt.HasText("Settings") {
		t.Fatal("English did not return")
	}
}

func TestLocalizedScreens(t *testing.T) {
	RegisterFonts()
	for _, locale := range []string{"en", "zh", "ja"} {
		for _, surface := range []string{"main", "look", "terminal", "machines", "alerts", "palette", "welcome", "close"} {
			t.Run(locale+"/"+surface, func(t *testing.T) {
				a := NewApp(&herdr.Session{}, ThemeByName("graphite"))
				a.setLanguage(locale)
				switch surface {
				case "main":
				case "look", "terminal", "machines", "alerts":
					a.OpenOverlay("settings")
					a.settings.Section = surface
				case "close":
					a.closing = closeState{kind: "tab", names: []string{"user-program"}}
					a.OpenOverlay("close-confirm")
				default:
					a.OpenOverlay(surface)
				}
				tt := myui.NewTester(a.View, 1280, 820)
				tt.Frame()
				if surface == "main" && !tt.HasText(a.tr("No spaces yet")) {
					t.Fatal("localized empty state missing")
				}
				if surface == "look" && !tt.HasText(a.tr("Language")) {
					t.Fatal("localized language control missing")
				}
				if dir := os.Getenv("LOCALIZATION_EVIDENCE"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					saveThemeShot(t, tt, filepath.Join(dir, locale+"-"+surface+".png"))
				}
			})
		}
	}
}
