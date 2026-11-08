package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// paletteCommand is one entry of the command palette.
type paletteCommand struct {
	Title   string
	Group   string
	Action  string
	Keyword string
}

// paletteCommands are the commands the palette offers, as Rove names them.
var paletteCommands = append([]paletteCommand{
	{"New tab", "Tab", "tab.new", "tab agent new task"},
	{"Go to space or tab…", "Tab", "mod.open:picker", "switch jump find"},
	{"Rename tab", "Tab", "tab.rename", "tab"},
	{"Close tab", "Tab", "tab.close", "tab kill"},
	{"Next tab", "Tab", "tab.next", "tab"},
	{"Previous tab", "Tab", "tab.prev", "tab"},
	{"Split right", "Pane", "pane.split:right", "vertical"},
	{"Split down", "Pane", "pane.split:down", "horizontal"},
	{"Close pane", "Pane", "pane.close", "kill"},
	{"Zoom pane", "Pane", "pane.zoom", "maximize fullscreen"},
	{"Focus pane left", "Pane", "pane.go:left", "move go"},
	{"Focus pane right", "Pane", "pane.go:right", "move go"},
	{"Focus pane up", "Pane", "pane.go:up", "move go"},
	{"Focus pane down", "Pane", "pane.go:down", "move go"},
	{"Swap pane left", "Pane", "pane.swap:left", "move"},
	{"Swap pane right", "Pane", "pane.swap:right", "move"},
	{"Swap pane up", "Pane", "pane.swap:up", "move"},
	{"Swap pane down", "Pane", "pane.swap:down", "move"},
	{"New space", "Space", "space.new", "workspace"},
	{"Rename space", "Space", "space.rename", "workspace"},
	{"Close space", "Space", "space.close", "workspace kill"},
	{"Next space", "Space", "space.next", "workspace"},
	{"Previous space", "Space", "space.prev", "workspace"},
	{"Next layout", "Layout", "layout.switch", "look switch"},
	{"New terminal", "Terminal", "terminal.open", "shell"},
	{"Find in terminal", "Terminal", "terminal.find", "search output scrollback"},
	{"Find next match", "Terminal", "terminal.find_next", "search output older"},
	{"Find previous match", "Terminal", "terminal.find_previous", "search output newer"},
	{"Copy", "Terminal", "terminal.copy", "clipboard"},
	{"Paste", "Terminal", "terminal.paste", "clipboard"},
	{"Next agent", "Agent", "agent.next", ""},
	{"Previous agent", "Agent", "agent.prev", ""},
	{"Minimize window", "Window", "window.minimize", ""},
	{"Maximize window", "Window", "window.maximize", "restore"},
	{"Close window", "Window", "window.close", "quit"},
	{"Settings", "App", "app.settings", "preferences options config look font theme sound"},
	{"Keys", "App", "app.help", "keybindings shortcuts help"},
	{"Welcome", "App", "app.welcome", "intro start onboarding"},
	{"Zoom in", "App", "app.zoom_in", "bigger font size"},
	{"Zoom out", "App", "app.zoom_out", "smaller font size"},
	{"Reset zoom", "App", "app.zoom_reset", "font size"},
	{"Quit Rove", "App", "app.quit", "exit"},
	{"Theme: graphite", "App", "app.theme:graphite", "colors look"},
	{"Theme: ink", "App", "app.theme:ink", "colors look"},
	{"Theme: moss", "App", "app.theme:moss", "colors look"},
	{"Theme: paper", "App", "app.theme:paper", "colors look"},
}, keroThemeCommands()...)

func keroThemeCommands() []paletteCommand {
	var commands []paletteCommand
	for _, name := range keroThemes {
		commands = append(commands, paletteCommand{"Theme: " + name, "App", "app.theme:" + name, "colors look"})
	}
	return commands
}

// paletteGroupOrder is the palette's grouping.
var paletteGroupOrder = []string{"Tab", "Pane", "Space", "Layout", "Terminal", "Agent", "Window", "App"}

// paletteRows returns the entries matching the query, in order.
func (a *App) paletteRows() []paletteCommand {
	commands := make([]paletteCommand, len(paletteCommands))
	for i, command := range paletteCommands {
		command.Keyword += " " + command.Title + " " + command.Group
		if strings.HasPrefix(command.Title, "Theme: ") {
			command.Title = a.tr("Theme: %s", strings.TrimPrefix(command.Title, "Theme: "))
		} else {
			command.Title = a.tr(command.Title)
		}
		command.Group = a.tr(command.Group)
		commands[i] = command
	}
	q := strings.ToLower(strings.TrimSpace(a.palette.Query))
	if q == "" {
		var out []paletteCommand
		for _, group := range paletteGroupOrder {
			for _, cmd := range commands {
				if cmd.Group == a.tr(group) {
					out = append(out, cmd)
				}
			}
		}
		return out
	}
	q = strings.ReplaceAll(q, " ", "")
	type scored struct {
		cmd   paletteCommand
		score int
	}
	var hits []scored
	for _, cmd := range commands {
		if score, ok := fuzzyScore(q, strings.ToLower(cmd.Title)); ok {
			hits = append(hits, scored{cmd, score + 10})
			continue
		}
		if score, ok := fuzzyScore(q, strings.ToLower(cmd.Group+" "+cmd.Title+" "+cmd.Keyword)); ok {
			hits = append(hits, scored{cmd, score})
		}
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	out := make([]paletteCommand, len(hits))
	for i, h := range hits {
		out[i] = h.cmd
	}
	return out
}

// fuzzyScore matches the letters of q in order in text.
func fuzzyScore(q, text string) (int, bool) {
	score := 0
	at := 0
	run := 0
	for _, r := range q {
		i := strings.IndexRune(text[at:], r)
		if i < 0 {
			return 0, false
		}
		i += at
		start := i == 0 || strings.ContainsRune(" :.-_/", rune(text[i-1]))
		if i == at && run > 0 {
			run++
		} else {
			run = 0
		}
		score++
		if start {
			score += 6
		}
		score += run * 3
		at = i + utf8.RuneLen(r)
	}
	return score, true
}

// overlays draws the toast stack and whatever panel is over the window.
func (a *App) overlays(c *ui.Context) {
	defer a.toasts(c)
	if a.overlay == "" {
		return
	}
	ui.Overlay(c, func() {
		backdrop := ui.Box(c).Absolute().Fill()
		switch a.overlay {
		case "palette":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.paletteOverlay(c)
		case "welcome":
			a.welcomeOverlay(c)
		case "machine-add":
			if st := a.addMachineSnapshot(); st.term == "" && backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.machineAddOverlay(c)
		case "settings":
			if a.T.Dark {
				backdrop.Background(ui.RGBA(8, 9, 11, 0.50))
			} else {
				backdrop.Background(ui.RGBA(20, 21, 24, 0.30))
			}
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.settingsOverlay(c)
		case "help":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.helpOverlay(c)
		case "picker":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.pickerOverlay(c)
		case "new-menu":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.newMenuOverlay(c)
		case "menu":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.contextMenuOverlay(c)
		case "close-confirm":
			backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
			a.closeConfirmOverlay(c)
		case "prompt":
			backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
			a.promptOverlay(c)
		case "files":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.filesOverlay(c)
		case "peek":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.peekOverlay(c)
		case "changes":
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
			a.changesOverlay(c)
		default:
			// A name with no screen would leave a backdrop that eats clicks.
			if backdrop.Clicked() {
				a.CloseOverlay()
			}
		}
	})
}

// card is a floating card the app draws for its own screens.
func (a *App) card(c *ui.Context, width, height float32) ui.Element {
	t := a.T
	return ui.Column(c).Width(width).Height(height).Radius(12).Background(t.Popover).
		Border(1, t.Border).Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.45)).Clip()
}

// paletteOverlay is the command palette: a search line and every command.
func (a *App) paletteOverlay(c *ui.Context) {
	t := a.T
	rows := a.paletteRows()
	if a.palette.At >= len(rows) {
		a.palette.At = maxInt(0, len(rows)-1)
	}
	width, height := float32(620), a.H*0.85
	panel := a.card(c, width, height).Absolute().Top(60).Left((a.W - width) / 2).Padding(18).Gap(10)
	panel.Children(func() {
		field := ui.Row(c).Height(36).PaddingX(10).Gap(10).Radius(9).Border(1, t.Accent).Background(t.Background).AlignItems(ui.Center)
		field.Children(func() {
			a.icon(c, "command", 16, t.Dim)
			input := ui.TextInputBase(c, &a.palette.Query).Grow(1).FontSize(a.fontSize(12.8)).TextColor(t.Foreground).
				Placeholder(a.tr("Type a command…")).AutoFocus()
			input.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown {
					return false
				}
				switch ev.Key {
				case ui.KeyDown:
					a.palette.At = minInt(a.palette.At+1, len(rows)-1)
					a.Repaint()
					return true
				case ui.KeyUp:
					a.palette.At = maxInt(a.palette.At-1, 0)
					a.Repaint()
					return true
				}
				return false
			})
			if input.Submitted() && len(rows) > 0 {
				a.CloseOverlay()
				a.Run(rows[a.palette.At].Action)
			}
			ui.Text(c, "Esc").FontSize(a.fontSize(11)).TextColor(t.Dim)
		})
		if len(rows) == 0 {
			ui.Box(c).Padding(16, 0, 14, 0).Children(func() {
				ui.Text(c, a.tr("Nothing matches %q.", a.palette.Query)).FontSize(a.fontSize(13)).TextColor(t.Dim)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				last := ""
				for i, cmd := range rows {
					if a.palette.Query == "" && cmd.Group != last {
						last = cmd.Group
						ui.Row(c).Height(26).FillWidth().Padding(0, 14, 4, 14).AlignItems(ui.End).Children(func() {
							ui.Text(c, cmd.Group).FontSize(a.fontSize(11)).FontWeight(600).TextColor(t.Dim)
						})
					}
					row := ui.Row(c).Height(32).FillWidth().MarginX(6).PaddingX(10).Gap(10).Radius(7).AlignItems(ui.Center)
					if i == a.palette.At {
						row.Background(t.Selected)
					} else if row.Hovered() {
						row.Background(t.Hover)
					}
					row.Cursor(ui.CursorPointer)
					row.Children(func() {
						ui.Text(c, cmd.Title).FontSize(a.fontSize(12.8)).TextColor(t.Foreground).Grow(1).Ellipsis("…")
						if a.palette.Query != "" {
							ui.Text(c, cmd.Group).FontSize(a.fontSize(11.5)).TextColor(t.Dim)
						}
						if keys := KeyFor(cmd.Action); keys != "" {
							a.keyChips(c, keys)
						}
					})
					if row.Clicked() {
						a.CloseOverlay()
						a.Run(cmd.Action)
					}
				}
			})
		})
	})
}

// keyChips draws a binding as the palette shows it.
func (a *App) keyChips(c *ui.Context, keys string) {
	t := a.T
	parts := bindingChips(keys)
	ui.Row(c).Gap(3).Children(func() {
		for _, p := range parts {
			ui.Box(c).PaddingX(5).Radius(4).Border(1, t.Border).Background(t.Raised).Children(func() {
				ui.Text(c, p).FontSize(a.fontSize(11)).TextColor(t.Dim)
			})
		}
	})
}

// bindingChips splits "cmd-shift-p" into the symbols each key shows.
func bindingChips(keys string) []string {
	if strings.HasSuffix(keys, "--") {
		return append(strings.Split(strings.TrimSuffix(keys, "--"), "-"), "-")
	}
	return strings.Split(keys, "-")
}

// welcomeOverlay is the first-run welcome.
func (a *App) welcomeOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(660), float32(600)
	panel := a.card(c, width, height).Absolute().Top((a.H - height) / 2).Left((a.W - width) / 2)
	panel.Children(func() {
		head := ui.Row(c).Height(64).FillWidth().PaddingX(24).Gap(10).AlignItems(ui.Center)
		head.Children(func() {
			a.logo(c, 26)
			ui.Text(c, a.tr("Welcome to Rove")).FontSize(a.fontSize(17)).FontWeight(600).TextColor(t.Foreground)
			ui.Box(c).PaddingX(6).Radius(4).Border(1, t.Border).Children(func() {
				ui.Text(c, a.tr("BETA")).FontSize(a.fontSize(10)).FontWeight(600).TextColor(t.Dim)
			})
			ui.Spacer(c)
			ui.Text(c, a.tr("Drag to move")).FontSize(a.fontSize(12)).TextColor(t.Dim)
			close := ui.Box(c).Size(26, 26).Radius(6).Center()
			if close.Hovered() {
				close.Background(t.Hover)
			}
			close.Cursor(ui.CursorPointer)
			close.Children(func() { a.icon(c, "close", 14, t.Dim) })
			if close.Clicked() {
				a.CloseOverlay()
			}
		})
		ui.Scroll(c).Height(height - 128).Children(func() {
			ui.Column(c).FillWidth().Padding(24, 32, 0, 32).Gap(14).Children(func() {
				ui.Text(c, a.tr("Rove runs your terminals in the background")).FontSize(a.fontSize(21)).FontWeight(600).TextColor(t.Foreground)
				ui.Text(c, a.tr("The terminals and agents you open here don't live in this window. They run in herdr-core, a small background program on your computer. Close Rove, update it, or let it crash, and your agents keep working. Open it again and everything is where you left it, as long as the computer is on.")).
					FontSize(a.fontSize(13.5)).TextColor(t.Muted).Wrap()
				a.welcomeFeature(c, t, "welcome-bot", a.tr("Agents tell you when they need you"),
					a.tr("A ring while one works, “Needs you” when it asks something, a sound when it finishes."))
				a.welcomeFeature(c, t, "welcome-messages", a.tr("Agents can start helpers"),
					a.tr("They show up under the agent that started them. Click one to watch it."))
				a.welcomeFeature(c, t, "layers", a.tr("Your other machines, in the same window"),
					a.tr("A server or a Mac mini runs its own herdr-core, so its agents keep going even while this laptop sleeps."))
				ui.Box(c).Height(1).FillWidth().Background(t.Border)
				ui.Row(c).Height(52).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.tr("Appearance")).FontSize(a.fontSize(13)).TextColor(t.Muted).Width(150)
					a.settingsSegmentedOn(c, appearanceNames, &a.settings.Appearance, a.setAppearance)
				})
				ui.Row(c).Height(52).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.tr("Size")).FontSize(a.fontSize(13)).TextColor(t.Muted).Width(150)
					a.stepper(c, fmt.Sprintf("%.0f%%", a.Zoom*100))
				})
			})
		})
		foot := ui.Row(c).Height(64).FillWidth().PaddingX(24).AlignItems(ui.Center)
		foot.Children(func() {
			for i := 0; i < 4; i++ {
				dot := ui.Box(c).Size(8, 8).Radius(4).MarginX(2)
				if i == 0 {
					dot.Background(t.Foreground)
				} else {
					dot.Background(t.Dim)
				}
			}
			ui.Spacer(c)
			btn := ui.Row(c).Height(36).PaddingX(16).Gap(8).Radius(9).Background(t.Solid).AlignItems(ui.Center)
			btn.Cursor(ui.CursorPointer)
			btn.Children(func() {
				ui.Text(c, a.tr("Next")).FontSize(a.fontSize(13)).FontWeight(600).TextColor(t.SolidForeground)
				a.icon(c, "chevron-right", 14, t.SolidForeground)
			})
			if btn.Clicked() {
				a.CloseOverlay()
			}
		})
	})
}

// welcomeFeature is one row of the welcome's features.
func (a *App) welcomeFeature(c *ui.Context, t *Theme, icon, title, body string) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(34, 34).Radius(8).Background(t.Raised).Center().Children(func() {
			if svg := iconSVG(icon); svg != nil {
				ui.Icon(c, svg).Size(18, 18).TextColor(t.Muted)
			}
		})
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(a.fontSize(13.5)).FontWeight(600).TextColor(t.Foreground)
			ui.Text(c, body).FontSize(a.fontSize(12.5)).TextColor(t.Dim).Wrap()
		})
	})
}

// helpOverlay lists every key binding.
func (a *App) helpOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(640), a.H*0.8
	panel := a.card(c, width, height).Absolute().Top((a.H - height) / 2).Left((a.W - width) / 2).Padding(20).Gap(10)
	panel.Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Text(c, a.tr("Keys")).FontSize(a.fontSize(17)).FontWeight(600).TextColor(t.Foreground).Grow(1)
			close := ui.Box(c).Size(26, 26).Radius(6).Center()
			if close.Hovered() {
				close.Background(t.Hover)
			}
			close.Children(func() { a.icon(c, "close", 14, t.Dim) })
			if close.Clicked() {
				a.CloseOverlay()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, b := range Bindings() {
					if b.Section != "global" {
						continue
					}
					ui.Row(c).Height(28).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, b.Action).FontSize(a.fontSize(12)).TextColor(t.Muted).Grow(1)
						a.keyChips(c, b.Keys)
					})
				}
			})
		})
	})
}

// pickerOverlay goes to a space or tab.
func (a *App) pickerOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(560), float32(480)
	panel := a.card(c, width, height).Absolute().Top(80).Left((a.W - width) / 2).Padding(16).Gap(8)
	panel.Children(func() {
		field := ui.Row(c).Height(36).PaddingX(10).Gap(10).Radius(9).Border(1, t.Accent).Background(t.Background).AlignItems(ui.Center)
		field.Children(func() {
			a.icon(c, "search", 15, t.Dim)
			input := ui.TextInputBase(c, &a.picker.Query).Grow(1).FontSize(a.fontSize(12.8)).TextColor(t.Foreground).
				Placeholder(a.tr("Go to a tab, space or agent…")).AutoFocus()
			if input.Submitted() {
				a.CloseOverlay()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for _, sp := range a.S.Places() {
					name := a.S.SpaceName(sp)
					if a.picker.Query != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(a.picker.Query)) {
						continue
					}
					head := ui.Row(c).Height(28).FillWidth().PaddingX(8).AlignItems(ui.Center)
					head.Children(func() {
						ui.Text(c, name).FontSize(a.fontSize(11.5)).FontWeight(600).TextColor(t.Dim)
					})
					if head.Clicked() {
						a.S.SelectSpace(0, sp.ID)
						a.CloseOverlay()
					}
					for _, tab := range sp.Tabs {
						row := ui.Row(c).Height(30).FillWidth().PaddingX(8).Gap(8).Radius(7).AlignItems(ui.Center)
						if tab.Active {
							row.Background(t.Selected)
						} else if row.Hovered() {
							row.Background(t.Hover)
						}
						row.Cursor(ui.CursorPointer)
						row.Children(func() {
							a.icon(c, "terminal", 14, t.Muted)
							ui.Text(c, herdr.TabName(tab, tab.Index)).FontSize(a.fontSize(13)).TextColor(t.Foreground).Grow(1)
						})
						if row.Clicked() {
							a.S.SelectTab(tab.ID)
							a.CloseOverlay()
						}
					}
				}
			})
		})
	})
}

// newMenuOverlay starts a terminal, a space, or an agent.
func (a *App) newMenuOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(250), float32(300)
	x, y := a.menuX, a.menuY
	if x == 0 && y == 0 {
		x = a.W - width - 10
		y = 72
	} else {
		x -= width - 12
		if x < 8 {
			x = 8
		}
		if y+height > a.H-8 {
			y = a.H - height - 8
		}
	}
	panel := a.card(c, width, height).Absolute().Top(y).Left(x).Padding(5).Gap(0)
	panel.Children(func() {
		item := func(icon, label, hint string, action func()) {
			row := ui.Row(c).Height(31).FillWidth().PaddingX(9).Gap(10).Radius(7).AlignItems(ui.Center)
			if row.Hovered() {
				row.Background(t.Selected)
			}
			row.Cursor(ui.CursorPointer)
			row.Children(func() {
				a.icon(c, icon, 14, t.Foreground)
				ui.Text(c, label).FontSize(a.fontSize(12.8)).TextColor(t.Foreground).Grow(1)
				if hint != "" {
					ui.Text(c, hint).FontSize(a.fontSize(11)).TextColor(t.Dim)
				}
			})
			if row.Clicked() {
				a.CloseOverlay()
				action()
			}
		}
		item("terminal", a.tr("Terminal"), prettyKeys(KeyFor("tab.new")), a.newTab)
		ui.Row(c).Height(26).FillWidth().Padding(8, 9, 0, 9).Children(func() {
			ui.Text(c, a.tr("Start an agent")).FontSize(a.fontSize(11)).TextColor(t.Dim)
		})
		ui.Text(c, a.tr("No agents to start from the app yet.")).FontSize(a.fontSize(12.8)).TextColor(t.Dim).Padding(9, 0, 6, 9)
		ui.Box(c).Height(1).Margin(4, 5).Background(t.Border)
		item("plus", a.tr("New space"), prettyKeys(KeyFor("space.new")), a.newSpace)
	})
}

// contextMenuOverlay is a right-click menu.
func (a *App) contextMenuOverlay(c *ui.Context) {
	t := a.T
	items := a.menuItems
	if len(items) == 0 {
		return
	}
	height := float32(8 + len(items)*30)
	panel := a.card(c, 200, height).Absolute().Top(a.menuY).Left(a.menuX).Padding(4).Gap(0)
	panel.Children(func() {
		for _, item := range items {
			row := ui.Row(c).Height(30).FillWidth().PaddingX(8).Gap(8).Radius(6).AlignItems(ui.Center)
			if row.Hovered() {
				row.Background(t.Hover)
			}
			row.Cursor(ui.CursorPointer)
			row.Children(func() {
				a.icon(c, item.Icon, 14, t.Muted)
				ui.Text(c, item.Label).FontSize(a.fontSize(12.8)).TextColor(t.Foreground).Grow(1)
			})
			action, do := item.Action, item.Do
			if row.Clicked() {
				a.CloseOverlay()
				if do != nil {
					do()
				} else {
					a.Run(action)
				}
			}
		}
	})
}

// openMenuAt shows a menu at a point of the window.
func (a *App) openMenuAt(x, y float32, items []MenuItem) {
	if x+200 > a.W {
		x = a.W - 200
	}
	if y+float32(8+len(items)*30) > a.H {
		y = a.H - float32(8+len(items)*30)
	}
	a.menuX, a.menuY = x, y
	a.menuItems = items
	a.OpenOverlay("menu")
}

// MenuItem is one row of a context menu.
type MenuItem struct {
	Label  string
	Icon   string
	Action string
	Do     func()
}

// promptOverlay asks for a name.
func (a *App) promptOverlay(c *ui.Context) {
	t := a.T
	width := float32(380)
	panel := a.card(c, width, 190).Absolute().Top((a.H - 190) / 2).Left((a.W - width) / 2).Padding(20).Gap(12)
	panel.Children(func() {
		ui.Text(c, a.prompt.Title).FontSize(a.fontSize(15)).FontWeight(600).TextColor(t.Foreground)
		ui.Text(c, a.prompt.Hint).FontSize(a.fontSize(12)).TextColor(t.Dim).Wrap()
		input := ui.TextInput(c, &a.prompt.Value).AutoFocus().FontSize(a.fontSize(13))
		submit := func() {
			action, target := a.prompt.Command, a.prompt.Target
			value := a.prompt.Value
			a.CloseOverlay()
			switch action {
			case "tab.rename":
				a.S.RenameTab(target, value)
			case "space.rename":
				a.S.RenameSpace(target, value)
			}
		}
		if input.Submitted() {
			submit()
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			cancel := ui.Button(c, a.tr("Cancel"))
			if cancel.Clicked() {
				a.CloseOverlay()
			}
			ok := ui.PrimaryButton(c, a.tr("Save"))
			if ok.Clicked() {
				submit()
			}
		})
	})
}

// toasts draws the notes at the bottom right.
func (a *App) toasts(c *ui.Context) {
	a.mu.Lock()
	notes := append([]Toast(nil), a.notes...)
	a.mu.Unlock()
	if len(notes) == 0 {
		return
	}
	t := a.T
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Right(16).Bottom(40).Gap(8).AlignItems(ui.End).Children(func() {
			for _, n := range notes {
				card := ui.Column(c).Width(320).Padding(12).Gap(3).Radius(10).Background(t.Popover).
					Border(1, t.Border).Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.4))
				card.Children(func() {
					ui.Row(c).Gap(8).FillWidth().AlignItems(ui.Center).Children(func() {
						color := t.Foreground
						switch n.Level {
						case "error":
							color = t.Blocked
						case "success":
							color = t.Done
						}
						ui.Text(c, n.Title).FontSize(a.fontSize(12.5)).FontWeight(600).TextColor(color).Grow(1).Ellipsis("…")
						a.iconButton(c, "close", 22, a.tr("Dismiss notification"), func() { a.expireToast(n.id) })
					})
					if n.Body != "" {
						ui.Text(c, n.Body).FontSize(a.fontSize(11.5)).TextColor(t.Muted).Wrap()
					}
				})
			}
		})
	})
}

// filesOverlay and changesOverlay list the shown folder's files.
func (a *App) filesOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(420), a.H*0.7
	panel := a.card(c, width, height).Absolute().Top(80).Left((a.W - width) / 2).Padding(16).Gap(8)
	panel.Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Text(c, a.tr("Files")).FontSize(a.fontSize(16)).FontWeight(600).TextColor(t.Foreground).Grow(1)
			close := ui.Box(c).Size(24, 24).Center()
			close.Children(func() { a.icon(c, "close", 13, t.Dim) })
			if close.Clicked() {
				a.CloseOverlay()
			}
		})
		dir := a.S.FocusedPane().Cwd
		if dir == "" {
			dir = homeDir()
		}
		ui.Text(c, herdr.ShortFolder(dir)).FontSize(a.fontSize(12)).TextColor(t.Dim)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				entries, err := readDir(dir)
				if err != nil {
					ui.Text(c, err.Error()).FontSize(a.fontSize(12)).TextColor(t.Dim)
					return
				}
				for _, e := range entries {
					row := ui.Row(c).Height(26).FillWidth().PaddingX(6).Gap(8).Radius(5).AlignItems(ui.Center)
					if row.Hovered() {
						row.Background(t.Hover)
					}
					row.Children(func() {
						if e.IsDir() {
							a.icon(c, "folder", 13, t.Muted)
						} else {
							a.icon(c, "file", 13, t.Dim)
						}
						ui.Text(c, e.Name()).FontSize(a.fontSize(12.5)).TextColor(t.Muted).Grow(1).Ellipsis("…")
					})
				}
			})
		})
	})
}

func (a *App) changesOverlay(c *ui.Context) {
	t := a.T
	width, height := float32(420), a.H*0.7
	panel := a.card(c, width, height).Absolute().Top(80).Left((a.W - width) / 2).Padding(16).Gap(8)
	panel.Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Text(c, a.tr("Changes")).FontSize(a.fontSize(16)).FontWeight(600).TextColor(t.Foreground).Grow(1)
			close := ui.Box(c).Size(24, 24).Center()
			close.Children(func() { a.icon(c, "close", 13, t.Dim) })
			if close.Clicked() {
				a.CloseOverlay()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				dir := a.S.FocusedPane().Cwd
				if dir == "" {
					dir = homeDir()
				}
				lines, err := gitStatus(dir)
				if err != nil || len(lines) == 0 {
					ui.Text(c, a.tr("No changes.")).FontSize(a.fontSize(12.5)).TextColor(t.Dim)
					return
				}
				for _, line := range lines {
					ui.Text(c, line).Font("monospace").FontSize(a.fontSize(11.5)).TextColor(t.Muted)
				}
			})
		})
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
