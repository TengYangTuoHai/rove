package ui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"herdr-mygo/internal/assets"
)

type keyDocument map[string]map[string]string

func defaultKeyDocument() (keyDocument, error) {
	name := "keys.json"
	if runtime.GOOS == "darwin" {
		name = "keys.macos.json"
	}
	data, err := fs.ReadFile(assets.Files, name)
	if err != nil {
		return nil, err
	}
	var doc keyDocument
	err = json.Unmarshal(data, &doc)
	return doc, err
}

// readKeyDocument preserves terminal and other sections when an app key changes.
func readKeyDocument() (keyDocument, error) {
	data, err := os.ReadFile(filepath.Join(AppHome(), "keys.json"))
	if os.IsNotExist(err) {
		return defaultKeyDocument()
	}
	if err != nil {
		return nil, err
	}
	var doc keyDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("empty shortcut configuration")
	}
	return doc, nil
}

type keyConflict struct{ action string }

func (e keyConflict) Error() string { return "shortcut already used by " + e.action }

// changeKey saves first, then updates the in-memory bindings used by the app.
// Conflicts are checked by actual modifier/key values, including aliases.
func changeKey(section, action, oldKey, newKey string) error {
	doc, err := readKeyDocument()
	if err != nil {
		return err
	}
	if section != "global" && section != "layout:main" {
		return fmt.Errorf("unsupported shortcut section")
	}
	if doc[section] == nil {
		doc[section] = map[string]string{}
	}
	if oldKey != "" && doc[section][oldKey] != action {
		return fmt.Errorf("shortcut changed; reopen settings")
	}
	if newKey != "" {
		mods, key, ok := parseBinding(newKey)
		if !ok {
			return fmt.Errorf("unsupported shortcut")
		}
		for sec, entries := range doc {
			if sec != "global" && sec != "layout:main" && sec != "terminal" {
				continue
			}
			for name, other := range entries {
				if sec == section && name == oldKey {
					continue
				}
				m, k, valid := parseBinding(name)
				if valid && m == mods && k == key {
					return keyConflict{action: other}
				}
			}
		}
	}
	if oldKey != "" {
		delete(doc[section], oldKey)
	}
	if newKey != "" {
		doc[section][newKey] = action
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err = writeFileAtomic(filepath.Join(AppHome(), "keys.json"), append(data, '\n')); err != nil {
		return err
	}
	bindings = nil
	bindingsOnce = false
	Bindings()
	return nil
}

func shortcutTitle(action string) string {
	for _, command := range paletteCommands {
		if command.Action == action {
			return command.Title
		}
	}
	return action
}

type shortcutAction struct{ section, action string }

// Include defaults even after removing their last binding, so they can be added again.
func shortcutActions() []shortcutAction {
	seen := map[shortcutAction]bool{}
	if doc, err := defaultKeyDocument(); err == nil {
		for _, section := range []string{"global", "layout:main"} {
			for _, action := range doc[section] {
				seen[shortcutAction{section, action}] = true
			}
		}
	}
	for _, b := range Bindings() {
		seen[shortcutAction{b.Section, b.Action}] = true
	}
	// The palette also has actions without a default key. Offer those so a
	// shortcut can be added from settings, excluding terminal-owned keys.
	for _, command := range paletteCommands {
		if command.Action == "terminal.copy" || command.Action == "terminal.paste" || command.Action == "layout.switch" || strings.HasPrefix(command.Action, "app.theme:") {
			continue
		}
		if !seen[shortcutAction{"layout:main", command.Action}] {
			seen[shortcutAction{"global", command.Action}] = true
		}
	}
	out := make([]shortcutAction, 0, len(seen))
	for action := range seen {
		out = append(out, action)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := shortcutTitle(out[i].action), shortcutTitle(out[j].action)
		if a == b {
			return out[i].section < out[j].section
		}
		return a < b
	})
	return out
}

// shortcutFromEvent records the actual Super key independently of ui.Cmd,
// which means Control on Linux and Windows.
func shortcutFromEvent(ev ui.InputEvent) (string, bool) {
	key := ev.Key
	var name string
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('a' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		name = fmt.Sprintf("f%d", 1+key-ui.KeyF1)
	default:
		name = map[ui.Key]string{
			ui.KeyEnter: "enter", ui.KeyTab: "tab", ui.KeySpace: "space",
			ui.KeyBackspace: "backspace", ui.KeyDelete: "delete", ui.KeyInsert: "insert",
			ui.KeyHome: "home", ui.KeyEnd: "end", ui.KeyPageUp: "pageup", ui.KeyPageDown: "pagedown",
			ui.KeyLeft: "left", ui.KeyRight: "right", ui.KeyUp: "up", ui.KeyDown: "down",
			ui.KeyMinus: "minus", ui.KeyEqual: "equal", ui.KeyComma: "comma", ui.KeyPeriod: "period",
			ui.KeySlash: "slash", ui.KeySemicolon: "semicolon", ui.KeyQuote: "quote",
			ui.KeyBracketLeft: "bracketleft", ui.KeyBracketRight: "bracketright",
			ui.KeyBackslash: "backslash", ui.KeyBackquote: "backquote",
		}[key]
	}
	if name == "" || (ev.Mods == 0 && (key < ui.KeyF1 || key > ui.KeyF12)) {
		return "", false
	}
	var parts []string
	if ev.Mods&ui.Super != 0 {
		parts = append(parts, "super")
	}
	if ev.Mods&ui.Ctrl != 0 {
		parts = append(parts, "ctrl")
	}
	if ev.Mods&ui.Alt != 0 {
		parts = append(parts, "alt")
	}
	if ev.Mods&ui.Shift != 0 {
		parts = append(parts, "shift")
	}
	return strings.Join(append(parts, name), "-"), true
}

func (a *App) beginKeyCapture(section, action, old string) {
	a.settings.KeySection, a.settings.KeyAction, a.settings.KeyOld = section, action, old
	a.settings.KeyError = ""
	a.Repaint()
}

func (a *App) finishKeyCapture(key string) {
	s := &a.settings
	if err := changeKey(s.KeySection, s.KeyAction, s.KeyOld, key); err != nil {
		if conflict, ok := err.(keyConflict); ok {
			s.KeyError = a.tr("Shortcut already in use") + ": " + a.tr(shortcutTitle(conflict.action))
		} else {
			s.KeyError = err.Error()
		}
	} else {
		s.KeyAction, s.KeyError = "", ""
	}
	a.Repaint()
}

func (a *App) settingsKeys(c *ui.Context) {
	a.settingsHead(c, a.tr("Keyboard shortcuts"), a.tr("Click a shortcut to change it, or add another. Press Escape to cancel."))
	a.settingsNote(c, "terminal", a.tr("These shortcuts control Herdr. Terminal copy and paste use their own keys."))
	search := ui.TextInputBase(c.Key("shortcut-search"), &a.settings.KeySearch).FillWidth().Height(32).
		FontSize(a.fontSize(12)).TextColor(a.T.Foreground).Placeholder(a.tr("Search shortcuts…"))
	if search.Changed() {
		a.settings.KeyAction, a.settings.KeyError = "", ""
	}
	var actions []shortcutAction
	query := strings.ToLower(strings.TrimSpace(a.settings.KeySearch))
	for _, action := range shortcutActions() {
		text := shortcutTitle(action.action) + " " + a.tr(shortcutTitle(action.action)) + " " + action.action
		for _, binding := range Bindings() {
			if binding.Section == action.section && binding.Action == action.action {
				text += " " + binding.Keys + " " + prettyKeys(binding.Keys)
			}
		}
		if query == "" || strings.Contains(strings.ToLower(text), query) {
			actions = append(actions, action)
		}
	}
	if len(actions) == 0 {
		ui.Text(c, a.tr("No shortcuts found.")).FontSize(a.fontSize(12)).TextColor(a.T.Dim)
		return
	}
	a.settingsCard(c, func() {
		for i, action := range actions {
			if i > 0 {
				a.settingsDivider(c)
			}
			title := a.tr(shortcutTitle(action.action))
			a.settingsRow(c, title, "", func() {
				if a.settings.KeyAction == action.action && a.settings.KeySection == action.section {
					capture := ui.Box(c.Key("capture")).Height(28).PaddingX(10).Radius(7).Border(1, a.T.Accent).Center().Focusable().AutoFocus()
					capture.Children(func() { ui.Text(c, a.tr("Press shortcut…")).FontSize(a.fontSize(11.5)).TextColor(a.T.Accent) })
					capture.HandleInput(func(ev ui.InputEvent) bool {
						if ev.Kind != ui.InputKeyDown {
							return false
						}
						if ev.Key == ui.KeyEscape && ev.Mods == 0 {
							a.settings.KeyAction, a.settings.KeyError = "", ""
							a.Repaint()
							return true
						}
						if key, ok := shortcutFromEvent(ev); ok {
							a.finishKeyCapture(key)
						}
						return true
					})
					if a.settings.KeyOld != "" {
						a.settingsButton(c, a.tr("Remove"), false, true, func() { a.finishKeyCapture("") })
					}
					a.settingsButton(c, a.tr("Cancel"), false, true, func() { a.settings.KeyAction = ""; a.Repaint() })
					return
				}
				for _, binding := range Bindings() {
					if binding.Section == action.section && binding.Action == action.action {
						b := binding
						a.settingsButton(c, prettyKeys(b.Keys), false, true, func() { a.beginKeyCapture(b.Section, b.Action, b.Keys) })
					}
				}
				a.settingsButton(c, "+", false, true, func() { a.beginKeyCapture(action.section, action.action, "") })
			})
			if a.settings.KeyAction == action.action && a.settings.KeySection == action.section && a.settings.KeyError != "" {
				ui.Text(c, a.settings.KeyError).Padding(0, 16, 10, 16).FontSize(a.fontSize(11.5)).TextColor(a.T.Dim).Wrap()
			}
		}
	})
}
