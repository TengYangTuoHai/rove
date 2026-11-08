package ui

import (
	"io/fs"
	"os"
	"runtime"
	"strings"
	"time"

	"herdr-mygo/internal/assets"
)

// TerminalFont is the font terminals draw in, registered from the app's own
// files.
const TerminalFont = "JetBrains Mono"

// UIFont is the panels' font.
const UIFont = "Geist"

func osGOOS() string { return runtime.GOOS }

// iconPNG reads a bitmap from the app's assets.
func iconPNG(name string) ([]byte, error) {
	return fs.ReadFile(assets.Files, name)
}

// Toast is a note that shows at the window's bottom right for a moment.
type Toast struct {
	Title string
	Body  string
	Level string
	id    uint64
}

const toastDuration = 5 * time.Second

// toast shows a note, replacing notes of the same title.
func (a *App) toast(title, body, level string) {
	a.mu.Lock()
	out := a.notes[:0]
	for _, n := range a.notes {
		if n.Title != title {
			out = append(out, n)
		}
	}
	a.nextToastID++
	id := a.nextToastID
	a.notes = append(out, Toast{Title: title, Body: body, Level: level, id: id})
	if len(a.notes) > 3 {
		a.notes = a.notes[len(a.notes)-3:]
	}
	a.mu.Unlock()
	a.Repaint()
	time.AfterFunc(toastDuration, func() { a.expireToast(id) })
}

func (a *App) expireToast(id uint64) {
	a.mu.Lock()
	for i, n := range a.notes {
		if n.id == id {
			a.notes = append(a.notes[:i], a.notes[i+1:]...)
			a.mu.Unlock()
			a.Repaint()
			return
		}
	}
	a.mu.Unlock()
}

// PaletteState is the command palette's search and selection.
type PaletteState struct {
	Query string
	At    int
	open  bool
}

// PromptState is a dialog that asks for a name.
type PromptState struct {
	Title   string
	Hint    string
	Value   string
	Command string
	Target  string
}

// settingsState is what the Settings screen shows, and the view the window
// keeps between launches.
type settingsState struct {
	Section       string
	Language      string
	Appearance    int
	Theme         string // legacy single theme; used until an appearance has its own choice
	LightTheme    string
	DarkTheme     string
	ThemeSearch   string
	ThemePicker   int // 0 closed, 1 light, 2 dark
	Accent        string
	InterfaceFont string
	InterfaceSize int
	TerminalFont  string
	TerminalSize  int
	Zoom          float32

	OptionAsAlt int // legacy saved preference; control hidden until MyGo supports it
	Clipboard   bool
	MinContrast float64

	AlertDone   bool
	AlertNeeds  bool
	AlertSound  bool
	DoneSound   string
	NeedsSound  string
	MutedAgents map[string]bool

	UpdateChannel   int
	AutoInstall     bool
	BackgroundCheck bool

	Telemetry bool

	// SidebarOpen and SidebarWidth are the sidebar as it was left: whether
	// it is shown, and the width it opens at. AgentsCollapsed and
	// FoldedMachines are the folds as they were left: the agents list
	// folded to its header, and the machines whose spaces are hidden.
	SidebarOpen     bool
	SidebarWidth    float32
	AgentsCollapsed bool
	FoldedMachines  map[string]bool

	Search     string
	SearchOpen bool
	SearchAt   int

	KeySearch  string
	KeyAction  string
	KeySection string
	KeyOld     string
	KeyError   string
}

// defaultSettings is the state before the person changes anything.
func defaultSettings() settingsState {
	return settingsState{
		Section:         "look",
		Language:        "system",
		Theme:           "graphite",
		Accent:          "",
		InterfaceFont:   "Geist",
		InterfaceSize:   13,
		TerminalFont:    "JetBrains Mono",
		TerminalSize:    14,
		Zoom:            1,
		Clipboard:       true,
		MinContrast:     1,
		AlertDone:       true,
		AlertNeeds:      true,
		AlertSound:      true,
		DoneSound:       "Glass",
		NeedsSound:      "Ping",
		MutedAgents:     map[string]bool{},
		AutoInstall:     true,
		BackgroundCheck: true,
		Telemetry:       true,
		SidebarOpen:     true,
		SidebarWidth:    268,
		FoldedMachines:  map[string]bool{},
	}
}

// pickerState is the go-to-space-or-tab list.
type pickerState struct {
	Query string
}

// isOverlay reports whether a panel is over the window.
func (a *App) isOverlay() bool { return a.overlay != "" }

// Run runs one of the app's commands by name, with an optional value.
func (a *App) Run(action string) {
	if a.overlay == "close-confirm" && action != "overlay.close" {
		return
	}
	name, value := action, ""
	if i := strings.Index(action, ":"); i > 0 {
		name, value = action[:i], action[i+1:]
	}
	sp, tab := a.S.Active()
	switch name {
	case "tab.new":
		a.newTab()
	case "tab.close":
		if tab != nil {
			a.requestClose("tab.close", tab.ID)
		}
	case "tab.next":
		a.S.NextTab(1)
	case "tab.prev":
		a.S.NextTab(-1)
	case "tab.select":
		if n := atoi(value); n > 0 {
			if tabs := a.S.Tabs(); n <= len(tabs) {
				a.S.SelectTab(tabs[n-1].ID)
			}
		}
	case "tab.rename":
		if tab != nil {
			a.Prompt(a.tr("Rename tab"), a.tr("Leave empty for its number"), tab.Name, "tab.rename", tab.ID)
		}
	case "space.new":
		a.newSpace()
	case "space.close":
		if sp != nil {
			a.requestClose("space.close", sp.ID)
		}
	case "space.next":
		a.S.NextSpace(1)
	case "space.prev":
		a.S.NextSpace(-1)
	case "space.select":
		if n := atoi(value); n > 0 {
			a.S.SelectSpace(n, "")
		}
	case "space.rename":
		if sp != nil {
			a.Prompt(a.tr("Rename space"), a.tr("Leave empty for where its first terminal is"), sp.Name, "space.rename", sp.ID)
		}
	case "pane.split":
		a.split(value)
	case "pane.close":
		if p := a.S.FocusedPane(); p.Pane != "" {
			a.requestClose("pane.close", p.Pane)
		}
	case "pane.zoom":
		if p := a.S.FocusedPane(); p.Pane != "" {
			a.S.ZoomPane(p.Pane)
		}
	case "pane.go":
		a.S.Go(value)
	case "pane.swap":
		a.S.Swap(value)
	case "agent.next":
		a.nextAgent(1)
	case "agent.prev":
		a.nextAgent(-1)
	case "layout.toggle":
		if value == "sidebar" {
			a.toggleSidebar()
		}
	case "app.palette":
		a.ToggleOverlay("palette")
	case "app.settings":
		a.settings.Section = value
		if a.settings.Section == "" {
			a.settings.Section = "look"
		}
		a.ToggleOverlay("settings")
	case "app.help":
		a.ToggleOverlay("help")
	case "app.welcome":
		a.OpenOverlay("welcome")
	case "app.zoom_in":
		a.zoomBy(1.1)
	case "app.zoom_out":
		a.zoomBy(1 / 1.1)
	case "app.zoom_reset":
		a.Zoom = 1
		a.Repaint()
	case "app.theme":
		if knownTheme(value) {
			dark := a.appearanceDark()
			switch value {
			case "paper":
				dark = false
			case "graphite", "ink", "moss":
				dark = true
			}
			a.chooseTheme(value, dark)
		}
		a.Repaint()
	case "app.quit":
		a.quit()
	case "mod.open":
		if value == "picker" {
			a.ToggleOverlay("picker")
		} else {
			a.OpenOverlay(value)
		}
	case "terminal.find":
		a.openFind()
	case "terminal.find_next":
		a.findStep(false)
	case "terminal.find_previous":
		a.findStep(true)
	case "terminal.open":
		a.newTab()
	case "window.minimize":
		if a.Win != nil {
			a.Win.Minimize()
		}
	case "window.maximize":
		if a.Win != nil {
			a.Win.ToggleMaximize()
		}
	case "window.close":
		if a.Win != nil {
			a.Win.Close()
		}
	case "overlay.close":
		a.CloseOverlay()
	}
}

// Prompt opens a dialog that asks for a name.
func (a *App) Prompt(title, hint, value, command, target string) {
	a.prompt = PromptState{Title: title, Hint: hint, Value: value, Command: command, Target: target}
	a.OpenOverlay("prompt")
}

// nextAgent focuses the next agent in the list.
func (a *App) nextAgent(delta int) {
	list := a.S.Agents()
	if len(list) == 0 {
		return
	}
	at := -1
	focused := a.S.FocusedPane()
	for i, ag := range list {
		if ag.Pane == focused.Pane {
			at = i
		}
	}
	n := len(list)
	if at < 0 {
		at = 0
	} else {
		at = ((at+delta)%n + n) % n
	}
	a.S.FocusPane(list[at].Pane)
}

// zoomBy changes the app's zoom, between 0.5 and 3 as settings.json allows.
func (a *App) zoomBy(factor float32) {
	z := a.Zoom * factor
	if z < 0.5 {
		z = 0.5
	}
	if z > 3 {
		z = 3
	}
	a.Zoom = z
	a.Repaint()
}

// quit closes the window and the app.
func (a *App) quit() {
	if a.Win != nil {
		a.Win.Close()
	}
}

// OpenOverlay shows one of the app's overlays.
func (a *App) OpenOverlay(name string) {
	a.overlay = name
	if name == "palette" {
		a.palette.Query = ""
		a.palette.At = 0
	}
	if name == "settings" {
		a.settings.SearchOpen = false
		a.settings.Search = ""
		a.settings.SearchAt = 0
	}
	a.Repaint()
}

// ToggleOverlay opens an overlay, or closes it when it is the one showing.
func (a *App) ToggleOverlay(name string) {
	if a.overlay == name {
		a.CloseOverlay()
		return
	}
	a.OpenOverlay(name)
}

// CloseOverlay closes the overlay on top.
func (a *App) CloseOverlay() {
	if a.overlay == "settings" {
		a.settings.KeyAction, a.settings.KeyError = "", ""
	}
	if a.overlay == "close-confirm" {
		a.closing = closeState{}
	}
	if a.overlay == "peek" {
		a.dropPeek()
	}
	a.overlay = ""
	a.focusTerm = nil
	a.find.focus = a.find.key != ""
	a.Repaint()
}

// atoi parses a small positive number.
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// homeDir is the user's home.
func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
