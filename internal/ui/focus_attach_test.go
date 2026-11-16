package ui

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestTerminalFocusAcrossViews keeps the keyboard with the terminal a space
// shows, not the one it replaced.
func TestTerminalFocusAcrossViews(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	mk := func() *terminal.Terminal {
		opts := app.currentTerminalOptions()
		opts.Conn, opts.Theme = newPeekConn(), terminalTheme(app.T)
		term, err := terminal.New(opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { term.Close() })
		term.Feed([]byte("hello"))
		return term
	}
	a, b := mk(), mk()
	show := "a"
	focused := map[string]bool{}
	tt := myui.NewTester(func(c *myui.Context) {
		if show == "a" {
			e := terminal.View(c, a).Fill()
			if !focused["a"] {
				focused["a"] = true
				e.Focus()
			}
		} else {
			e := terminal.View(c, b).Fill()
			if !focused["b"] {
				focused["b"] = true
				e.Focus()
			}
		}
	}, 600, 200)
	tt.Frame()
	if !tt.Focused("Terminal") {
		t.Fatal("the first terminal is not focused")
	}
	show = "b"
	tt.Frame()
	if !tt.Focused("Terminal") {
		t.Fatal("the second terminal is not focused after switching")
	}
}

// TestTerminalFocusWhenViewArrivesLate covers a remote terminal whose view
// appears after the space was already selected: the keyboard must follow it.
func TestTerminalFocusWhenViewArrivesLate(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	mk := func() (*terminal.Terminal, *peekConn) {
		conn := newPeekConn()
		opts := app.currentTerminalOptions()
		opts.Conn, opts.Theme = conn, terminalTheme(app.T)
		term, err := terminal.New(opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { term.Close() })
		term.Feed([]byte("hello"))
		return term, conn
	}
	termA, connA := mk()
	termB, connB := mk()
	pa := herdr.PaneInfo{Pane: "pa", Terminal: "ta", Focused: true}
	pb := herdr.PaneInfo{Pane: "pb", Terminal: "tb", Focused: true}
	show := "a"
	app.terms["pa|ta"] = &paneTerm{term: termA}
	app.failedTerms["pb|tb"] = true // the second terminal attaches later
	tt := myui.NewTester(func(c *myui.Context) {
		if show == "a" {
			app.terminalView(c, pa)
		} else {
			app.terminalView(c, pb)
		}
	}, 600, 200)
	tt.Frame()
	show = "b"
	tt.Frame()
	app.terms["pb|tb"] = &paneTerm{term: termB}
	tt.Frame()
	tt.Key(0, myui.KeyX)
	connA.flushInput(t, termA)
	connB.flushInput(t, termB)
	if connB.typed() != "x" {
		t.Fatalf("the late terminal did not take the keyboard: A=%q B=%q", connA.typed(), connB.typed())
	}
}
