package ui

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestSettingsShowsBuildVersion(t *testing.T) {
	RegisterFonts()
	previous := mygo.App.Version()
	t.Cleanup(func() { mygo.App.SetVersion(previous) })
	for _, version := range []string{"", "0.2.0", "0.2.0-dev.42.gabcdef1"} {
		mygo.App.SetVersion(version)
		app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
		app.Run("app.settings:look")
		tt := myui.NewTester(app.View, 1280, 820)
		tt.Frame()
		want := version
		if want == "" {
			want = "dev"
		}
		if !tt.HasText(want) {
			t.Errorf("settings did not show build version %q", want)
		}
		if path := os.Getenv("SETTINGS_VERSION_SHOT"); path != "" && version == "0.2.0-dev.42.gabcdef1" {
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
	}
}

func TestDefaultSettings(t *testing.T) {
	s := defaultSettings()
	if s.Section != "look" {
		t.Fatalf("section = %q, want look", s.Section)
	}
	if s.Theme != "graphite" {
		t.Fatalf("theme = %q, want graphite", s.Theme)
	}
	if s.InterfaceSize != 13 || s.TerminalSize != 14 {
		t.Fatalf("font sizes = %d/%d, want 13/14", s.InterfaceSize, s.TerminalSize)
	}
	if !s.Clipboard || !s.AlertDone || !s.AlertNeeds || !s.AlertSound || !s.Telemetry {
		t.Fatalf("defaults should turn clipboard, alerts and telemetry on: %+v", s)
	}
}

func TestSettingsSearchOmitsUnimplementedSections(t *testing.T) {
	app := NewApp(nil, ThemeByName("graphite"))
	for _, query := range []string{"", "updates", "privacy", "channel", "automatic", "background", "herdr-core", "telemetry", "reports"} {
		app.settings.Search = query
		for _, result := range app.settingsResults() {
			if result.Section == "updates" || result.Section == "privacy" {
				t.Errorf("search %q advertised hidden setting %q", query, result.Title)
			}
		}
	}
}

func TestSettingsHidesUnimplementedSections(t *testing.T) {
	RegisterFonts()
	for _, section := range []string{"look", "terminal", "machines", "alerts", "updates", "privacy"} {
		t.Run(section, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.Run("app.settings:" + section)
			tt := myui.NewTester(app.View, 1280, 820)
			tt.Frame()
			for _, label := range []string{"Look", "Terminal", "Machines", "Alerts"} {
				if _, ok := tt.Find(label); !ok {
					t.Errorf("active section %q is missing", label)
				}
			}
			for _, label := range []string{"Updates", "Privacy", "Channel", "Install automatically", "Check in the background", "Check now", "View notes  ›", "Anonymous usage counts", "View the report", "What would be sent", "Copy"} {
				if _, ok := tt.Find(label); ok {
					t.Errorf("placeholder %q is still visible", label)
				}
			}
			want := section
			if section == "updates" || section == "privacy" {
				want = "look"
			}
			if app.settings.Section != want {
				t.Errorf("selected section = %q, want %q", app.settings.Section, want)
			}
			body := map[string]string{
				"look": "Appearance", "terminal": "Clipboard",
				"machines": "Add machine", "alerts": "Done",
			}[want]
			if _, ok := tt.Find(body); !ok {
				t.Errorf("section content %q is missing", body)
			}
		})
	}
}

func TestSettingsResultsMatchThemeFirst(t *testing.T) {
	a := &App{T: ThemeByName("graphite")}
	a.settings = defaultSettings()
	a.Zoom = 1
	a.settings.Search = "theme"

	results := a.settingsResults()
	if len(results) == 0 {
		t.Fatal("no results for theme")
	}
	if results[0].Title != "Theme" {
		t.Fatalf("first result = %q, want Theme", results[0].Title)
	}
}

func TestSettingsResultsEmptyQueryListsEveryEntry(t *testing.T) {
	a := &App{T: ThemeByName("graphite")}
	a.settings = defaultSettings()
	a.Zoom = 1

	if got, want := len(a.settingsResults()), len(a.settingsEntries()); got != want {
		t.Fatalf("results = %d, want %d", got, want)
	}
}

func TestChooseSettingsResultOpensItsSection(t *testing.T) {
	a := &App{T: ThemeByName("graphite")}
	a.settings = defaultSettings()
	a.Zoom = 1
	a.settings.Search = "sound"
	a.settings.SearchOpen = true

	at := -1
	for i, r := range a.settingsResults() {
		if r.Section == "alerts" {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("no alerts result for sound")
	}
	a.chooseSettingsResult(at)
	if a.settings.Section != "alerts" {
		t.Fatalf("section = %q, want alerts", a.settings.Section)
	}
	if a.settings.SearchOpen {
		t.Fatal("search should close after choosing")
	}
}

func TestSettingsMoveStopsAtEnds(t *testing.T) {
	a := &App{T: ThemeByName("graphite")}
	a.settings = defaultSettings()
	a.Zoom = 1
	a.settings.SearchAt = 0

	a.settingsMove(-1)
	if a.settings.SearchAt != 0 {
		t.Fatalf("moved above the first result to %d", a.settings.SearchAt)
	}
	a.settingsMove(1)
	if a.settings.SearchAt != 1 {
		t.Fatalf("moved to %d, want 1", a.settings.SearchAt)
	}
	results := len(a.settingsResults())
	a.settings.SearchAt = results - 1
	a.settingsMove(1)
	if a.settings.SearchAt != results-1 {
		t.Fatalf("moved past the last result to %d", a.settings.SearchAt)
	}
}

// TestSettingsNavRowsKeepTheirGap draws the sections list: its rows sit 2 px
// apart, as the sidebar's do, so a row's hover or selection never runs into
// the row above or below it.
func TestSettingsNavRowsKeepTheirGap(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(settingsNavWidth).Height(420).Children(func() { app.settingsNav(c) })
	}, 400, 500)
	tt.Frame()
	look, ok := tt.Find("Look")
	if !ok {
		t.Fatal("the Look row is not drawn")
	}
	terminal, ok := tt.Find("Terminal")
	if !ok {
		t.Fatal("the Terminal row is not drawn")
	}
	if got := terminal.X - look.X; got != 0 {
		t.Errorf("the labels start %v apart, want them in one column", got)
	}
	if got := terminal.Y - look.Y; got != 36 {
		t.Errorf("the rows are %v apart, want 36: 34 px rows with the 2 px gap", got)
	}
}
