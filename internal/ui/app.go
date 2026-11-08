// Package ui draws the window: the Herdr frame rebuilt in Go on MyGo's
// native UI, with herdr-core running the terminals behind it.
package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"herdr-mygo/internal/herdr"
	"herdr-mygo/internal/i18n"
)

const (
	// sidebarMinWidth and sidebarMaxWidth bound the width the edge drags;
	// the sidebar opens again at the width it was closed at.
	sidebarMinWidth float32 = 160
	sidebarMaxWidth float32 = 480
	// sidebarFooterHeight is the height of the sidebar's footer.
	sidebarFooterHeight float32 = 41
	// paneGap is the room between two panes, where their divider sits;
	// paneMinWidth and paneMinHeight are the smallest a pane is dragged to.
	paneGap       float32 = 8
	paneMinWidth  float32 = 80
	paneMinHeight float32 = 60
	// sidebarDuration is how long the sidebar takes to open or close.
	sidebarDuration = 200 * time.Millisecond
	// Keep recently visited tabs ready without retaining every visited terminal.
	maxHiddenTerminals = 12
)

// App is the window's state: the core's world, what the window shows of it,
// and the terminal views attached to the core's terminals.
type App struct {
	S   *herdr.Session
	T   *Theme
	Win *mygo.Window

	localizer           atomic.Pointer[i18n.Localizer]
	systemLanguage      string
	localizationStarted bool

	W, H float32

	// Zoom is how large the app draws, as settings.json's zoom.
	Zoom float32
	// SidebarWidth is the sidebar's width in DIPs while it is open, and
	// SidebarOpen tells whether it is shown. sidebarShown is the width the
	// last frame drew, which eases between the two as it opens and closes;
	// sidebarDrag tells whether the pointer is dragging its edge.
	SidebarWidth float32
	SidebarOpen  bool
	sidebarShown float32
	sidebarDrag  bool
	// splitRatio is the spaces list's share of the sidebar's two lists;
	// agentsCollapsed folds the agents list to its header, as the settings
	// keep it.
	splitRatio      float32
	agentsCollapsed bool
	// paneDrag tells that the pointer holds a divider between panes this
	// frame, paneDragged that it held one the frame before, so that letting
	// go writes the sizes the drag left to the document.
	paneDrag    bool
	paneDragged bool

	// themeName is the theme drawn now: the appearance may pick one over
	// the theme chosen. systemDark is the desktop's appearance, and
	// systemKnown tells whether it was read yet.
	themeName   string
	systemDark  bool
	systemKnown bool

	// settingsPath is the file the settings are kept in, empty when the
	// window was made without one (a test, a preview), and savedSettings
	// the text written last.
	settingsPath  string
	savedSettings string

	mu            sync.Mutex
	terms         map[string]*paneTerm
	starting      map[string]bool
	failedTerms   map[string]bool // failed attachments wait until the pane is shown again
	nextToastID   uint64
	termUsed      uint64
	terminalStyle terminal.Font       // guarded by mu; attachment goroutines use the latest choice
	terminalPrefs terminalPreferences // guarded by mu
	fontFamilies  []ui.FontFamily

	focusTerm *terminal.Terminal
	overlay   string
	err       error

	// peek is the terminal the window looks at over the tab, and what has
	// become of its attachment.
	peek peekState
	find findState

	palette PaletteState
	notes   []Toast

	closing   closeState
	prompt    PromptState
	settings  settingsState
	alerts    alertState
	picker    pickerState
	menuItems []MenuItem
	menuTitle string
	menuX     float32
	menuY     float32

	panel         string
	files         filesPanel
	changes       changesPanel
	commitMessage string

	mach           machineAddState
	foldedMachines map[string]bool // the machines folded to their header, as the settings keep them

	fsMu   sync.Mutex
	remote map[string]*remoteDir
}

type paneTerm struct {
	key      string
	pane     string
	addr     string
	term     *terminal.Terminal
	conn     peekStream
	lastUsed uint64
}

// NewApp makes the window's state.
func NewApp(s *herdr.Session, theme *Theme) *App {
	return &App{
		S: s, T: theme,
		themeName:      "graphite",
		Zoom:           1,
		SidebarWidth:   268,
		SidebarOpen:    true,
		sidebarShown:   268,
		splitRatio:     0.5,
		settings:       defaultSettings(),
		terminalStyle:  terminal.Font{Family: TerminalFont, Size: 14},
		terminalPrefs:  terminalPreferences{clipboard: true, contrast: 1},
		terms:          map[string]*paneTerm{},
		starting:       map[string]bool{},
		failedTerms:    map[string]bool{},
		foldedMachines: map[string]bool{},
	}
}

// Repaint asks the window for a frame.
func (a *App) Repaint() {
	if a.Win != nil {
		a.Win.Invalidate()
	}
}

// View builds the whole window.
func (a *App) View(c *ui.Context) {
	style := *c.Theme()
	style.Font, style.FontSize = a.interfaceFont(), a.fontSize(13)
	c.SetTheme(&style)
	a.applyTerminalFont()
	t := a.T
	if a.S == nil {
		ui.Box(c).Fill().Background(t.Canvas).Center().Children(func() {
			ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Herdr").FontSize(a.fontSize(18)).FontWeight(600).TextColor(t.Foreground)
				msg := a.tr("Connecting to herdr-core…")
				if a.err != nil {
					msg = a.err.Error()
				}
				ui.Text(c, msg).FontSize(a.fontSize(12.5)).TextColor(t.Dim)
			})
		})
		return
	}
	c.SetZoom(a.Zoom)
	w, h := c.Size()
	a.W, a.H = w, h
	if a.overlay != "settings" || a.settings.KeyAction == "" {
		a.keys(c)
	}
	if a.overlay == "settings" && a.settings.KeyAction == "" && c.Shortcut(ui.Cmd, ui.KeyF) {
		a.openSettingsSearch()
	}
	if a.overlay != "" && a.overlay != "peek" && !(a.overlay == "settings" && a.settings.KeyAction != "") && !(a.overlay == "machine-add" && a.addMachineSnapshot().term != "") && c.Shortcut(0, ui.KeyEscape) {
		if a.overlay == "settings" && a.settings.ThemePicker != 0 {
			a.settings.ThemePicker = 0
		} else if a.overlay == "settings" && a.settings.SearchOpen {
			a.closeSettingsSearch()
		} else {
			a.CloseOverlay()
		}
	}
	a.collect()
	a.agentAlerts()
	// The edge is read before the sidebar is built, so that a drag follows
	// the pointer in the frame it moves.
	a.resizeHandle(c)
	a.paneDragged, a.paneDrag = a.paneDrag, false
	frame := t.windowFrame(c).Font(a.interfaceFont())
	frame.Children(func() {
		a.sidebar(c)
		if a.sidebarShown > 0 {
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
		}
		a.work(c)
	})
	a.overlays(c)
	if a.paneDragged && !a.paneDrag {
		// The pointer let a divider go: its panes keep the sizes it left.
		a.S.WriteDoc()
	}
	// A setting this frame changed goes to the file it is kept in.
	a.saveSettings()
}

// sidebar draws the brand row, the spaces and agents lists, and the footer.
// Its width eases between the width it opened at and nothing, as MyGo's
// sidebar example does, and ClipX cuts the content as it goes.
func (a *App) sidebar(c *ui.Context) {
	t := a.T
	// listHeight is what the spaces and agents lists share, the divider aside.
	headerHeight, _, _, _ := a.brandLayout(c)
	listHeight := a.H - headerHeight - sidebarFooterHeight - 1
	if listHeight < 90 {
		listHeight = 90
	}
	spacesHeight := listHeight / 2
	if a.agentsCollapsed {
		spacesHeight = listHeight - 41
	} else {
		minSpaces, maxSpaces := splitLimits(listHeight)
		spacesHeight = listHeight * a.splitRatio
		if spacesHeight < minSpaces {
			spacesHeight = minSpaces
		}
		if spacesHeight > maxSpaces {
			spacesHeight = maxSpaces
		}
	}
	side := ui.Column(c).FillHeight().Background(t.Chrome).ClipX()
	var width float32
	if a.sidebarDrag {
		// The pointer holds the edge: it goes where it goes, at once.
		width = side.Jump("sidebar-width", a.SidebarWidth)
	} else {
		target := float32(0)
		if a.SidebarOpen {
			target = a.SidebarWidth
		}
		width = side.AnimateWith("sidebar-width", target, sidebarDuration, ui.EaseInOut)
	}
	a.sidebarShown = width
	side.Width(width)
	side.Children(func() {
		a.brand(c)
		ui.Box(c).Height(spacesHeight).FillWidth().Children(func() { a.spaces(c) })
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Box(c).Grow(1).FillWidth().Children(func() { a.agents(c) })
		a.footer(c)
	})
	if a.SidebarOpen && width > 0 && !a.agentsCollapsed {
		a.splitHandle(c, spacesHeight, listHeight)
	}
}

// splitHandle drags the line between the spaces and agents lists.
func (a *App) splitHandle(c *ui.Context, spacesHeight, listHeight float32) {
	t := a.T
	headerHeight, _, _, _ := a.brandLayout(c)
	ui.Overlay(c, func() {
		handle := ui.Box(c).Absolute().Top(headerHeight + spacesHeight - 3).Left(0).
			Width(a.sidebarShown).Height(6).Cursor(ui.CursorResizeNS)
		// Dragged reports the pointer's horizontal travel first: the divider
		// follows its vertical one, and stays lit while the press lasts.
		_, dy, held := handle.Dragged()
		if held || handle.Hovered() {
			handle.Background(Alpha(t.Accent, 0.35))
		}
		if held && listHeight > 0 {
			minSpaces, maxSpaces := splitLimits(listHeight)
			next := spacesHeight + dy
			if next < minSpaces {
				next = minSpaces
			}
			if next > maxSpaces {
				next = maxSpaces
			}
			a.splitRatio = next / listHeight
		}
	})
}

// splitLimits keeps the spaces list tall enough to read, and leaves the
// agents header and one section of its list showing, until the window is
// too short for both.
func splitLimits(listHeight float32) (min, max float32) {
	min = 90
	max = listHeight - 101
	if max < min {
		max = listHeight - 41
		if max < 0 {
			max = 0
		}
		min = max
	}
	return min, max
}

// brandLayout keeps the brand and its actions clear of native title buttons.
// Wide controls on the left can leave too little room in a narrow sidebar;
// in that case the brand sits below the controls instead.
func (a *App) brandLayout(c *ui.Context) (height, top, left, right float32) {
	bar := c.TitleBar()
	left = 14
	if isMac() {
		left = 76
	}
	left = max(left, bar.Left+10)
	right = max(float32(10), bar.Right-(a.W-a.sidebarShown)+10)
	height = max(float32(46), bar.Height)
	if bar.Height > 0 && a.sidebarShown-left-right < 130 {
		top, left, right = bar.Height, 14, 10
		height = top + 46
	}
	return
}

// brand is the row with the logo, search and the sidebar toggle.
func (a *App) brand(c *ui.Context) {
	t := a.T
	height, top, left, right := a.brandLayout(c)
	row := ui.Row(c).Height(height).FillWidth().Background(t.Chrome).
		Padding(top, right, 0, left).Gap(4).AlignItems(ui.Center).DragWindow()
	row.Children(func() {
		ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
			a.logo(c, 20)
			ui.Text(c, "Herdr").FontSize(a.fontSize(13.5)).FontWeight(600).TextColor(t.Foreground)
		})
		a.iconButton(c, "search", 26, a.tr("Search"), func() { a.OpenOverlay("picker") })
		a.iconButton(c, "sidebar", 26, a.tr("Hide sidebar"), a.toggleSidebar)
	})
}

func (a *App) logo(c *ui.Context, size float32) {
	data, err := iconPNG("img/herdr.png")
	if err != nil {
		return
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		return
	}
	ui.Box(c).Size(size, size).Radius(5).Clip().Children(func() {
		ui.Image(c, bm).Size(size, size).Fit(ui.Contain)
	})
}

// iconButton is a small square button of the chrome; tip names it for the
// pointer and for assistive technology.
func (a *App) iconButton(c *ui.Context, name string, size float32, tip string, onClick func()) {
	t := a.T
	b := ui.Box(c).Size(size, size).Radius(6).Center().Label(tip).Tooltip(tip)
	if b.Hovered() {
		b.Background(t.Hover)
	}
	if b.Clicked() && onClick != nil {
		onClick()
	}
	b.Cursor(ui.CursorPointer)
	b.Children(func() {
		color := t.Muted
		if b.Hovered() {
			color = t.Foreground
		}
		a.icon(c, name, size-11, color)
	})
}

// icon draws one of the app's SVGs.
func (a *App) icon(c *ui.Context, name string, size float32, color ui.Color) {
	svg := iconSVG(name)
	if svg == nil {
		return
	}
	ui.Icon(c, svg).Size(size, size).TextColor(color)
}

// spinner is the working indicator: the ring and its arc, turning once a
// second. Loop keeps frames coming while it shows, so the window animates
// only while some agent or pane is working.
func (a *App) spinner(c *ui.Context, size float32, color ui.Color) {
	svg := iconSVG("spinner")
	if svg == nil {
		return
	}
	spin := ui.Icon(c, svg).Size(size, size).TextColor(color)
	spin.Rotate(spin.Loop("spin", time.Second, ui.Linear) * 360)
}

// programIcon draws what a terminal runs, as the app's program table says:
// a full-color logo in its own colors, or a one-color glyph in color.
func (a *App) programIcon(c *ui.Context, p herdr.ProgramInfo, size float32, color ui.Color) {
	svg := iconSVG("program:" + p.Icon)
	if svg == nil {
		a.icon(c, "terminal", size, color)
		return
	}
	if !p.Tint {
		ui.Image(c, svg).Size(size, size).Fit(ui.Contain)
		return
	}
	ui.Icon(c, svg).Size(size, size).TextColor(color)
}

// toggleSidebar hides the sidebar, or brings it back at the width it had.
func (a *App) toggleSidebar() {
	a.SidebarOpen = !a.SidebarOpen
	a.settings.SidebarOpen = a.SidebarOpen
	a.settings.SidebarWidth = a.SidebarWidth
	a.Repaint()
}

// footer is Settings.
func (a *App) footer(c *ui.Context) {
	t := a.T
	ui.Column(c).Height(sidebarFooterHeight).FillWidth().Background(t.Chrome).Children(func() {
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Column(c).Grow(1).FillWidth().Padding(4, 8, 8, 8).Children(func() {
			a.footerRow(c, "settings", a.tr("Settings"), false, func() { a.OpenOverlay("settings") })
		})
	})
}

func (a *App) footerRow(c *ui.Context, icon, label string, dim bool, action func()) {
	t := a.T
	row := ui.Row(c).Height(28).FillWidth().PaddingX(8).Gap(9).Radius(8).AlignItems(ui.Center)
	color := t.Muted
	if dim {
		color = t.Dim
	}
	if row.Hovered() {
		row.Background(t.Hover)
		color = t.Foreground
	}
	row.Cursor(ui.CursorPointer)
	row.Children(func() {
		a.icon(c, icon, 14, color)
		ui.Text(c, label).FontSize(a.fontSize(13)).TextColor(color)
	})
	if row.Clicked() {
		action()
	}
}

// work is the tabs bar, the panes and the status bar.
func (a *App) work(c *ui.Context) {
	t := a.T
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.tabsBar(c)
		ui.Row(c).Grow(1).FillWidth().Background(t.Canvas).Padding(8).Gap(8).Children(func() {
			ui.Box(c).Grow(1).FillHeight().Children(func() { a.paneArea(c) })
			if a.panel != "" {
				ui.Box(c).Width(panelWidth).FillHeight().Children(func() { a.panelView(c) })
			}
		})
		a.statusBar(c)
	})
}

// tabsBar is the row of tabs, the new tab button and the panel buttons.
func (a *App) tabsBar(c *ui.Context) {
	t := a.T
	// With the sidebar hidden the tabs start at the window's edge, so on
	// macOS they leave the traffic lights their room, as the brand does.
	left := float32(10)
	if isMac() {
		if room := 76 - a.sidebarShown; room > left {
			left = room
		}
	}
	bar := c.TitleBar()
	start := a.sidebarShown
	if start > 0 {
		start++ // the border between the sidebar and the tabs
	}
	// Native controls overlay either edge on Linux and the right on Windows.
	// Only reserve the part of the left controls that reaches this row.
	left = max(left, bar.Left-start+10)
	ui.Column(c).Height(max(float32(42), bar.Height+1)).FillWidth().Children(func() {
		ui.Row(c).Grow(1).FillWidth().Background(t.Chrome).Padding(0, bar.Right+10, 0, left).Gap(3).AlignItems(ui.Center).DragWindow().Children(func() {
			if !a.SidebarOpen {
				a.iconButton(c, "sidebar", 26, a.tr("Show sidebar"), a.toggleSidebar)
			}
			if !a.spaceEmpty() {
				a.tabList(c)
			} else {
				ui.Spacer(c)
			}
			a.newMenuButton(c)
			a.panelButton(c, "files", a.tr("Files"))
			a.panelButton(c, "changes", a.tr("Changes"))
		})
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
	})
}

// newMenuButton opens the new tab menu at the pointer.
func (a *App) newMenuButton(c *ui.Context) {
	t := a.T
	b := ui.Box(c).Size(26, 26).Radius(6).Center()
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Cursor(ui.CursorPointer)
	b.Children(func() {
		color := t.Dim
		if b.Hovered() {
			color = t.Foreground
		}
		a.icon(c, "plus", 15, color)
	})
	if b.Clicked() {
		x, y, _ := b.PointerPosition()
		bounds := b.Bounds()
		a.menuX, a.menuY = bounds.X+x, bounds.Y+y
		a.OpenOverlay("new-menu")
	}
}

// tabList draws the tabs of the shown space, scrolled sideways.
func (a *App) tabList(c *ui.Context) {
	sp, active := a.S.Active()
	if sp == nil {
		return
	}
	ui.Row(c).Grow(1).FillHeight().Gap(3).AlignItems(ui.Center).Children(func() {
		for _, tab := range sp.Tabs {
			a.tab(c, tab, active != nil && tab.ID == active.ID)
		}
	})
}

// tab is one tab pill.
func (a *App) tab(c *ui.Context, tab *herdr.Tab, active bool) {
	t := a.T
	panes := a.S.Panes(tab.ID)
	name := herdr.TabName(tab, tab.Index)
	if strings.HasPrefix(name, "tb") && len(name) > 12 {
		name = a.tr("Terminal")
	}
	status := ""
	zoomed := false
	lead := herdr.PaneInfo{Program: herdr.Program(nil)}
	for _, p := range panes {
		if p.Status != "" {
			status = herdr.Worse(status, p.Status)
		}
		if p.Zoomed {
			zoomed = true
		}
		// The tab wears the program of its lead pane: the focused one, else the
		// first (the app's mods/tabs.js lead()).
		if lead.Terminal == "" || p.Focused {
			lead = p
		}
	}
	row := ui.Row(c).Height(29).PaddingX(10).Gap(7).Radius(7).AlignItems(ui.Center).Border(1, t.Chrome).Shrink(1)
	row.Background(t.Chrome)
	color := t.Muted
	if active {
		row.Background(t.Raised).Border(1, t.Border)
		color = t.Foreground
	} else if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Cursor(ui.CursorPointer)
	row.Children(func() {
		a.programIcon(c, lead.Program, 14, t.Muted)
		label := name
		if zoomed {
			label += " Z"
		}
		ui.Text(c, label).FontSize(a.fontSize(12.8)).TextColor(color).MaxWidth(170).Ellipsis("…")
		switch status {
		case "working":
			a.spinner(c, 12, t.Working)
		case "blocked", "done":
			ui.Box(c).Size(6, 6).Radius(3).Background(t.Status(status))
		}
		if active {
			a.iconButton(c, "close", 23, a.tr("Close tab"), func() { a.requestClose("tab.close", tab.ID) })
		}
	})
	if row.Clicked() {
		a.S.SelectTab(tab.ID)
	}
	if row.DoubleClicked() {
		a.Prompt(a.tr("Rename tab"), a.tr("Leave empty for its number"), tab.Name, "tab.rename", tab.ID)
	}
	if row.RightClicked() {
		x, y, _ := row.PointerPosition()
		b := row.Bounds()
		id, spaceID := tab.ID, a.S.ActiveSpaceID()
		a.openMenuAt(b.X+x, b.Y+y, []MenuItem{
			{Label: a.tr("Rename"), Icon: "square-pen", Do: func() { a.Prompt(a.tr("Rename tab"), a.tr("Leave empty for its number"), tab.Name, "tab.rename", id) }},
			{Label: a.tr("New tab"), Icon: "plus", Do: func() { a.newTab() }},
			{Label: a.tr("Close"), Icon: "close", Do: func() { a.requestClose("tab.close", id) }},
		})
		_ = spaceID
	}
}

// panelButton is Files or Changes in the tabs bar.
func (a *App) panelButton(c *ui.Context, id, label string) {
	t := a.T
	on := a.panel == id
	row := ui.Row(c).Height(26).PaddingX(8).Gap(5).Radius(6).AlignItems(ui.Center).Background(t.Chrome)
	color := t.Muted
	if on {
		row.Background(t.Selected)
		color = t.Foreground
	}
	row.Cursor(ui.CursorPointer)
	if row.Hovered() && !on {
		row.Background(t.Hover)
	}
	row.Children(func() {
		a.icon(c, id, 14, color)
		ui.Text(c, label).FontSize(a.fontSize(12)).TextColor(color)
	})
	if row.Clicked() {
		a.OpenPanel(id)
	}
}

// spaceEmpty reports whether nothing is open at all.
func (a *App) spaceEmpty() bool {
	return a.S.ActiveSpaceID() == "" || len(a.S.Tabs()) == 0
}

// newTab adds a tab to the shown space, or a space when there is none.
func (a *App) newTab() {
	go func() {
		if a.spaceEmpty() {
			_, _, err := a.S.NewSpace("", "", "", nil)
			if err != nil {
				a.toast(a.tr("Cannot start a terminal"), err.Error(), "error")
			}
			return
		}
		if _, err := a.S.NewTab("", "", nil); err != nil {
			a.toast(a.tr("Cannot start a terminal"), err.Error(), "error")
		}
	}()
}

// paneArea draws the shown tab's panes as its tree arranges them.
func (a *App) paneArea(c *ui.Context) {
	sp, tab := a.S.Active()
	if sp == nil || tab == nil {
		a.emptyWork(c)
		return
	}
	panes := a.S.Panes(tab.ID)
	if len(panes) == 0 {
		a.emptyWork(c)
		return
	}
	if zoomed := a.S.Zoomed(); zoomed != "" {
		for _, p := range panes {
			if p.Pane == zoomed {
				a.paneCard(c, p, 10, 0)
				return
			}
		}
	}
	tree := a.S.Tree(tab.ID)
	if tree == nil {
		a.emptyWork(c)
		return
	}
	byPane := map[string]herdr.PaneInfo{}
	for _, p := range panes {
		byPane[p.Pane] = p
	}
	a.renderNode(c, tab.ID, tree, byPane, true, "row")
}

// renderNode draws a row, a column, or a pane. axis is the direction of the
// parent: a row's children grow across, a column's children grow down.
func (a *App) renderNode(c *ui.Context, tab string, n *herdr.Node, panes map[string]herdr.PaneInfo, root bool, axis string) {
	if n == nil {
		return
	}
	if n.Type == "row" || n.Type == "column" {
		var box ui.Element
		if n.Type == "row" {
			box = ui.Row(c.Key(n.ID))
		} else {
			box = ui.Column(c.Key(n.ID))
		}
		if root {
			box = box.Fill()
		} else {
			box = a.applySize(box, n, axis)
		}
		// The room between two children is their divider: a drag there gives
		// one pane more and the other less.
		across := n.Type == "row"
		box.Children(func() {
			for i, child := range n.Children {
				if i > 0 {
					a.paneDivider(c, tab, n, box, i, across)
				}
				a.renderNode(c, tab, child, panes, false, n.Type)
			}
		})
		return
	}
	info, ok := panes[n.ID]
	if !ok {
		return
	}
	box := ui.Box(c.Key(n.ID))
	if root {
		box = box.Fill()
	} else {
		box = a.applySize(box, n, axis)
	}
	box.Children(func() { a.paneCard(c, info, 10, 0) })
}

// paneDivider is the line between the children at and at-1 of a split: the
// pointer drags it along the split's axis, and it lights up while it is
// hovered or held, as the sidebar's divider does.
func (a *App) paneDivider(c *ui.Context, tab string, n *herdr.Node, box ui.Element, at int, across bool) {
	t := a.T
	var handle ui.Element
	if across {
		handle = ui.Box(c).Width(paneGap).FillHeight().Shrink(0).Cursor(ui.CursorResizeEW)
	} else {
		handle = ui.Box(c).Height(paneGap).FillWidth().Shrink(0).Cursor(ui.CursorResizeNS)
	}
	dx, dy, held := handle.Dragged()
	if held || handle.Hovered() {
		handle.Background(Alpha(t.Accent, 0.35))
	}
	if !held || a.S == nil {
		return
	}
	delta := dx
	min := paneMinWidth
	if !across {
		delta, min = dy, paneMinHeight
	}
	if delta == 0 {
		return
	}
	weights, ok := paneWeights(paneSpans(n, box, across), at-1, delta, min)
	if !ok {
		return
	}
	a.paneDrag = true
	a.S.SetWeights(tab, weights)
}

// paneSpan is one child of a split as a divider drag reads it: the weight it
// is sized by, zero when it is fixed to a px size, and the DIPs that size
// gave it in the frame before.
type paneSpan struct {
	ID     string
	Weight float64
	Span   float32
}

// paneSpans reads the children of a split as its last frame drew them, along
// the axis its dividers move on: the room the box had, the gaps between the
// children aside, shared as applySize shares it.
func paneSpans(n *herdr.Node, box ui.Element, across bool) []paneSpan {
	r := box.Bounds()
	extent := r.W
	if !across {
		extent = r.H
	}
	room := extent - float32(len(n.Children)-1)*paneGap
	var fixed float32
	var weightSum float64
	for _, child := range n.Children {
		fixed += fixedSpan(child)
		weightSum += nodeWeight(child)
	}
	pool := room - fixed
	out := make([]paneSpan, len(n.Children))
	for i, child := range n.Children {
		w := nodeWeight(child)
		span := fixedSpan(child)
		if w > 0 && weightSum > 0 {
			span = pool * float32(w/weightSum)
		}
		out[i] = paneSpan{ID: child.ID, Weight: w, Span: span}
	}
	return out
}

// fixedSpan is a node's px size, 0 when it grows by a weight.
func fixedSpan(n *herdr.Node) float32 {
	if n.Size != nil && n.Size.PX != nil && nodeWeight(n) == 0 {
		return float32(*n.Size.PX)
	}
	return 0
}

// nodeWeight is the weight applySize grows a node by: its own, else one for a
// node without a size, and none for one fixed to px.
func nodeWeight(n *herdr.Node) float64 {
	if n.Size != nil {
		if n.Size.Weight != nil && *n.Size.Weight > 0 {
			return *n.Size.Weight
		}
		if n.Size.PX != nil && *n.Size.PX > 0 {
			return 0
		}
	}
	return 1
}

// paneWeights is where a drag of the divider at at leaves the two children
// either side of it: the DIPs it travelled give the first more room and the
// second less, and neither goes under min. A child fixed to a px size takes
// the weight that keeps the room it has, so both stay in proportion as the
// window changes. ok is false when there is no room to share out.
func paneWeights(children []paneSpan, at int, delta, min float32) (map[string]float64, bool) {
	if at < 0 || at+1 >= len(children) {
		return nil, false
	}
	first, second := children[at], children[at+1]
	var inner, rest float32
	var restWeight float64
	for i, ch := range children {
		inner += ch.Span
		if i != at && i != at+1 && ch.Weight > 0 {
			rest += ch.Span
			restWeight += ch.Weight
		}
	}
	if inner <= 0 {
		return nil, false
	}
	// A fixed child takes the weight its DIPs are worth among the weights it
	// joins, which is what keeps the room it has.
	convert := func(ch paneSpan) float64 {
		if ch.Weight > 0 {
			return ch.Weight
		}
		if restWeight > 0 && rest > 0 {
			return float64(ch.Span) * restWeight / float64(rest)
		}
		return float64(ch.Span)
	}
	wf, ws := convert(first), convert(second)
	// unit is what a weight is worth in DIPs: from the pair when both grow,
	// else from the weights the fixed one joins, else one DIP a weight.
	unit := 1.0
	if first.Weight > 0 && second.Weight > 0 && first.Span+second.Span > 0 {
		unit = float64(first.Span+second.Span) / (first.Weight + second.Weight)
	} else if restWeight > 0 && rest > 0 {
		unit = float64(rest) / restWeight
	}
	dw := float64(delta) / unit
	sum := wf + ws
	low := float64(min) / unit
	if low*2 > sum {
		low = sum / 2
	}
	// A pane that reaches its smallest stops there, and the other one takes
	// the rest: the pair keeps the room it had.
	nf, ns := wf+dw, ws-dw
	if nf < low {
		nf, ns = low, sum-low
	}
	if ns < low {
		ns, nf = low, sum-low
	}
	return map[string]float64{first.ID: nf, second.ID: ns}, true
}

// applySize gives a node its share along its parent: px, a weight, or equal.
func (a *App) applySize(el ui.Element, n *herdr.Node, axis string) ui.Element {
	if n.Size != nil {
		if n.Size.Weight != nil && *n.Size.Weight > 0 {
			if axis == "row" {
				return el.Grow(float32(*n.Size.Weight)).FillHeight()
			}
			return el.Grow(float32(*n.Size.Weight)).FillWidth()
		}
		if n.Size.PX != nil && *n.Size.PX > 0 {
			if axis == "row" {
				return el.Width(float32(*n.Size.PX)).FillHeight()
			}
			return el.Height(float32(*n.Size.PX)).FillWidth()
		}
	}
	if axis == "row" {
		return el.Grow(1).FillHeight()
	}
	return el.Grow(1).FillWidth()
}

// paneCard frames one pane: its header and its terminal.
func (a *App) paneCard(c *ui.Context, p herdr.PaneInfo, radius, pad float32) {
	t := a.T
	borderColor := t.Border
	if p.Focused {
		borderColor = t.Accent
	}
	card := ui.Column(c.Key(p.Pane+"|"+p.Terminal)).Fill().Background(t.Background).Radius(radius).Border(1, borderColor).Clip()
	card.Children(func() {
		a.paneHeader(c, p)
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			a.terminalView(c, p)
		})
	})
}

// paneHeader is the pane's bar: program, folder, status, buttons.
func (a *App) paneHeader(c *ui.Context, p herdr.PaneInfo) {
	t := a.T
	row := ui.Row(c).Height(30).FillWidth().Background(t.Background).PaddingX(10).Gap(8).AlignItems(ui.Center)
	hovered := row.Hovered()
	row.Children(func() {
		if p.Terminal != "" {
			a.programIcon(c, p.Program, 14, t.Dim)
		}
		name := p.Title
		if name == "" {
			name = "shell"
		}
		ui.Text(c, name).FontSize(a.fontSize(12)).FontWeight(500).TextColor(t.Foreground).MaxWidth(260).Ellipsis("…")
		ui.Text(c, herdr.ShortFolder(p.Cwd)).FontSize(a.fontSize(12)).TextColor(t.Dim).Grow(1).Ellipsis("…")
		switch p.Status {
		case "working":
			a.spinner(c, 12, t.Working)
		case "blocked":
			ui.Box(c).Height(18).PaddingX(6).Radius(5).Background(Alpha(t.Blocked, 0.15)).Center().Children(func() {
				ui.Text(c, a.tr("Needs you")).FontSize(a.fontSize(10.5)).FontWeight(600).TextColor(t.Blocked)
			})
		case "done":
			ui.Box(c).Size(7, 7).Radius(4).Background(t.Done)
		}
		if hovered {
			a.paneButton(c, "split-right", func() { a.split("right") })
			a.paneButton(c, "split-down", func() { a.split("down") })
			if p.Zoomed {
				a.paneButton(c, "unzoom", func() { a.S.ZoomPane(p.Pane) })
			} else {
				a.paneButton(c, "zoom", func() { a.S.ZoomPane(p.Pane) })
			}
			a.paneButton(c, "close", func() { a.requestClose("pane.close", p.Pane) })
		}
	})
	if row.Clicked() && !hovered {
		a.S.FocusPane(p.Pane)
	}
	if row.RightClicked() {
		x, y, _ := row.PointerPosition()
		b := row.Bounds()
		pane := p
		a.openMenuAt(b.X+x, b.Y+y, []MenuItem{
			{Label: a.tr("Find in terminal"), Icon: "search", Do: func() {
				a.S.FocusPane(pane.Pane)
				a.openFindFor(pane.Pane+"|"+pane.Terminal, a.terminalFor(pane))
			}},
			{Label: a.tr("Split right"), Icon: "split-right", Do: func() { a.splitPane(pane.Pane, "right") }},
			{Label: a.tr("Split down"), Icon: "split-down", Do: func() { a.splitPane(pane.Pane, "down") }},
			{Label: a.tr("Zoom"), Icon: "zoom", Do: func() { a.S.ZoomPane(pane.Pane) }},
			{Label: a.tr("Close pane"), Icon: "close", Do: func() { a.requestClose("pane.close", pane.Pane) }},
		})
	}
}

func (a *App) paneButton(c *ui.Context, icon string, action func()) {
	t := a.T
	b := ui.Box(c).Padding(3).Radius(4)
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Cursor(ui.CursorPointer)
	b.Children(func() {
		color := t.Dim
		if b.Hovered() {
			color = t.Foreground
		}
		a.icon(c, icon, 14, color)
	})
	if b.Clicked() {
		action()
	}
}

// splitPane focuses a pane and splits it.
func (a *App) splitPane(pane, direction string) {
	a.S.FocusPane(pane)
	a.split(direction)
}

func (a *App) split(direction string) {
	go func() {
		if _, err := a.S.Split(direction, nil); err != nil {
			a.toast(a.tr("Cannot split the pane"), err.Error(), "error")
		}
	}()
}

// terminalView draws the core terminal of a pane, attaching on first sight.
func (a *App) terminalView(c *ui.Context, p herdr.PaneInfo) {
	term := a.terminalFor(p)
	if term == nil {
		return
	}
	view := terminal.View(c, term).Fill()
	// Give a newly selected or attached terminal the keyboard once; later
	// frames leave panel inputs and terminal selections alone.
	key := p.Pane + "|" + p.Terminal
	if p.Focused && !a.isOverlay() && !(a.find.key == key && a.find.focus) && a.focusTerm != term {
		a.focusTerm = term
		view.Focus()
	}
	if view.Clicked() {
		a.S.FocusPane(p.Pane)
	}
	if view.RightClicked() {
		x, y, _ := view.PointerPosition()
		b := view.Bounds()
		pane := p
		a.openMenuAt(b.X+x, b.Y+y, []MenuItem{
			{Label: a.tr("Find in terminal"), Icon: "search", Do: func() {
				a.S.FocusPane(pane.Pane)
				a.openFindFor(key, term)
			}},
			{Label: a.tr("Split right"), Icon: "split-right", Do: func() { a.splitPane(pane.Pane, "right") }},
			{Label: a.tr("Split down"), Icon: "split-down", Do: func() { a.splitPane(pane.Pane, "down") }},
			{Label: a.tr("Zoom"), Icon: "zoom", Do: func() { a.S.ZoomPane(pane.Pane) }},
			{Label: a.tr("Close pane"), Icon: "close", Do: func() { a.requestClose("pane.close", pane.Pane) }},
		})
	}
	a.findBar(c, key, term)
}

// terminalFor returns the pane's terminal view, starting its attachment in
// the background the first time the pane shows.
func (a *App) terminalFor(p herdr.PaneInfo) *terminal.Terminal {
	if p.Terminal == "" {
		return nil
	}
	key := p.Pane + "|" + p.Terminal
	a.mu.Lock()
	if entry, ok := a.terms[key]; ok {
		a.termUsed++
		entry.lastUsed = a.termUsed
		a.mu.Unlock()
		return entry.term
	}
	if a.starting[key] || a.failedTerms[key] {
		a.mu.Unlock()
		return nil
	}
	a.starting[key] = true
	a.mu.Unlock()

	go a.startTerminal(key, p)
	return nil
}

// startTerminal attaches to the pane's terminal and makes its view.
func (a *App) startTerminal(key string, p herdr.PaneInfo) {
	t := a.T
	conn := newPeekConnection(func(cols, rows, cellW, cellH int) (peekStream, error) {
		attached, err := a.S.Attach(p.Terminal, cols, rows, t.TerminalForeground, t.Background, t.Cursor, cellW, cellH)
		if err != nil {
			a.terminalFailed(key, a.tr("Cannot show the terminal"), err)
			return nil, err
		}
		return attached, nil
	})
	var term *terminal.Terminal
	opts := a.currentTerminalOptions()
	opts.Conn, opts.Theme = conn, terminalTheme(t)
	opts.Padding, opts.PaddingColor = terminal.Padding{X: 14, Y: 12}, terminal.PaddingExtend
	// A paste that holds an image is taken for a pane on another
	// machine, whose program cannot read this computer's clipboard
	// (pasteImage). term is set by New, before any frame can draw
	// the terminal that a paste comes from.
	opts.OnPaste = func(d transfer.Data) bool {
		return a.pasteImage(term, p.Terminal, d)
	}
	term, err := terminal.New(opts)
	if err != nil {
		_ = conn.Close()
		a.terminalFailed(key, a.tr("Cannot start the terminal"), err)
		return
	}
	a.mu.Lock()
	if !a.starting[key] {
		a.mu.Unlock()
		_ = term.Close()
		return
	}
	term.SetFont(a.terminalStyle)
	term.SetMinimumContrast(a.terminalPrefs.contrast)
	a.termUsed++
	entry := &paneTerm{key: key, pane: p.Pane, addr: p.Terminal, term: term, conn: conn, lastUsed: a.termUsed}
	a.terms[key] = entry
	delete(a.starting, key)
	a.mu.Unlock()
	a.Repaint()
	go func() {
		<-term.Done()
		a.mu.Lock()
		current := a.terms[key] == entry && !a.failedTerms[key]
		a.mu.Unlock()
		if current {
			if p.Pane == "machine-add" {
				a.S.RefreshMachines()
				a.Repaint()
			} else {
				a.S.ClosePane(p.Pane)
			}
		}
	}()
}

// terminalFailed reports a failed attachment once per visible pane. Redrawing
// the warning or dismissing it must not immediately start another failing attach.
func (a *App) terminalFailed(key, title string, err error) {
	a.mu.Lock()
	delete(a.starting, key)
	a.failedTerms[key] = true
	entry := a.terms[key]
	delete(a.terms, key)
	a.mu.Unlock()
	if entry != nil {
		_ = entry.term.Close()
	}
	a.toast(title, err.Error(), "error")
}

// collect keeps recent open panes attached across tab switches, releasing
// removed or replaced panes and the oldest views beyond the hidden cache limit.
func (a *App) collect() {
	_, tab := a.S.Active()
	visible := map[string]bool{}
	if tab != nil {
		for _, p := range a.S.Panes(tab.ID) {
			visible[p.Pane+"|"+p.Terminal] = true
		}
	}
	live := map[string]bool{}
	for _, sp := range a.S.Places() {
		for _, tab := range sp.Tabs {
			for _, p := range a.S.Panes(tab.ID) {
				live[p.Pane+"|"+p.Terminal] = true
			}
		}
	}
	if a.overlay == "machine-add" {
		if st := a.addMachineSnapshot(); st.term != "" {
			key := "machine-add|" + st.term
			live[key], visible[key] = true, true
		}
	}
	if a.find.key != "" && !visible[a.find.key] && a.find.key != "peek|"+a.peek.addr {
		a.closeFind()
	}
	a.mu.Lock()
	for key := range a.starting {
		if !live[key] {
			delete(a.starting, key)
		}
	}
	for key := range a.failedTerms {
		if !visible[key] {
			delete(a.failedTerms, key)
		}
	}
	var gone []*paneTerm
	var hidden []*paneTerm
	for key, entry := range a.terms {
		if !live[key] {
			gone = append(gone, entry)
			delete(a.terms, key)
		} else if !visible[key] {
			hidden = append(hidden, entry)
		}
	}
	for len(hidden) > maxHiddenTerminals {
		oldest := 0
		for i, entry := range hidden {
			if entry.lastUsed < hidden[oldest].lastUsed {
				oldest = i
			}
		}
		entry := hidden[oldest]
		gone = append(gone, entry)
		delete(a.terms, entry.key)
		hidden = append(hidden[:oldest], hidden[oldest+1:]...)
	}
	a.mu.Unlock()
	for _, entry := range gone {
		_ = entry.term.Close()
	}
}

// SetSystemAppearance follows the desktop's light or dark appearance, and
// re-themes the window when the appearance asks for the system's. Main
// thread only.
func (a *App) SetSystemAppearance(dark bool) {
	if a.systemKnown && a.systemDark == dark {
		return
	}
	a.systemDark, a.systemKnown = dark, true
	if a.settings.Appearance == AppearanceSystem {
		a.applyTheme()
	}
}

func (a *App) appearanceDark() bool {
	switch a.settings.Appearance {
	case AppearanceLight:
		return false
	case AppearanceDark:
		return true
	}
	return a.systemDark
}

// applyTheme gives the appearance’s chosen colors to the window and terminals.
func (a *App) applyTheme() {
	name, th := a.settings.themeFor(a.appearanceDark())
	if validHex(a.settings.Accent) {
		th.Accent = ui.Hex(a.settings.Accent)
	}
	a.themeName, a.T = name, th
	a.applyTerminalTheme(th)
	a.Repaint()
}

// currentThemeName is the theme drawn for the current appearance.
func (a *App) currentThemeName() string {
	if a.themeName != "" {
		return a.themeName
	}
	return a.settings.Theme
}

// applyTerminalTheme repaints every open terminal with th, and tells the
// core the colors programs ask about.
func (a *App) applyTerminalTheme(th *Theme) {
	tt := terminalTheme(th)
	a.mu.Lock()
	views := make([]*paneTerm, 0, len(a.terms))
	for _, entry := range a.terms {
		views = append(views, entry)
	}
	if a.peek.term != nil {
		// A peeked terminal is not a pane's, and follows the theme too.
		peek := &paneTerm{term: a.peek.term}
		if a.peek.conn != nil {
			peek.conn = a.peek.conn
		}
		views = append(views, peek)
	}
	a.mu.Unlock()
	for _, entry := range views {
		entry.term.SetTheme(tt, nil)
		if entry.conn != nil {
			_ = entry.conn.UseTheme(tt)
		}
	}
	if a.Win != nil {
		_ = a.Win.SetBackgroundColor(colorHex(th.Background))
	}
}

// colorHex writes a color as the #rrggbb a window option takes.
func colorHex(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// terminalTheme is the terminal's colors from the window's theme.
func terminalTheme(t *Theme) *terminal.Theme {
	th := &terminal.Theme{
		Foreground:    t.TerminalForeground,
		Background:    t.Background,
		Cursor:        t.Cursor,
		Selection:     t.Selection,
		SelectionText: t.SelectionText,
		CursorText:    t.CursorText,
		Palette:       t.Ansi,
	}
	if t.terminalColors != nil {
		th.Search, th.SearchText = t.terminalColors.Search, t.terminalColors.SearchText
		th.SearchSelected, th.SearchSelectedText = t.terminalColors.SearchSelected, t.terminalColors.SearchSelectedText
	}
	return th
}

// statusBar is the bottom row: machine, branch, agents and the palette hint.
func (a *App) statusBar(c *ui.Context) {
	t := a.T
	ui.Column(c).Height(28).FillWidth().Children(func() {
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		row := ui.Row(c).Grow(1).FillWidth().Background(t.Chrome).PaddingX(14).Gap(16).AlignItems(ui.Center)
		row.Children(func() {
			icon, machine := "monitor", a.tr("This computer")
			if sp, _ := a.S.Active(); sp != nil && sp.Machine != nil {
				icon, machine = "server", *sp.Machine
			}
			a.statusItem(c, icon, machine)
			if b := a.S.Branch(); b != "" {
				a.statusItem(c, "branch", b)
			}
			agents := a.S.Agents()
			needs := 0
			for _, ag := range agents {
				if ag.Status == "blocked" {
					needs++
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.Dim)
				ui.Text(c, a.tr("%d agents", len(agents))).FontSize(a.fontSize(11.8)).TextColor(t.Muted)
				if needs > 0 {
					ui.Text(c, a.tr("· %d needs you", needs)).FontSize(a.fontSize(11.8)).TextColor(t.Blocked)
				}
			})
			ui.Spacer(c)
			hint := ui.Text(c, a.paletteHint()).FontSize(a.fontSize(11.8)).TextColor(t.Dim)
			if hint.Clicked() {
				a.OpenOverlay("palette")
			}
		})
	})
}

func (a *App) statusItem(c *ui.Context, icon, label string) {
	t := a.T
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		a.icon(c, icon, 13, t.Muted)
		ui.Text(c, label).FontSize(a.fontSize(11.8)).TextColor(t.Muted)
	})
}

// emptyWork is what the work area says before anything is open.
func (a *App) emptyWork(c *ui.Context) {
	t := a.T
	ui.Box(c).Fill().Center().Children(func() {
		ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.logo(c, 40)
			ui.Text(c, a.tr("No spaces yet")).FontSize(a.fontSize(15)).FontWeight(600).TextColor(t.Foreground)
			ui.Text(c, a.tr("Press ⌘N for a new space, or use the sidebar.")).FontSize(a.fontSize(12.5)).TextColor(t.Dim)
		})
	})
}

func isMac() bool { return osGOOS() == "darwin" }

var _ = time.Now

// Attach gives the window its world once the core is reached.
func (a *App) Attach(s *herdr.Session) {
	a.mu.Lock()
	a.S = s
	a.mu.Unlock()
	a.Repaint()
}

// Fail shows why the window cannot reach herdr-core.
func (a *App) Fail(err error) {
	a.err = err
	a.Repaint()
}

// paletteHint is the bottom right's live hint for the command palette.
func (a *App) paletteHint() string {
	if k := prettyKeys(KeyFor("app.palette")); k != "" {
		return a.tr("%s  commands", k)
	}
	return a.tr("commands")
}

// resizeHandle drags the sidebar's edge, as Herdr's divider does. The edge
// is where the last frame drew it, so it keeps up while the sidebar eases
// open or closed; a drag moves the width at once, as the pointer does.
func (a *App) resizeHandle(c *ui.Context) {
	t := a.T
	was := a.sidebarDrag
	a.sidebarDrag = false
	if !a.SidebarOpen {
		if was {
			a.settings.SidebarWidth = a.SidebarWidth
		}
		return
	}
	ui.Overlay(c, func() {
		handle := ui.Box(c).Absolute().Top(0).Bottom(0).Width(6).Left(a.sidebarShown - 2).Cursor(ui.CursorResizeEW)
		if handle.Hovered() {
			handle.Background(Alpha(t.Border, 0.8))
		}
		dx, _, held := handle.Dragged()
		a.sidebarDrag = held
		if held {
			a.SidebarWidth = clampWidth(a.sidebarShown + dx)
		}
		if was && !held {
			// Keep the width it was let go at, which is the width it opens
			// with again.
			a.settings.SidebarWidth = a.SidebarWidth
		}
	})
}

// clampWidth keeps the sidebar between the widths it can be dragged to.
func clampWidth(w float32) float32 {
	return min(max(w, sidebarMinWidth), sidebarMaxWidth)
}
