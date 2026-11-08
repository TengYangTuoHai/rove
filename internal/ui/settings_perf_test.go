package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveSettingsDetectsMapMutation(t *testing.T) {
	app := NewApp(nil, ThemeByName("graphite"))
	app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	app.saveSettings()
	app.settings.MutedAgents["codex"] = true
	app.saveSettings()
	data, err := os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded := readSettings(data, defaultSettings())
	if !loaded.MutedAgents["codex"] {
		t.Fatal("in-place map change was not saved")
	}
	delete(app.settings.MutedAgents, "codex")
	app.settings.FoldedMachines["server"] = true
	app.saveSettings()
	data, err = os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded = readSettings(data, defaultSettings())
	if loaded.MutedAgents["codex"] || !loaded.FoldedMachines["server"] {
		t.Fatal("map removal or folded machine was not saved")
	}
}
