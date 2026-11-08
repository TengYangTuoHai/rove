package ui

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"rove/internal/assets"
	"rove/internal/herdr"
)

// Binding is one key in keys.json: its section, its key and what it does.
type Binding struct {
	Section string `json:"section"`
	Keys    string `json:"keys"`
	Action  string `json:"action"`
}

// AppHome is the folder the app keeps its own files in: settings.json,
// keys.json and view.json, never herdr-app's.
func AppHome() string { return herdr.AppHome() }

var (
	bindings     []Binding
	bindingsOnce bool
)

// Bindings returns the key bindings, from keys.json when the person has
// one, else the ones Herdr ships.
func Bindings() []Binding {
	if bindingsOnce {
		return bindings
	}
	bindingsOnce = true
	text, err := os.ReadFile(filepath.Join(AppHome(), "keys.json"))
	if err != nil {
		name := "keys.json"
		if runtime.GOOS == "darwin" {
			name = "keys.macos.json"
		}
		text, _ = fs.ReadFile(assets.Files, name)
	}
	var doc map[string]map[string]string
	if json.Unmarshal(text, &doc) != nil {
		return nil
	}
	sections := make([]string, 0, len(doc))
	for s := range doc {
		sections = append(sections, s)
	}
	for _, section := range []string{"global", "layout:main"} {
		keys := doc[section]
		if len(keys) == 0 {
			continue
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			bindings = append(bindings, Binding{Section: section, Keys: k, Action: keys[k]})
		}
	}
	return bindings
}

// KeyFor returns the keys bound to an action in the global section.
func KeyFor(action string) string {
	for _, b := range Bindings() {
		if b.Section == "global" && b.Action == action {
			return b.Keys
		}
	}
	return ""
}

// keyName maps a key of keys.json to MyGo's key.
func keyName(s string) (ui.Key, bool) {
	switch s {
	case "enter", "return":
		return ui.KeyEnter, true
	case "esc", "escape":
		return ui.KeyEscape, true
	case "tab":
		return ui.KeyTab, true
	case "space":
		return ui.KeySpace, true
	case "backspace":
		return ui.KeyBackspace, true
	case "delete":
		return ui.KeyDelete, true
	case "insert":
		return ui.KeyInsert, true
	case "home":
		return ui.KeyHome, true
	case "end":
		return ui.KeyEnd, true
	case "pageup":
		return ui.KeyPageUp, true
	case "pagedown":
		return ui.KeyPageDown, true
	case "left":
		return ui.KeyLeft, true
	case "right":
		return ui.KeyRight, true
	case "up":
		return ui.KeyUp, true
	case "down":
		return ui.KeyDown, true
	case "minus", "-":
		return ui.KeyMinus, true
	case "equal", "=":
		return ui.KeyEqual, true
	case "comma", ",":
		return ui.KeyComma, true
	case "period", ".":
		return ui.KeyPeriod, true
	case "slash", "/":
		return ui.KeySlash, true
	case "backslash", "\\":
		return ui.KeyBackslash, true
	case "semicolon", ";":
		return ui.KeySemicolon, true
	case "quote", "'":
		return ui.KeyQuote, true
	case "bracketleft", "[":
		return ui.KeyBracketLeft, true
	case "bracketright", "]":
		return ui.KeyBracketRight, true
	case "backquote", "`":
		return ui.KeyBackquote, true
	}
	if len(s) == 1 && s[0] >= 'a' && s[0] <= 'z' {
		return ui.KeyA + ui.Key(s[0]-'a'), true
	}
	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
		return ui.Key0 + ui.Key(s[0]-'0'), true
	}
	if len(s) >= 2 && s[0] == 'f' {
		n := 0
		for _, r := range s[1:] {
			if r < '0' || r > '9' {
				return 0, false
			}
			n = n*10 + int(r-'0')
		}
		if n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1), true
		}
	}
	return 0, false
}

// parseBinding splits "cmd-shift-p" into its modifiers and key.
func parseBinding(keys string) (ui.Modifiers, ui.Key, bool) {
	keys = strings.ToLower(strings.TrimSpace(keys))
	var mods ui.Modifiers
	for {
		cut := strings.Index(keys, "-")
		if cut < 0 {
			break
		}
		name := keys[:cut]
		rest := keys[cut+1:]
		switch name {
		case "cmd", "command":
			mods |= ui.Cmd
		case "super":
			mods |= ui.Super
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "alt", "option", "opt":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		default:
			goto done
		}
		keys = rest
	}
done:
	k, ok := keyName(keys)
	return mods, k, ok
}

// keys handles the app's own shortcuts; keys the terminal takes stay with it.
func (a *App) keys(c *ui.Context) {
	for _, b := range Bindings() {
		mods, key, ok := parseBinding(b.Keys)
		if !ok {
			continue
		}
		if c.Shortcut(mods, key) {
			a.Run(b.Action)
		}
	}
}

// keyHint writes a binding the way Herdr shows it: ⌘⇧P on macOS.
func keyHint(action string) string {
	return prettyKeys(KeyFor(action))
}

// prettyKeys turns "cmd-shift-p" into "⌘⇧P" on macOS and "Ctrl+Shift+P" elsewhere.
func prettyKeys(keys string) string {
	if keys == "" {
		return ""
	}
	parts := strings.Split(keys, "-")
	if strings.HasSuffix(keys, "--") {
		parts = append(strings.Split(strings.TrimSuffix(keys, "--"), "-"), "-")
	}
	mac := runtime.GOOS == "darwin"
	var out []string
	for i, p := range parts {
		last := i == len(parts)-1
		switch p {
		case "cmd", "command":
			if mac {
				out = append(out, "⌘")
			} else {
				out = append(out, "Ctrl")
			}
		case "super":
			if mac {
				out = append(out, "⌘")
			} else {
				out = append(out, "Super")
			}
		case "ctrl":
			if mac {
				out = append(out, "⌃")
			} else {
				out = append(out, "Ctrl")
			}
		case "alt", "option":
			if mac {
				out = append(out, "⌥")
			} else {
				out = append(out, "Alt")
			}
		case "shift":
			if mac {
				out = append(out, "⇧")
			} else {
				out = append(out, "Shift")
			}
		default:
			if last {
				out = append(out, strings.ToUpper(p))
			} else {
				out = append(out, strings.ToUpper(p))
			}
		}
	}
	if mac {
		return strings.Join(out, "")
	}
	return strings.Join(out, "+")
}
