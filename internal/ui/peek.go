package ui

import (
	"fmt"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// peekState is the terminal the window looks at over the tab: a sub-agent with
// no pane of its own, seen where it runs, as Herdr's peek shows it. The
// attachment starts in the background, and ends when the overlay closes.
type peekState struct {
	addr    string
	term    *terminal.Terminal
	conn    *herdr.Attach
	focused bool   // the bar's terminal has taken the keyboard
	kept    bool   // the keep answered, and the look closes
	err     string // why the terminal cannot be looked at
}

// peekWords is how the bar words a terminal's status, as Herdr's peek does.
var peekWords = map[string]string{
	"working": "working",
	"blocked": "needs you",
	"done":    "finished",
	"idle":    "idle",
}

// peekTerminal looks at a terminal over the tab without giving it a pane, as
// `terminal.peek` does in Herdr: the agents list opens it for a sub-agent,
// whose bar offers to keep it as a pane, or to close the look.
func (a *App) peekTerminal(addr string) {
	a.dropPeek()
	a.mu.Lock()
	a.peek = peekState{addr: addr}
	a.mu.Unlock()
	a.OpenOverlay("peek")
	go a.startPeek(addr)
}

// startPeek attaches the peek's terminal, as a pane's view does, and hands it
// to the overlay to draw. The core takes a moment to answer, and the window
// keeps drawing while it does.
func (a *App) startPeek(addr string) {
	if term := a.S.Terminal(addr); term == nil || !term.Live() {
		a.peekFailed(addr, fmt.Errorf("no running terminal %q", addr))
		return
	}
	t := a.T
	var view *terminal.Terminal
	conn := newPeekConnection(func(cols, rows, cellW, cellH int) (peekStream, error) {
		attached, err := a.S.Attach(addr, cols, rows, t.TerminalForeground, t.Background, t.Cursor, cellW, cellH)
		if err != nil {
			a.peekFailed(addr, err)
			return nil, err
		}
		a.mu.Lock()
		if a.peek.addr == addr && a.peek.term == view {
			a.peek.conn = attached
		}
		a.mu.Unlock()
		return attached, nil
	})
	opts := a.currentTerminalOptions()
	opts.Conn, opts.Theme = conn, terminalTheme(t)
	opts.Padding, opts.PaddingColor = terminal.Padding{X: 14, Y: 12}, terminal.PaddingExtend
	view, err := terminal.New(opts)
	if err != nil {
		_ = conn.Close()
		a.peekFailed(addr, err)
		return
	}
	a.mu.Lock()
	if a.peek.addr != addr {
		// Another terminal was peeked at meanwhile, or the look closed: this
		// view is not wanted. Closing it ends the attachment with it.
		a.mu.Unlock()
		_ = view.Close()
		return
	}
	view.SetFont(a.terminalStyle)
	view.SetMinimumContrast(a.terminalPrefs.contrast)
	a.peek.term = view
	a.mu.Unlock()
	a.Repaint()
}

// peekFailed says why a terminal cannot be looked at, in the bar's body.
func (a *App) peekFailed(addr string, err error) {
	a.mu.Lock()
	if a.peek.addr == addr {
		a.peek.err = err.Error()
	}
	a.mu.Unlock()
	a.Repaint()
}

// dropPeek ends the look: the terminal keeps running, it is the attachment
// that closes.
func (a *App) dropPeek() {
	if a.find.key == "peek|"+a.peek.addr {
		a.closeFind()
	}
	a.mu.Lock()
	st := a.peek
	a.peek = peekState{}
	a.mu.Unlock()
	if st.term != nil {
		_ = st.term.Close()
	} else if st.conn != nil {
		_ = st.conn.Close()
	}
}

// peekHome tells whether a pane already shows the terminal: the bar's keep
// goes to that pane instead of giving the same program a second one.
func (a *App) peekHome(addr string) bool {
	for _, sp := range a.S.Places() {
		for _, tab := range sp.Tabs {
			for _, p := range a.S.Panes(tab.ID) {
				if p.Terminal == addr {
					return true
				}
			}
		}
	}
	return false
}

// peekOverlay is a terminal over the tab, as `terminal.peek` shows it in
// Herdr: the bar that says who it is and offers keep or close, and the
// terminal itself under it — drawn our way, both on the card's own surface.
func (a *App) peekOverlay(c *ui.Context) {
	a.mu.Lock()
	st := a.peek
	a.mu.Unlock()
	if st.kept {
		// The keep answered: the pane it made is in the tab behind the look.
		a.CloseOverlay()
		return
	}
	t := a.T
	width, height := a.W*0.8, a.H*0.8
	// The card is ours, and it is one surface: the terminal's own background,
	// on which the bar and the view under it both sit, as a pane's header and
	// its terminal do on the pane's. A band of another colour filled inside the
	// radius would show through the corners' edge.
	// A peek repaints with the terminal on every scroll frame. Its border
	// separates it from the tab without repainting a dialog's wide shadow.
	panel := ui.Column(c).Width(width).Height(height).Radius(12).
		Background(t.Background).Border(1, t.Border).Clip().
		Absolute().Top((a.H - height) / 2).Left((a.W - width) / 2)
	panel.Children(func() {
		a.peekBar(c, st)
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		// MyGo's GPU clips custom terminal painting to the innermost clip.
		// Match the card's inner bottom radius (12 minus its 1px border).
		ui.Box(c).Grow(1).FillWidth().Radius(0, 0, 11, 11).Clip().Children(func() {
			switch {
			case st.err != "":
				ui.Box(c).Fill().Center().Padding(24).Children(func() {
					ui.Text(c, st.err).FontSize(a.fontSize(12.5)).TextColor(t.Dim).Wrap().TextAlign(ui.Center)
				})
			case st.term == nil:
				ui.Box(c).Fill().Center().Children(func() {
					ui.Text(c, a.tr("Opening the terminal…")).FontSize(a.fontSize(12.5)).TextColor(t.Dim)
				})
			default:
				view := terminal.View(c, st.term).Fill()
				if !st.focused && a.find.key != "peek|"+st.addr {
					a.mu.Lock()
					a.peek.focused = true
					a.mu.Unlock()
					view.Focus()
				}
				a.findBar(c, "peek|"+st.addr, st.term)
			}
		})
	})
}

// peekBar is the row over a peeked terminal: its status and name, the agent it
// came from, and the buttons that keep it in a pane or close the look.
func (a *App) peekBar(c *ui.Context, st peekState) {
	t := a.T
	term := a.S.Terminal(st.addr)
	word := ""
	if term != nil {
		word = term.Word()
	}
	row := ui.Row(c).Height(34).FillWidth().PaddingX(12).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		a.peekStatus(c, word)
		if term != nil {
			a.programIcon(c, herdr.Program(term), 14, t.Foreground)
		}
		ui.Text(c, terminalName(term, st.addr)).FontSize(a.fontSize(12)).FontWeight(600).TextColor(t.Foreground).
			Grow(1).MinWidth(0).SingleLine()
		if w := peekWords[word]; w != "" {
			ui.Text(c, a.tr(w)).FontSize(a.fontSize(12)).TextColor(t.Status(word)).Shrink(0)
		}
		if parent := a.peekParent(term); parent != "" {
			ui.Text(c, a.tr("from %s", parent)).FontSize(a.fontSize(12)).TextColor(t.Dim).MaxWidth(220).SingleLine()
		}
		home := a.peekHome(st.addr)
		keep := a.tr("↗ keep")
		if home {
			keep = a.tr("↗ go to")
		}
		a.peekButton(c, keep, func() { a.keepPeek(st.addr, home) })
		a.peekButton(c, "✕", func() { a.CloseOverlay() })
	})
}

// peekStatus is the mark before a peeked terminal's name: the spinner while it
// works, else a dot in its status' color, an empty ring while it is idle.
func (a *App) peekStatus(c *ui.Context, word string) {
	t := a.T
	switch word {
	case "working":
		a.spinner(c, 12, t.Working)
	case "idle":
		ui.Box(c).Size(8, 8).Radius(4).Border(1.5, t.Idle)
	case "":
		ui.Text(c, "·").FontSize(a.fontSize(12)).TextColor(t.Dim)
	default:
		ui.Box(c).Size(8, 8).Radius(4).Background(t.Status(word))
	}
}

// peekButton is one of the bar's buttons, as a word that lights on hover.
func (a *App) peekButton(c *ui.Context, label string, onClick func()) {
	t := a.T
	b := ui.Row(c).Height(22).PaddingX(8).Radius(5).AlignItems(ui.Center)
	color := t.Muted
	if b.Hovered() {
		b.Background(t.Hover)
		color = t.Foreground
	}
	b.Cursor(ui.CursorPointer)
	b.Children(func() {
		ui.Text(c, label).FontSize(a.fontSize(12)).TextColor(color)
	})
	if b.Clicked() {
		onClick()
	}
}

// keepPeek gives the peeked terminal a pane in the shown tab, as the bar's
// keep does in Herdr, and closes the look: a terminal a pane already shows
// comes to the front instead.
func (a *App) keepPeek(addr string, home bool) {
	if home {
		a.S.FocusTerminal(addr)
		a.CloseOverlay()
		return
	}
	go func() {
		if _, _, err := a.keepTerminal(addr); err != nil {
			a.toast(a.tr("Cannot keep the sub-agent"), err.Error(), "error")
			a.Repaint()
			return
		}
		a.mu.Lock()
		if a.peek.addr == addr {
			a.peek.kept = true
		}
		a.mu.Unlock()
		a.Repaint()
	}()
}

// peekParent is the terminal that started a peeked one, as the bar names it.
func (a *App) peekParent(term *herdr.Terminal) string {
	if term == nil || term.Parent == nil {
		return ""
	}
	addr := *term.Parent
	return terminalName(a.S.Terminal(addr), addr)
}

// terminalName is what the list and the peek bar call a terminal: its session
// title, else its name, template, program, or address.
func terminalName(term *herdr.Terminal, addr string) string {
	if term == nil {
		return addr
	}
	if title := term.AgentTitle(); title != "" {
		return title
	}
	if term.Name != nil && *term.Name != "" {
		return *term.Name
	}
	if term.Template != nil && *term.Template != "" {
		return *term.Template
	}
	if name := herdr.Program(term).Name; name != "" {
		return name
	}
	return addr
}
