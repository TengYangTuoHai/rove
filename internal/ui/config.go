package ui

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// ConfigPath is the settings file, in the app's own folder: an empty
// string when there is nowhere to keep it.
func ConfigPath() string {
	dir := AppHome()
	if dir == "" || dir == "." {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

// settingsFile is what the app keeps between launches of settingsState: the
// search box and the section open are not settings.
type settingsFile struct {
	Language        string          `json:"language"`
	Appearance      int             `json:"appearance"`
	Theme           string          `json:"theme"`
	LightTheme      string          `json:"light_theme,omitempty"`
	DarkTheme       string          `json:"dark_theme,omitempty"`
	Accent          string          `json:"accent"`
	InterfaceFont   string          `json:"interface_font"`
	InterfaceSize   int             `json:"interface_size"`
	TerminalFont    string          `json:"terminal_font"`
	TerminalSize    int             `json:"terminal_size"`
	Zoom            float32         `json:"zoom"`
	OptionAsAlt     int             `json:"option_as_alt"`
	Clipboard       bool            `json:"clipboard"`
	MinContrast     float64         `json:"minimum_contrast"`
	AlertDone       bool            `json:"alert_done"`
	AlertNeeds      bool            `json:"alert_needs"`
	AlertSound      bool            `json:"alert_sound"`
	DoneSound       string          `json:"done_sound"`
	NeedsSound      string          `json:"needs_sound"`
	MutedAgents     map[string]bool `json:"muted_agents,omitempty"`
	UpdateChannel   int             `json:"update_channel"`
	AutoInstall     bool            `json:"auto_install"`
	BackgroundCheck bool            `json:"background_check"`
	Telemetry       bool            `json:"telemetry"`

	SidebarOpen     bool            `json:"sidebar_open"`
	SidebarWidth    float32         `json:"sidebar_width"`
	AgentsCollapsed bool            `json:"agents_collapsed"`
	FoldedMachines  map[string]bool `json:"folded_machines"`
}

// file is the state as the settings file keeps it.
func (s settingsState) file() settingsFile {
	return settingsFile{
		Language:   s.Language,
		Appearance: s.Appearance, Theme: s.Theme, Accent: s.Accent,
		LightTheme: s.LightTheme, DarkTheme: s.DarkTheme,
		InterfaceFont: s.InterfaceFont, InterfaceSize: s.InterfaceSize,
		TerminalFont: s.TerminalFont, TerminalSize: s.TerminalSize, Zoom: s.Zoom,
		OptionAsAlt: s.OptionAsAlt, Clipboard: s.Clipboard,
		MinContrast: s.MinContrast,
		AlertDone:   s.AlertDone, AlertNeeds: s.AlertNeeds, AlertSound: s.AlertSound,
		DoneSound: s.DoneSound, NeedsSound: s.NeedsSound, MutedAgents: s.MutedAgents,
		UpdateChannel: s.UpdateChannel, AutoInstall: s.AutoInstall,
		BackgroundCheck: s.BackgroundCheck, Telemetry: s.Telemetry,
		SidebarOpen: s.SidebarOpen, SidebarWidth: s.SidebarWidth,
		AgentsCollapsed: s.AgentsCollapsed, FoldedMachines: s.FoldedMachines,
	}
}

// readSettings applies the settings a file keeps to s, leaving what the
// file does not say at s's own value, as a file written by another version
// leaves the settings it does not know.
func readSettings(data []byte, s settingsState) settingsState {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return s
	}
	take := func(key string, dst any) {
		if raw, ok := fields[key]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	take("language", &s.Language)
	take("appearance", &s.Appearance)
	take("theme", &s.Theme)
	take("light_theme", &s.LightTheme)
	take("dark_theme", &s.DarkTheme)
	take("accent", &s.Accent)
	take("interface_font", &s.InterfaceFont)
	take("interface_size", &s.InterfaceSize)
	take("terminal_font", &s.TerminalFont)
	take("terminal_size", &s.TerminalSize)
	take("zoom", &s.Zoom)
	take("option_as_alt", &s.OptionAsAlt)
	take("clipboard", &s.Clipboard)
	take("minimum_contrast", &s.MinContrast)
	take("alert_done", &s.AlertDone)
	take("alert_needs", &s.AlertNeeds)
	take("alert_sound", &s.AlertSound)
	take("done_sound", &s.DoneSound)
	take("needs_sound", &s.NeedsSound)
	take("muted_agents", &s.MutedAgents)
	take("update_channel", &s.UpdateChannel)
	take("auto_install", &s.AutoInstall)
	take("background_check", &s.BackgroundCheck)
	take("telemetry", &s.Telemetry)
	take("sidebar_open", &s.SidebarOpen)
	take("sidebar_width", &s.SidebarWidth)
	take("agents_collapsed", &s.AgentsCollapsed)
	take("folded_machines", &s.FoldedMachines)
	return s.sanitized()
}

// sanitized brings what a file may say back to what the window can draw.
func (s settingsState) sanitized() settingsState {
	if !validLanguage(s.Language) {
		s.Language = "system"
	}
	if s.Appearance < AppearanceSystem || s.Appearance > AppearanceDark {
		s.Appearance = AppearanceSystem
	}
	if !knownTheme(s.Theme) {
		s.Theme = "graphite"
	}
	if s.LightTheme != "" && !knownTheme(s.LightTheme) {
		s.LightTheme = "paper"
	}
	if s.DarkTheme != "" && !knownTheme(s.DarkTheme) {
		s.DarkTheme = "graphite"
	}
	if s.Accent != "" && !validHex(s.Accent) {
		s.Accent = defaultSettings().Accent
	}
	if strings.TrimSpace(s.InterfaceFont) == "" {
		s.InterfaceFont = UIFont
	}
	if strings.TrimSpace(s.TerminalFont) == "" {
		s.TerminalFont = TerminalFont
	}
	s.InterfaceSize = clampInt(s.InterfaceSize, 8, 40)
	s.TerminalSize = clampInt(s.TerminalSize, 8, 40)
	if s.Zoom <= 0 {
		s.Zoom = 1
	}
	s.Zoom = min(max(s.Zoom, 0.5), 3)
	if s.MinContrast < 1 {
		s.MinContrast = 1
	}
	if s.MinContrast > 21 {
		s.MinContrast = 21
	}
	if s.OptionAsAlt < 0 || s.OptionAsAlt >= len(optionAltNames) {
		s.OptionAsAlt = 0
	}
	if s.UpdateChannel < 0 || s.UpdateChannel >= len(updateChannelNames) {
		s.UpdateChannel = 0
	}
	if s.SidebarWidth <= 0 {
		s.SidebarWidth = 268
	}
	s.SidebarWidth = min(max(s.SidebarWidth, sidebarMinWidth), sidebarMaxWidth)
	if !validAlertSound(s.DoneSound) {
		s.DoneSound = "Glass"
	}
	if !validAlertSound(s.NeedsSound) {
		s.NeedsSound = "Ping"
	}
	if s.MutedAgents == nil {
		s.MutedAgents = map[string]bool{}
	}
	if s.FoldedMachines == nil {
		s.FoldedMachines = map[string]bool{}
	}
	return s
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// validHex reports whether s is a #rrggbb color, as ui.Hex takes one.
func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// LoadSettings reads the settings file the last launch wrote, or keeps the
// defaults when there is none, and remembers where the next change goes.
func (a *App) LoadSettings() {
	a.settingsPath = ConfigPath()
	if a.settingsPath == "" {
		return
	}
	if data, err := os.ReadFile(a.settingsPath); err == nil {
		a.settings = readSettings(data, a.settings)
		// What the file holds now, as this version would write it: a
		// launch that changes nothing writes nothing. A first launch,
		// with no file, writes the defaults.
		if canonical, err := settingsJSON(a.settings); err == nil {
			a.savedSettings = string(canonical)
			a.rememberSettings(a.settings.file())
		}
	}
	a.setLanguage(a.settings.Language)
	a.Zoom = a.settings.Zoom
	a.applyTerminalFont()
	a.applyTerminalPreferences()
	a.SidebarOpen = a.settings.SidebarOpen
	a.SidebarWidth = a.settings.SidebarWidth
	a.agentsCollapsed = a.settings.AgentsCollapsed
	a.foldedMachines = a.settings.FoldedMachines
	a.sidebarShown = a.SidebarWidth
	if !a.SidebarOpen {
		a.sidebarShown = 0
	}
}

// saveSettings writes the settings when they changed since the last write,
// as every frame that changed one calls it and the rest do nothing.
func (a *App) saveSettings() {
	a.applyTerminalPreferences()
	a.settings.Zoom = a.Zoom
	if a.settingsPath == "" {
		return
	}
	current := a.settings.file()
	if a.savedSettingsFile != nil && reflect.DeepEqual(&current, a.savedSettingsFile) {
		return
	}
	data, err := settingsJSON(a.settings)
	if err != nil {
		return
	}
	if string(data) != a.savedSettings {
		if err := writeFileAtomic(a.settingsPath, data); err != nil {
			return
		}
		a.savedSettings = string(data)
	}
	a.rememberSettings(current)
}

// rememberSettings copies maps because settings controls mutate them in place.
func (a *App) rememberSettings(current settingsFile) {
	current.MutedAgents = maps.Clone(current.MutedAgents)
	current.FoldedMachines = maps.Clone(current.FoldedMachines)
	a.savedSettingsFile = &current
}

// settingsJSON is the file's text for the settings.
func settingsJSON(s settingsState) ([]byte, error) {
	data, err := json.MarshalIndent(s.file(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, byte(10)), nil
}

// writeFileAtomic replaces path with data, so a launch reading it while it
// is written sees either the old file or the new one.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// SettingsPath is the file the Look section names, with the home folder as
// ~ so it reads short.
func (a *App) SettingsPath() string {
	path := a.settingsPath
	if path == "" {
		path = ConfigPath()
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
