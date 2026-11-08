package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"herdr-mygo/internal/herdr"
)

func isolateShortcutSettings(t *testing.T) {
	t.Helper()
	t.Setenv("HERDR_MYGO_HOME", t.TempDir())
	oldBindings, oldOnce := bindings, bindingsOnce
	bindings, bindingsOnce = nil, false
	t.Cleanup(func() { bindings, bindingsOnce = oldBindings, oldOnce })
}

func TestShortcutSettingsPersistsAndRejectsConflicts(t *testing.T) {
	isolateShortcutSettings(t)
	if runtime.GOOS != "linux" {
		t.Skip("Linux default shortcuts")
	}
	if err := changeKey("global", "pane.split:right", "super-d", "super-shift-x"); err != nil {
		t.Fatal(err)
	}
	if got := KeyFor("pane.split:right"); got != "ctrl-shift-o" && got != "super-shift-x" {
		t.Fatalf("binding = %q", got)
	}
	if err := changeKey("global", "pane.split:right", "super-shift-x", "ctrl-shift-e"); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("conflict = %v", err)
	}
	bindings, bindingsOnce = nil, false
	doc, err := readKeyDocument()
	if err != nil {
		t.Fatal(err)
	}
	if doc["global"]["super-shift-x"] != "pane.split:right" || doc["global"]["super-d"] != "" || doc["terminal"]["ctrl-shift-c"] != "terminal.copy" {
		t.Fatalf("saved document lost bindings: %#v", doc)
	}
	if err := changeKey("global", "pane.split:right", "super-shift-x", ""); err != nil {
		t.Fatal(err)
	}
	if err := changeKey("global", "pane.split:right", "", "super-d"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(AppHome(), "keys.json"))
	if err != nil || !json.Valid(data) {
		t.Fatalf("saved json: %v", err)
	}
}

func TestShortcutSettingsRecording(t *testing.T) {
	isolateShortcutSettings(t)
	if runtime.GOOS != "linux" {
		t.Skip("Linux default shortcuts")
	}
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.Run("app.settings:keys")
	app.settings.KeySearch = "split right"
	tt := myui.NewTester(app.View, 1280, 820)
	tt.SetFocused(true)
	tt.Frame()
	if !tt.HasText("Keyboard shortcuts") || !tt.HasText("Split right") {
		t.Fatalf("shortcut settings missing: %v", tt.Texts())
	}
	if err := tt.Click("Super+D"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Press shortcut…") {
		t.Fatalf("recorder did not open: action=%q section=%q", app.settings.KeyAction, app.settings.Section)
	}
	tt.Key(myui.Ctrl|myui.Shift, myui.KeyE)
	tt.Frame()
	if app.settings.KeyAction == "" || !strings.Contains(app.settings.KeyError, "already in use") {
		t.Fatalf("conflicting key was accepted: %q", app.settings.KeyError)
	}
	tt.Key(myui.Super|myui.Shift, myui.KeyX)
	tt.Frame()
	if app.settings.KeyAction != "" {
		t.Fatalf("recorder remained open: %s", app.settings.KeyError)
	}
	doc, err := readKeyDocument()
	if err != nil {
		t.Fatal(err)
	}
	if doc["global"]["super-shift-x"] != "pane.split:right" {
		t.Fatalf("recording was not saved: %v", doc["global"])
	}
	if err := tt.Click("Super+Shift+X"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, myui.KeyEscape)
	tt.Frame()
	if app.settings.KeyAction != "" || app.overlay != "settings" {
		t.Fatal("Escape should cancel recording without closing settings")
	}
}
