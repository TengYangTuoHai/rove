package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// settingsEntry is one searchable setting. The search and the sections both
// use the same names, so a result and its row say the same thing.
type settingsEntry struct {
	Section     string
	SectionName string
	Title       string
	About       string
	Value       string
	Icon        string
	SearchText  string
}

var (
	appearanceNames    = []string{"System", "Light", "Dark"}
	optionAltNames     = []string{"Auto", "Left", "Right", "Off"} // legacy saved values; control hidden until MyGo supports them
	updateChannelNames = []string{"Stable", "Preview"}            // legacy saved values; controls hidden until an updater exists
)

// settingsSectionNames labels every section, in the nav's order.
var settingsSectionNames = map[string]string{
	"look": "Look", "terminal": "Terminal", "machines": "Machines",
	"alerts": "Alerts", "keys": "Keyboard shortcuts",
}

// settingsSectionIcons is the icon each section's rows and results carry.
var settingsSectionIcons = map[string]string{
	"look": "sun", "terminal": "terminal", "machines": "server",
	"alerts": "bell", "keys": "command",
}

// settingKeywords are what a person might type to find a setting whose name
// does not contain the word itself.
var settingKeywords = map[string]string{
	"Language":         "locale language translation 中文 日本語",
	"Theme":            "appearance accent colors look",
	"Appearance":       "theme light dark system mode",
	"Accent":           "theme color colors highlight",
	"Interface font":   "font typeface size type",
	"Terminal font":    "font typeface mono terminal",
	"Zoom":             "size scale bigger smaller",
	"Clipboard":        "copy paste",
	"Minimum contrast": "contrast readability",
	"Machines":         "ssh host server remote",
	"Done":             "alert sound notification finish complete",
	"Needs you":        "alert sound notification approval question",
	"Play a sound":     "sound audio alert",
	"Done sound":       "sound audio alert",
	"Needs you sound":  "sound audio alert",
	"By agent":         "sound agent override",
}

func onOff(on bool) string {
	if on {
		return "On"
	}
	return "Off"
}

func pick(names []string, at int) string {
	if at < 0 || at >= len(names) {
		return names[0]
	}
	return names[at]
}

// settingsEntries is every setting, with the value it shows now.
func (a *App) settingsEntries() []settingsEntry {
	s := a.settings
	zoom := a.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	entry := func(section, title, about, value string) settingsEntry {
		return settingsEntry{
			Section: section, SectionName: a.tr(settingsSectionNames[section]),
			Title: a.tr(title), About: a.tr(about), Value: value,
			SearchText: title + " " + settingsSectionNames[section] + " " + settingKeywords[title],
			Icon:       settingsSectionIcons[section],
		}
	}
	return []settingsEntry{
		entry("look", "Language", "Follow the system, or choose a language.", a.languageName()),
		entry("look", "Appearance", "System, or always light or dark.", a.tr(pick(appearanceNames, s.Appearance))),
		entry("look", "Theme", "Colors for the chrome and every terminal.", themeLabel(a.currentThemeName())),
		entry("look", "Accent", "Focus rings, selections and key hints.", a.tr(accentLabel(s.Accent))),
		entry("look", "Interface font", "The panels’ typeface and size.", fmt.Sprintf("%s %d", s.InterfaceFont, s.InterfaceSize)),
		entry("look", "Terminal font", "Used by every pane, on every machine.", fmt.Sprintf("%s %d", s.TerminalFont, s.TerminalSize)),
		entry("look", "Zoom", "Scales the whole window, terminals included.", fmt.Sprintf("%.0f%%", zoom*100)),

		entry("terminal", "Clipboard", "Let terminal programs write the clipboard.", a.tr(onOff(s.Clipboard))),
		entry("terminal", "Minimum contrast", "Raise terminal text contrast. 1.0 leaves program colors alone.", fmt.Sprintf("%.1f", s.MinContrast)),

		entry("keys", "Keyboard shortcuts", "Change the keys for app actions.", a.tr("Manage")),

		entry("machines", "Machines", "Add, connect and remove the computers Rove reaches over ssh.", a.tr("Manage")),

		entry("alerts", "Done", "A system notification, with optional sound, when an agent finishes while you are elsewhere.", a.tr(onOff(s.AlertDone))),
		entry("alerts", "Needs you", "When an agent asks a question or waits for approval.", a.tr(onOff(s.AlertNeeds))),
		entry("alerts", "Play a sound", "Use the built-in sounds, or choose your own files.", a.tr(onOff(s.AlertSound))),
		entry("alerts", "Done sound", "Plays when an agent finishes.", alertSoundLabel(s.DoneSound)),
		entry("alerts", "Needs you sound", "Plays when an agent asks something.", alertSoundLabel(s.NeedsSound)),
		entry("alerts", "By agent", "Turn a sound off for one agent without changing the rest.", a.agentSoundSummary()),
	}
}

// settingsResults is the entries matching the search, best first.
func (a *App) settingsResults() []settingsEntry {
	all := a.settingsEntries()
	q := strings.ToLower(strings.TrimSpace(a.settings.Search))
	if q == "" {
		return all
	}
	q = strings.ReplaceAll(q, " ", "")
	type scored struct {
		entry settingsEntry
		score int
	}
	var hits []scored
	for _, e := range all {
		if score, ok := fuzzyScore(q, strings.ToLower(e.Title)); ok {
			hits = append(hits, scored{e, score + 10})
			continue
		}
		haystack := strings.ToLower(e.SectionName + " " + e.SearchText)
		if score, ok := fuzzyScore(q, haystack); ok {
			hits = append(hits, scored{e, score})
		}
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	out := make([]settingsEntry, len(hits))
	for i, h := range hits {
		out[i] = h.entry
	}
	return out
}

// settingsMove keeps the search selection on a result.
func (a *App) settingsMove(delta int) {
	n := len(a.settingsResults())
	if n == 0 {
		a.settings.SearchAt = 0
		return
	}
	at := a.settings.SearchAt + delta
	if at < 0 {
		at = 0
	}
	if at >= n {
		at = n - 1
	}
	a.settings.SearchAt = at
}

// chooseSettingsResult opens the section the chosen result lives in.
func (a *App) chooseSettingsResult(i int) {
	results := a.settingsResults()
	if i < 0 || i >= len(results) {
		return
	}
	a.settings.Section = results[i].Section
	a.settings.SearchOpen = false
	a.settings.Search = ""
	a.settings.SearchAt = 0
	a.Repaint()
}

// openSettingsSearch starts a search with an empty field.
func (a *App) openSettingsSearch() {
	if a.settings.SearchOpen {
		return
	}
	a.settings.SearchOpen = true
	a.settings.Search = ""
	a.settings.SearchAt = 0
	a.Repaint()
}

// closeSettingsSearch returns to the section alone.
func (a *App) closeSettingsSearch() {
	a.settings.SearchOpen = false
	a.settings.Search = ""
	a.settings.SearchAt = 0
	a.Repaint()
}

const (
	settingsWidth     = float32(1080)
	settingsHeight    = float32(720)
	settingsNavWidth  = float32(224)
	settingsCardAlpha = float32(0.035)
)

func minFloat(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// settingsOverlay is the Settings screen: a header, a section nav and the
// chosen section's controls.
func (a *App) settingsOverlay(c *ui.Context) {
	t := a.T
	if _, ok := settingsSectionNames[a.settings.Section]; !ok {
		a.settings.Section = "look"
	}
	width := minFloat(settingsWidth, a.W-80)
	height := minFloat(settingsHeight, a.H-40)
	panel := a.card(c, width, height).Absolute().
		Top((a.H - height) / 2).Left((a.W - width) / 2)
	panel.Children(func() {
		a.settingsHeader(c)
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Start).Children(func() {
			a.settingsNav(c)
			ui.Box(c).Width(1).FillHeight().Background(t.Border)
			a.settingsContent(c)
		})
	})
}

// settingsHeader is the title, the search field and the close button.
func (a *App) settingsHeader(c *ui.Context) {
	t := a.T
	ui.Row(c).Height(58).FillWidth().Padding(0, 16, 0, 22).Gap(12).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.tr("Settings")).FontSize(a.fontSize(15.5)).FontWeight(600).TextColor(t.Foreground)
		ui.Spacer(c)
		a.settingsSearchField(c)
		close := ui.Box(c).Size(28, 28).Radius(7).Center()
		if close.Hovered() {
			close.Background(t.Hover)
		}
		close.Cursor(ui.CursorPointer)
		close.Children(func() { a.icon(c, "x", 14, t.Dim) })
		if close.Clicked() {
			a.CloseOverlay()
		}
	})
}

// settingsSearchField is the field in the header, or a button that opens it.
func (a *App) settingsSearchField(c *ui.Context) {
	t := a.T
	field := ui.Row(c).Width(230).Height(30).PaddingX(9).Gap(8).Radius(8).
		Border(1, t.Border).Background(Alpha(t.Foreground, 0.04)).
		AlignItems(ui.Center)
	if a.settings.SearchOpen {
		field.Border(1, t.Accent).Background(t.Background)
		field.Children(func() {
			a.icon(c, "search", 14, t.Dim)
			input := ui.TextInputBase(c, &a.settings.Search).Grow(1).FontSize(a.fontSize(12.5)).
				TextColor(t.Foreground).Placeholder(a.tr("Search settings")).AutoFocus()
			input.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown {
					return false
				}
				switch ev.Key {
				case ui.KeyDown:
					a.settingsMove(1)
					a.Repaint()
					return true
				case ui.KeyUp:
					a.settingsMove(-1)
					a.Repaint()
					return true
				}
				return false
			})
			if input.Changed() {
				a.settings.SearchAt = 0
			}
			if input.Submitted() {
				a.chooseSettingsResult(a.settings.SearchAt)
			}
			a.settingsKeycap(c, "esc")
		})
		return
	}
	if field.Hovered() {
		field.Background(t.Hover)
	}
	field.Cursor(ui.CursorPointer)
	field.Children(func() {
		a.icon(c, "search", 14, t.Dim)
		ui.Text(c, a.tr("Search settings")).FontSize(a.fontSize(12.5)).TextColor(t.Dim).Grow(1)
		a.settingsKeycap(c, "⌘F")
	})
	if field.Clicked() {
		a.openSettingsSearch()
	}
}

// settingsKeycap is a small key hint.
func (a *App) settingsKeycap(c *ui.Context, label string) {
	t := a.T
	ui.Box(c).Height(16).PaddingX(4).Radius(4).Border(1, t.Border).
		Background(Alpha(t.Foreground, 0.04)).Center().Children(func() {
		ui.Text(c, label).FontSize(a.fontSize(10)).TextColor(t.Dim)
	})
}

// settingsNav is the sections list.
func (a *App) settingsNav(c *ui.Context) {
	t := a.T
	// Rows are 2 px apart, as the sidebar's are, so one row's hover or
	// selection never runs into the next.
	nav := ui.Column(c).Width(settingsNavWidth).FillHeight().Background(t.Chrome).
		Padding(8, 10, 12, 10).Gap(2)
	nav.Children(func() {
		navGroup := func(label string) {
			// The group keeps its 6 px of air under it: the column's gap
			// adds to this padding.
			ui.Text(c, strings.ToUpper(label)).FontSize(a.fontSize(10.5)).FontWeight(600).
				LetterSpacing(0.6).TextColor(t.Dim).Padding(14, 10, 4, 10)
		}
		navRow := func(id, label, icon string) {
			on := a.settings.Section == id
			row := ui.Row(c).Height(34).FillWidth().PaddingX(10).Gap(10).
				Radius(8).AlignItems(ui.Center)
			if on {
				row.Background(t.Selected)
			} else if row.Hovered() {
				row.Background(t.Hover)
			}
			row.Cursor(ui.CursorPointer)
			row.Children(func() {
				color, iconColor := t.Muted, t.Dim
				if on {
					color, iconColor = t.Foreground, t.Accent
				}
				a.icon(c, icon, 15, iconColor)
				ui.Text(c, label).FontSize(a.fontSize(13.5)).TextColor(color)
			})
			if row.Clicked() {
				a.settings.Section = id
				a.settings.KeyAction = ""
				a.settings.SearchOpen = false
				a.settings.Search = ""
				a.Repaint()
			}
		}

		navGroup(a.tr("App"))
		navRow("look", a.tr("Look"), "sun")
		navRow("terminal", a.tr("Terminal"), "terminal")
		navRow("keys", a.tr("Keyboard shortcuts"), "command")
		navGroup("Rove")
		navRow("machines", a.tr("Machines"), "server")
		navRow("alerts", a.tr("Alerts"), "bell")

		ui.Spacer(c)
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Row(c).Height(42).FillWidth().PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
			a.logo(c, 18)
			ui.Text(c, "Rove").FontSize(a.fontSize(11.5)).TextColor(t.Muted)
			ui.Spacer(c)
			version := mygo.App.Version()
			if version == "" {
				version = "dev"
			}
			ui.Text(c, version).Font(TerminalFont).FontSize(a.fontSize(10.5)).TextColor(t.Dim)
		})
	})
}

// settingsContent is the scrolling section, with search over it when open.
func (a *App) settingsContent(c *ui.Context) {
	area := ui.Column(c).Grow(1).FillHeight()
	area.Children(func() {
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(26, 32).Gap(14).Children(func() {
				a.settingsSection(c)
			})
		})
		if a.settings.SearchOpen {
			shade := ui.Box(c).Absolute().Fill()
			if a.T.Dark {
				shade.Background(ui.RGBA(18, 19, 22, 0.58))
			} else {
				shade.Background(ui.RGBA(245, 245, 241, 0.68))
			}
			a.settingsSearchResults(c)
		}
	})
}

// settingsSearchResults is the results card under the search field.
func (a *App) settingsSearchResults(c *ui.Context) {
	t := a.T
	results := a.settingsResults()
	if a.settings.SearchAt >= len(results) {
		a.settings.SearchAt = maxInt(0, len(results)-1)
	}
	shadow := ui.RGBA(0, 0, 0, 0.55)
	if !t.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.18)
	}
	pop := ui.Column(c).Absolute().Top(10).Right(56).Width(404).Radius(12).
		Background(t.Popover).Border(1, t.Input).Shadow(0, 18, 50, 0, shadow).Clip()
	pop.Children(func() {
		ui.Row(c).FillWidth().Padding(10, 12, 7, 12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("%d RESULTS", len(results))).FontSize(a.fontSize(10)).
				FontWeight(600).LetterSpacing(0.5).TextColor(t.Dim)
			ui.Spacer(c)
			if a.settings.Search != "" {
				ui.Text(c, a.settings.Search).FontSize(a.fontSize(10)).TextColor(t.Dim)
			}
		})
		if len(results) == 0 {
			ui.Text(c, a.tr("Nothing matches %q.", a.settings.Search)).
				FontSize(a.fontSize(12.5)).TextColor(t.Dim).Padding(14, 12)
		} else {
			ui.Scroll(c).MaxHeight(288).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for i, e := range results {
						row := ui.Row(c).Height(36).FillWidth().PaddingX(12).Gap(10).
							AlignItems(ui.Center)
						if i == a.settings.SearchAt {
							row.Background(t.Selected)
							row.ScrollIntoView()
						} else if row.Hovered() {
							row.Background(t.Hover)
						}
						row.Cursor(ui.CursorPointer)
						row.Children(func() {
							color, title := t.Dim, t.Muted
							if i == a.settings.SearchAt {
								color, title = t.Accent, t.Foreground
							}
							a.icon(c, e.Icon, 14, color)
							ui.Text(c, e.Title).FontSize(a.fontSize(12.5)).TextColor(title).Grow(1).
								Ellipsis("…")
							ui.Text(c, e.SectionName).FontSize(a.fontSize(10.5)).TextColor(t.Dim)
						})
						if row.Clicked() {
							a.chooseSettingsResult(i)
						}
					}
				})
			})
		}
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Row(c).FillWidth().Padding(7, 12).Gap(7).AlignItems(ui.Center).Children(func() {
			a.settingsKeycap(c, "↑↓")
			ui.Text(c, a.tr("move")).FontSize(a.fontSize(10.5)).TextColor(t.Dim)
			a.settingsKeycap(c, "↵")
			ui.Text(c, a.tr("open")).FontSize(a.fontSize(10.5)).TextColor(t.Dim)
			ui.Spacer(c)
			a.settingsKeycap(c, "esc")
			ui.Text(c, a.tr("close")).FontSize(a.fontSize(10.5)).TextColor(t.Dim)
		})
	})
}

// settingsSection draws the chosen section.
func (a *App) settingsSection(c *ui.Context) {
	switch a.settings.Section {
	case "keys":
		a.settingsKeys(c)
	case "terminal":
		a.settingsTerminal(c)
	case "machines":
		a.settingsMachines(c)
	case "alerts":
		a.settingsAlerts(c)
	default:
		a.settingsLook(c)
	}
}

// settingsHead is a section's title and one line about it.
func (a *App) settingsHead(c *ui.Context, title, about string) {
	t := a.T
	ui.Column(c).Gap(4).Padding(0, 0, 4, 0).Children(func() {
		ui.Text(c, title).FontSize(a.fontSize(21)).FontWeight(600).TextColor(t.Foreground)
		ui.Text(c, about).FontSize(a.fontSize(12.5)).TextColor(t.Muted).Wrap()
	})
}

// settingsCard is one group of rows.
func (a *App) settingsCard(c *ui.Context, rows func()) {
	t := a.T
	card := ui.Column(c).FillWidth().Radius(12).Border(1, t.Border).
		Background(Alpha(t.Foreground, settingsCardAlpha)).Clip()
	card.Children(rows)
}

// settingsDivider separates rows inside a card.
func (a *App) settingsDivider(c *ui.Context) {
	ui.Box(c).Height(1).FillWidth().MarginX(16).Background(a.T.Border)
}

// settingsRow is a label, a line about it, and its control.
func (a *App) settingsRow(c *ui.Context, label, about string, control func()) {
	a.settingsRowWhen(c, true, label, about, control)
}

// settingsRowWhen draws a row that fades when its setting is off.
func (a *App) settingsRowWhen(c *ui.Context, on bool, label, about string, control func()) {
	t := a.T
	labelColor := t.Foreground
	if !on {
		labelColor = t.Dim
	}
	ui.Row(c).FillWidth().MinHeight(58).Padding(11, 16).Gap(24).
		AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(a.fontSize(13.5)).FontWeight(500).TextColor(labelColor)
			if about != "" {
				ui.Text(c, about).FontSize(a.fontSize(11.5)).TextColor(t.Dim).Wrap()
			}
		})
		ctl := ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0)
		if !on {
			ctl.Opacity(0.45)
		}
		ctl.Children(control)
	})
}

// settingsNote is the small line under a section's cards.
func (a *App) settingsNote(c *ui.Context, icon, text string) {
	t := a.T
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(0, 2).Children(func() {
		a.icon(c, icon, 12, t.Dim)
		ui.Text(c, text).FontSize(a.fontSize(11)).TextColor(t.Dim).Wrap().Grow(1)
	})
}

// settingsToggle is an on/off switch.
func (a *App) settingsToggle(c *ui.Context, on *bool) {
	t := a.T
	track := ui.Box(c).Width(36).Height(21).Radius(11).Cursor(ui.CursorPointer)
	if *on {
		track.Background(t.Accent)
	} else {
		track.Background(Alpha(t.Foreground, 0.17))
	}
	track.Children(func() {
		ui.Row(c).Fill().Padding(2).AlignItems(ui.Center).Children(func() {
			if *on {
				ui.Spacer(c)
			}
			knob := ui.Box(c).Size(17, 17).Radius(9)
			if *on {
				knob.Background(t.AccentForeground)
			} else {
				knob.Background(t.Foreground)
			}
			if !*on {
				ui.Spacer(c)
			}
		})
	})
	if track.Clicked() {
		*on = !*on
		a.Repaint()
	}
}

// settingsSegmented is a row of mutually exclusive labels.
func (a *App) settingsSegmented(c *ui.Context, labels []string, value *int) {
	a.settingsSegmentedOn(c, labels, value, nil)
}

// settingsSegmentedOn is settingsSegmented that runs onChange with the
// index picked, for a setting that applies at once.
func (a *App) settingsSegmentedOn(c *ui.Context, labels []string, value *int, onChange func(int)) {
	t := a.T
	ui.Row(c).Padding(2).Radius(8).Gap(2).Background(Alpha(t.Foreground, 0.055)).
		AlignItems(ui.Center).Children(func() {
		for i, label := range labels {
			item := ui.Box(c).Height(26).PaddingX(12).Radius(6).Center().
				Cursor(ui.CursorPointer)
			if i == *value {
				item.Background(Alpha(t.Foreground, 0.12))
			}
			item.Children(func() {
				color := t.Muted
				if i == *value {
					color = t.Foreground
				}
				ui.Text(c, a.tr(label)).FontSize(a.fontSize(12)).FontWeight(500).TextColor(color)
			})
			if item.Clicked() {
				*value = i
				if onChange != nil {
					onChange(i)
				}
				a.Repaint()
			}
		}
	})
}

// themeSource is the mygo appearance an Appearance setting asks for, so
// the window's own chrome follows it too.
func themeSource(appearance int) mygo.ThemeSource {
	switch appearance {
	case AppearanceLight:
		return mygo.ThemeLight
	case AppearanceDark:
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// StartAppearance gives the window the appearance the settings keep, and
// the theme that matches, before it first shows.
func (a *App) StartAppearance() {
	mygo.Theme.SetSource(themeSource(a.settings.Appearance))
	if a.settings.Appearance == AppearanceSystem {
		// The source is the system's: read it now, as the desktop may
		// be the other way around from when the settings were written.
		a.SetSystemAppearance(mygo.Theme.IsDark())
	}
	a.applyTheme()
}

// setAppearance makes the window follow the desktop, or always light or
// dark, and applies the theme that matches.
func (a *App) setAppearance(n int) {
	a.settings.Appearance = n
	mygo.Theme.SetSource(themeSource(n))
	if n == AppearanceSystem {
		// Back to the system's: read it again, as it may have changed
		// while the window forced light or dark.
		a.systemKnown = false
		a.SetSystemAppearance(mygo.Theme.IsDark())
	}
	a.applyTheme()
}

// stepper draws − 100% +.
func (a *App) stepper(c *ui.Context, label string) {
	t := a.T
	ui.Row(c).Height(30).Padding(2).Radius(8).Background(Alpha(t.Foreground, 0.055)).
		Gap(2).AlignItems(ui.Center).Children(func() {
		minus := ui.ButtonBase(c.Key("zoom-out")).Width(26).Height(26).Radius(6).Center().Label(a.tr("Zoom out")).Cursor(ui.CursorPointer)
		if minus.Hovered() {
			minus.Background(t.Hover)
		}
		minus.Children(func() { ui.Text(c, "−").FontSize(a.fontSize(14)).TextColor(t.Muted) })
		if minus.Clicked() {
			a.zoomBy(1 / 1.1)
		}
		ui.Text(c, label).FontSize(a.fontSize(12.5)).TextColor(t.Foreground).Width(52).
			TextAlign(ui.Center)
		plus := ui.ButtonBase(c.Key("zoom-in")).Width(26).Height(26).Radius(6).Center().Label(a.tr("Zoom in")).Cursor(ui.CursorPointer)
		if plus.Hovered() {
			plus.Background(t.Hover)
		}
		plus.Children(func() { ui.Text(c, "+").FontSize(a.fontSize(14)).TextColor(t.Muted) })
		if plus.Clicked() {
			a.zoomBy(1.1)
		}
	})
}

// settingsButton is a primary, secondary or small button.
func (a *App) settingsButton(c *ui.Context, label string, solid, small bool, onClick func()) {
	t := a.T
	h, pad, size := float32(30), float32(12), float32(12)
	if small {
		h, pad, size = 26, 10, 11.5
	}
	b := ui.Row(c).Height(h).PaddingX(pad).Radius(8).Gap(6).AlignItems(ui.Center)
	if solid {
		b.Background(t.Solid)
	} else {
		b.Background(Alpha(t.Foreground, 0.05)).Border(1, t.Input)
	}
	if onClick != nil {
		b.Cursor(ui.CursorPointer)
		if b.Hovered() && !solid {
			b.Background(t.Hover)
		}
	}
	b.Children(func() {
		color := t.Foreground
		if solid {
			color = t.SolidForeground
		}
		if small {
			color = t.Muted
		}
		ui.Text(c, label).FontSize(a.fontSize(size)).FontWeight(500).TextColor(color)
	})
	if b.Clicked() && onClick != nil {
		onClick()
	}
}

// settingsLink is a quiet, accent-colored action.
func (a *App) settingsLink(c *ui.Context, label string, onClick func()) {
	t := a.T
	text := ui.Text(c, label).FontSize(a.fontSize(12)).TextColor(t.Accent)
	if onClick != nil {
		text.Cursor(ui.CursorPointer)
		if text.Clicked() {
			onClick()
		}
	}
}

func accentLabel(accent string) string {
	if accent == "" {
		return "Theme"
	}
	return strings.ToUpper(accent)
}

// settingsDots is the accent color chooser.
func (a *App) settingsDots(c *ui.Context) {
	t := a.T
	colors := []string{"#96b2d7", "#e5c07b", "#f0705f", "#a6bd91", "#b5a2ce"}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		a.settingsLink(c, a.tr("Theme"), func() { a.settings.Accent = ""; a.applyTheme() })
		for _, hex := range colors {
			hex := hex
			wrap := ui.Box(c).Size(24, 24).Radius(12).Center().Cursor(ui.CursorPointer)
			if a.settings.Accent == hex {
				wrap.Border(2, t.Accent)
			}
			wrap.Children(func() {
				ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex(hex))
			})
			if wrap.Clicked() {
				a.settings.Accent = hex
				a.applyTheme()
			}
		}
	})
}

// settingsLook is the Look section.
func (a *App) settingsLook(c *ui.Context) {
	t := a.T
	a.settingsHead(c, a.tr("Look"), a.tr("Theme, accent and type. Everything applies as you change it."))

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Language"), a.tr("Follow the system, or choose a language."), func() { a.settingsLanguage(c) })
		a.settingsDivider(c)
		a.settingsRow(c, a.tr("Appearance"), a.tr("Follow the system, or always use light or dark."), func() {
			a.settingsSegmentedOn(c, appearanceNames, &a.settings.Appearance, a.setAppearance)
		})
		a.settingsDivider(c)
		a.settingsRow(c, a.tr("Accent"), a.tr("Focus rings, selections and key hints."), func() {
			a.settingsDots(c)
		})
	})

	a.settingsThemeCard(c)

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Interface font"), a.tr("The panels’ typeface and size."), func() {
			a.settingsFont(c, &a.settings.InterfaceFont, &a.settings.InterfaceSize, false)
		})
		a.settingsDivider(c)
		a.settingsRow(c, a.tr("Terminal font"), a.tr("Used by every pane, on every machine."), func() {
			a.settingsFont(c, &a.settings.TerminalFont, &a.settings.TerminalSize, true)
		})
		a.settingsDivider(c)
		a.settingsRow(c, a.tr("Zoom"), a.tr("Scales the whole window, terminals included."), func() {
			a.stepper(c, fmt.Sprintf("%.0f%%", a.Zoom*100))
			a.settingsLink(c, a.tr("Reset"), func() { a.Zoom = 1; a.Repaint() })
		})
	})

	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		a.icon(c, "file", 12, t.Dim)
		ui.Text(c, a.SettingsPath()).Font(TerminalFont).FontSize(a.fontSize(10.5)).
			TextColor(t.Muted)
		ui.Text(c, a.tr("· saved as you go")).FontSize(a.fontSize(11)).TextColor(t.Dim)
	})
}

// settingsFont edits a family and size; terminal choices are monospaced.
func (a *App) settingsFont(c *ui.Context, name *string, size *int, mono bool) {
	t := a.T
	label := a.tr("Interface font")
	if mono {
		label = a.tr("Terminal font")
	}
	sel := ui.SelectBase(c.Key(label+"-family"), name)
	sel.Trigger.Row().Height(30).Width(150).PaddingX(9).Gap(8).Radius(8).
		Border(1, t.Border).Background(Alpha(t.Foreground, 0.045)).
		AlignItems(ui.Center).Label(a.tr("%s family", label)).Children(func() {
		ui.Text(c, *name).FontSize(a.fontSize(12.5)).TextColor(t.Foreground).Grow(1).Ellipsis("…")
		a.icon(c, "chevron-down", 12, t.Dim)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Width(240).MaxHeight(240).Padding(4).Radius(8).Border(1, t.Border).Background(t.Popover)
		ui.Scroll(c).FillWidth().Height(216).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, family := range a.fontNames(mono) {
					item := sel.Item(family).FillWidth().Padding(6, 8).Radius(4).Label(family)
					if item.Highlighted() {
						item.Background(t.Hover)
					}
					item.Children(func() {
						ui.Text(c, family).FontSize(a.fontSize(12.5)).TextColor(t.Foreground)
					})
				}
			})
		})
	})
	value := float64(*size)
	style := *c.Theme()
	style.Text, style.TextMuted = t.Foreground, t.Muted
	style.Surface, style.SurfaceHover, style.SurfacePressed = Alpha(t.Foreground, 0.045), t.Hover, t.Hover
	style.Border, style.Focus, style.Selection = t.Border, t.Accent, Alpha(t.Accent, 0.3)
	ui.Themed(c, &style).Background(ui.Transparent).Children(func() {
		input := ui.NumberInput(c.Key(label+"-size"), &value, 8, 40, 1).Label(a.tr("%s size", label))
		input.OnChange(func() {
			*size = int(value)
			if mono {
				a.applyTerminalFont()
			}
			a.Repaint()
		})
	})
	if sel.Trigger.Changed() {
		if mono {
			a.applyTerminalFont()
		}
		a.Repaint()
	}
}

// settingsThemeCard keeps a separate choice for light and dark windows.
func (a *App) settingsThemeCard(c *ui.Context) {
	t := a.T
	a.settingsCard(c, func() {
		ui.Column(c).Padding(14, 16, 16, 16).Gap(14).Children(func() {
			ui.Text(c, a.tr("Theme")).FontSize(a.fontSize(13.5)).FontWeight(500).TextColor(t.Foreground)
			ui.Text(c, a.tr("Choose colors for light and dark windows. Chrome and every terminal change together.")).FontSize(a.fontSize(11.5)).TextColor(t.Dim).Wrap()
			for _, dark := range []bool{false, true} {
				label, picker := a.tr("Light windows"), 1
				names := append([]string{"paper"}, keroThemes...)
				if dark {
					label, picker = a.tr("Dark windows"), 2
					names = append([]string{"graphite", "ink", "moss"}, keroThemes...)
				}
				ui.Text(c, label).FontSize(a.fontSize(12)).TextColor(t.Muted)
				ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
					for _, name := range names {
						a.settingsThemeTile(c, name, dark)
					}
				})
				name, _ := a.settings.themeFor(dark)
				a.settingsRow(c, a.tr("Ghostty theme"), a.tr("Built-in and installed Ghostty palettes."), func() {
					a.settingsButton(c, themeLabel(name)+"  ⌄", false, false, func() {
						if a.settings.ThemePicker == picker {
							a.settings.ThemePicker = 0
						} else {
							a.settings.ThemePicker = picker
							a.settings.ThemeSearch = ""
						}
					})
				})
				if a.settings.ThemePicker == picker {
					a.settingsGhosttyPicker(c, dark)
				}
			}
		})
	})
}

func themeLabel(name string) string {
	if _, ok := washTable[name]; ok {
		return strings.Title(name)
	}
	for _, builtin := range themes {
		if name == builtin {
			return strings.Title(name)
		}
	}
	return name
}

// settingsThemeTile previews the wash, terminal and ANSI colors together.
func (a *App) settingsThemeTile(c *ui.Context, name string, dark bool) {
	t, th := a.T, themeForAppearance(name, dark)
	selected, _ := a.settings.themeFor(dark)
	on := selected == name
	key := fmt.Sprintf("theme-%v-%s", dark, name)
	appearance := a.tr("light windows")
	if dark {
		appearance = a.tr("dark windows")
	}
	col := ui.ButtonBase(c.Key(key)).Column().Width(104).Gap(6).Label(themeLabel(name) + ", " + appearance)
	col.Children(func() {
		border := t.Border
		if on {
			border = t.Accent
		}
		prev := ui.Box(c).Size(104, 74).Radius(8).Background(th.Chrome).Border(1, border).Padding(7)
		if len(th.Backdrop.Stops) > 0 {
			prev.LinearGradient(th.Backdrop)
		}
		prev.Children(func() {
			ui.Column(c).Fill().Radius(5).Background(th.Background).Padding(7).Gap(5).Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Aa").FontSize(a.fontSize(16)).TextColor(th.TerminalForeground).Grow(1)
					ui.Box(c).Size(5, 12).Background(th.Cursor)
				})
				ui.Row(c).Gap(3).Children(func() {
					for i := 0; i < 8; i++ {
						ui.Box(c).Size(6, 6).Radius(3).Background(th.Ansi[i])
					}
				})
			})
			if on {
				ui.Box(c).Absolute().Top(4).Right(4).Size(14, 14).Radius(7).Background(t.Accent).Center().Children(func() { a.icon(c, "check", 9, t.AccentForeground) })
			}
		})
		color := t.Muted
		if on {
			color = t.Foreground
		}
		ui.Text(c, themeLabel(name)).FontSize(a.fontSize(11.5)).TextColor(color)
	})
	if col.Clicked() {
		a.chooseTheme(name, dark)
	}
}

func (a *App) settingsGhosttyPicker(c *ui.Context, dark bool) {
	t := a.T
	ui.Column(c).FillWidth().Gap(6).Padding(10).Radius(8).Border(1, t.Border).Background(t.Popover).Children(func() {
		input := ui.TextInputBase(c.Key("ghostty-search"), &a.settings.ThemeSearch).FillWidth().Height(28).FontSize(a.fontSize(12)).TextColor(t.Foreground).Placeholder(a.tr("Search Ghostty themes…")).AutoFocus()
		matches := ghosttyMatches(a.settings.ThemeSearch)
		if len(matches) == 0 {
			ui.Text(c, a.tr("No themes found")).FontSize(a.fontSize(12)).TextColor(t.Dim)
		}
		ui.Scroll(c).FillWidth().Height(min(float32(len(matches))*28, 168)).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, name := range matches {
					row := ui.ButtonBase(c.Key("ghostty-" + name)).Row().AlignItems(ui.Center).FillWidth().Height(28).PaddingX(8).Label(name)
					if row.Hovered() {
						row.Background(t.Hover)
					}
					row.Children(func() { ui.Text(c, name).FontSize(a.fontSize(12)).TextColor(t.Foreground) })
					if row.Clicked() {
						a.chooseTheme(name, dark)
						a.settings.ThemePicker = 0
					}
				}
			})
		})
		if len(matches) > 0 && input.Submitted() {
			a.chooseTheme(matches[0], dark)
			a.settings.ThemePicker = 0
		}
	})
}

func ghosttyMatches(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	var matches []string
	for _, name := range terminal.GhosttyThemes() {
		if strings.Contains(strings.ToLower(name), query) {
			matches = append(matches, name)
		}
	}
	return matches
}

// settingsTerminal is the Terminal section.
func (a *App) settingsTerminal(c *ui.Context) {
	t := a.T
	a.settingsHead(c, a.tr("Terminal"), a.tr("How panes behave, here and on every machine."))

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Clipboard"), a.tr("Let terminal programs write the clipboard."), func() {
			a.settingsToggle(c, &a.settings.Clipboard)
		})
	})

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Minimum contrast"), a.tr("1.0 leaves program colors alone. Raise it to steady light and dark text."), func() {
			a.settingsSlider(c, &a.settings.MinContrast, 1, 21)
		})
	})

	a.settingsCard(c, func() {
		a.settingsRow(c, "herdr-core", a.tr("The background program that runs your terminals and agents."), func() {
			pill := ui.Box(c).Height(28).PaddingX(8).Radius(7).Border(1, t.Border).
				Background(Alpha(t.Foreground, 0.045)).Center()
			pill.Children(func() {
				ui.Text(c, a.tr(corePath())).Font(TerminalFont).FontSize(a.fontSize(11)).TextColor(t.Muted).
					Width(230).Ellipsis("…")
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				color := t.Dim
				text := a.tr("Not running")
				if a.S != nil {
					color, text = t.Ansi[2], a.tr("Connected")
				}
				ui.Box(c).Size(6, 6).Radius(3).Background(color)
				ui.Text(c, text).FontSize(a.fontSize(11.5)).TextColor(t.Muted)
			})
		})
	})

	a.settingsTerminalPreview(c)
}

// corePath is the core binary the app would use, as a short path.
func corePath() string {
	path, err := exec.LookPath("hc")
	if err != nil {
		return "hc (not on PATH)"
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// settingsSlider is a horizontal value control.
func (a *App) settingsSlider(c *ui.Context, value *float64, lo, hi float64) {
	t := a.T
	frac := float32((*value - lo) / (hi - lo))
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	ui.Row(c).Width(224).Gap(12).AlignItems(ui.Center).Children(func() {
		track := ui.Box(c).Width(176).Height(20).Cursor(ui.CursorPointer)
		track.Children(func() {
			ui.Box(c).Absolute().Top(8).Left(0).Right(0).Height(4).Radius(2).
				Background(Alpha(t.Foreground, 0.13))
			if frac > 0 {
				ui.Box(c).Absolute().Top(8).Left(0).WidthPercent(frac * 100).Height(4).
					Radius(2).Background(t.Accent)
			}
			ui.Box(c).Absolute().Top(2).LeftPercent(frac*100).Translate(-8, 0).
				Size(16, 16).Radius(8).Background(t.Foreground).Border(1, t.Border)
		})
		ui.Text(c, fmt.Sprintf("%.1f", *value)).Font(TerminalFont).FontSize(a.fontSize(11.5)).
			TextColor(t.Muted).Width(36).TextAlign(ui.End)
		if x, _, ok := track.PointerPosition(); ok && track.Clicked() {
			f := float64(x / 176)
			if f < 0 {
				f = 0
			}
			if f > 1 {
				f = 1
			}
			*value = lo + f*(hi-lo)
			a.Repaint()
		}
	})
}

// settingsTerminalPreview is a small terminal in the current theme.
func (a *App) settingsTerminalPreview(c *ui.Context) {
	t := a.T
	box := ui.Column(c).FillWidth().Radius(12).Border(1, t.Border).
		Background(t.Background).Clip()
	box.Children(func() {
		ui.Row(c).Height(28).FillWidth().PaddingX(10).Gap(5).AlignItems(ui.Center).
			Children(func() {
				for i := 0; i < 3; i++ {
					ui.Box(c).Size(6, 6).Radius(3).Background(Alpha(t.Foreground, 0.15))
				}
				ui.Text(c, a.tr("Preview")).FontSize(a.fontSize(10.5)).TextColor(t.Dim).Padding(0, 0, 0, 6)
			})
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.Column(c).Padding(10, 14).Gap(1).Children(func() {
			a.termLine(c, "~", ` echo "agents keep running"`, t.Accent, t.TerminalForeground)
			a.termLine(c, "", "agents keep running", t.Dim, t.TerminalForeground)
			a.termLine(c, "~", " hc ls", t.Accent, t.TerminalForeground)
			a.termLine(c, "", "t3  claude    working    rove", t.Muted, t.Muted)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "~").Font(TerminalFont).FontSize(a.fontSize(10.5)).TextColor(t.Accent)
				ui.Box(c).Size(6, 11).Background(t.Accent)
			})
		})
	})
}

func (a *App) termLine(c *ui.Context, prompt, body string, promptColor, bodyColor ui.Color) {
	ui.Row(c).Gap(4).Children(func() {
		if prompt != "" {
			ui.Text(c, prompt).Font(TerminalFont).FontSize(a.fontSize(10.5)).TextColor(promptColor)
		}
		ui.Text(c, body).Font(TerminalFont).FontSize(a.fontSize(10.5)).TextColor(bodyColor).SingleLine()
	})
}

// settingsMachines is the Machines section.
func (a *App) settingsMachines(c *ui.Context) {
	a.settingsHead(c, a.tr("Machines"), a.tr("Rove reaches other computers over ssh. Their terminals and agents keep running there."))

	var machines []*herdr.Machine
	if a.S != nil {
		machines = a.S.Machines()
	}
	a.settingsCard(c, func() {
		a.settingsMachineRow(c, "monitor", a.tr("This computer"), a.tr("where this window runs"), "connected", "", nil)
		if len(machines) > 0 {
			for _, m := range machines {
				m := m
				a.settingsDivider(c)
				why := ""
				if m.Why != nil {
					why = *m.Why
				}
				a.settingsMachineRow(c, "server", m.Name, m.Target, m.State, why, func() {
					a.settingsMachineActions(c, m)
				})
			}
			a.settingsDivider(c)
			a.settingsAddMachineRow(c)
		}
	})

	if len(machines) == 0 {
		a.settingsEmptyMachines(c)
	}

	a.settingsNote(c, "server", a.tr("A machine runs its own herdr-core, so its agents keep working when this Mac sleeps."))
}

// settingsEmptyMachines is the first-run Machines page.
func (a *App) settingsEmptyMachines(c *ui.Context) {
	t := a.T
	a.settingsCard(c, func() {
		ui.Column(c).Padding(30, 20).Gap(9).AlignItems(ui.Center).Children(func() {
			icon := ui.Box(c).Size(46, 46).Radius(12).Background(Alpha(t.Foreground, 0.055)).
				Center()
			icon.Children(func() { a.icon(c, "server", 20, t.Muted) })
			ui.Text(c, a.tr("No other machines yet")).FontSize(a.fontSize(13.5)).FontWeight(500).
				TextColor(t.Foreground)
			ui.Text(c, a.tr("Add a server or a Mac from your ~/.ssh/config, or type how ssh reaches it. Its terminals and agents run there, and keep working when this Mac sleeps.")).
				FontSize(a.fontSize(11.5)).TextColor(t.Dim).Wrap().TextAlign(ui.Center).MaxWidth(430)
			a.settingsButton(c, a.tr("Add machine"), true, false, func() {
				a.OpenOverlay("machine-add")
			})
		})
	})
}

// settingsMachineRow draws one machine and its status.
func (a *App) settingsMachineRow(c *ui.Context, icon, name, target, state, why string, actions func()) {
	t := a.T
	row := ui.Row(c).FillWidth().MinHeight(66).Padding(11, 16).Gap(12).
		AlignItems(ui.Center)
	row.Children(func() {
		tile := ui.Box(c).Size(34, 34).Radius(9).Background(Alpha(t.Foreground, 0.055)).
			Center()
		tile.Children(func() { a.icon(c, icon, 16, t.Muted) })
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, name).FontSize(a.fontSize(13.5)).FontWeight(500).TextColor(t.Foreground)
			ui.Text(c, target).FontSize(a.fontSize(11.5)).TextColor(t.Dim).Ellipsis("…")
			if why != "" {
				ui.Text(c, why).FontSize(a.fontSize(11)).TextColor(t.Blocked).Ellipsis("…")
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(a.machineStateColor(state))
			ui.Text(c, a.tr(machineStateText(state))).FontSize(a.fontSize(11.5)).TextColor(t.Muted)
		})
		if actions != nil {
			actions()
		}
	})
}

func (a *App) machineStateColor(state string) ui.Color {
	switch state {
	case "connected":
		return a.T.Ansi[2]
	case "connecting":
		return a.T.Working
	case "needs_login", "unreachable":
		return a.T.Blocked
	}
	return a.T.Dim
}

func machineStateText(state string) string {
	switch state {
	case "connected":
		return "Connected"
	case "connecting":
		return "Connecting"
	case "needs_login":
		return "Needs login"
	case "unreachable":
		return "Offline"
	}
	return state
}

// settingsMachineActions is Connect or Remove for one machine.
func (a *App) settingsMachineActions(c *ui.Context, m *herdr.Machine) {
	if m.Connected() {
		a.settingsButton(c, a.tr("Remove"), false, true, func() {
			go func() {
				if err := a.S.RemoveMachine(m.Name); err != nil {
					a.toast(a.tr("Cannot remove the machine"), err.Error(), "error")
				}
				a.S.RefreshMachines()
				a.Repaint()
			}()
		})
		return
	}
	a.settingsButton(c, a.tr("Connect"), false, true, func() {
		go func() {
			if err := a.S.ConnectMachine(m.Name); err != nil {
				a.toast(a.tr("Cannot connect"), err.Error(), "error")
			}
			a.Repaint()
		}()
	})
}

// settingsAddMachineRow opens the add-machine sheet.
func (a *App) settingsAddMachineRow(c *ui.Context) {
	t := a.T
	row := ui.Row(c).FillWidth().Height(48).PaddingX(16).Gap(10).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Children(func() {
		plus := ui.Box(c).Size(24, 24).Radius(7).Border(1, t.Input).Center()
		plus.Children(func() { a.icon(c, "plus", 12, t.Muted) })
		ui.Text(c, a.tr("Add machine")).FontSize(a.fontSize(12.5)).TextColor(t.Foreground)
		ui.Spacer(c)
		ui.Text(c, a.tr("From ~/.ssh/config or an address")).FontSize(a.fontSize(11)).TextColor(t.Dim).
			Ellipsis("…")
	})
	if row.Clicked() {
		a.OpenOverlay("machine-add")
	}
}

// settingsAlerts is the Alerts section.
func (a *App) settingsAlerts(c *ui.Context) {
	t := a.T
	a.settingsHead(c, a.tr("Alerts"), a.tr("When an agent finishes or needs you, Rove can send a system notification and play a sound."))

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Done"), a.tr("A system notification, with optional sound, when an agent finishes while you are elsewhere."), func() {
			ui.Box(c).Label(a.tr("Done alerts")).Children(func() { a.settingsToggle(c, &a.settings.AlertDone) })
		})
		a.settingsDivider(c)
		a.settingsRow(c, a.tr("Needs you"), a.tr("When an agent asks a question or waits for approval."), func() {
			ui.Box(c).Label(a.tr("Needs you alerts")).Children(func() { a.settingsToggle(c, &a.settings.AlertNeeds) })
		})
	})

	soundOn := a.settings.AlertSound
	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("Play a sound"), a.tr("Use the built-in sounds, or choose a WAV file up to 10 seconds long."), func() {
			ui.Box(c).Label(a.tr("Play alert sounds")).Children(func() { a.settingsToggle(c, &a.settings.AlertSound) })
		})
		a.settingsDivider(c)
		a.settingsRowWhen(c, soundOn, a.tr("Done sound"), a.tr("Plays when an agent finishes."), func() {
			a.settingsSound(c, a.tr("Done sound"), &a.settings.DoneSound, soundOn)
		})
		a.settingsDivider(c)
		a.settingsRowWhen(c, soundOn, a.tr("Needs you sound"), a.tr("Plays when an agent asks something."), func() {
			a.settingsSound(c, a.tr("Needs you sound"), &a.settings.NeedsSound, soundOn)
		})
	})

	a.settingsCard(c, func() {
		a.settingsRow(c, a.tr("By agent"), a.tr("Turn a sound off for one agent without changing the rest."), func() {
			ui.Text(c, a.agentSoundSummary()).FontSize(a.fontSize(11.5)).TextColor(t.Muted)
			label := a.tr("Customize  ›")
			if a.alerts.customize {
				label = a.tr("Hide  ‹")
			}
			a.settingsLink(c, label, func() { a.alerts.customize = !a.alerts.customize; a.Repaint() })
		})
		if a.alerts.customize {
			a.settingsDivider(c)
			a.settingsAgentSounds(c)
		}
	})

	note := a.tr("Alerts appear only for agents in another space. Muting an agent turns off its sound, while keeping system notifications.")
	if !soundOn {
		note = a.tr("Sound is off. System notifications still appear when an agent in another space finishes or needs you.")
	}
	a.settingsNote(c, "bell", note)
}
