package ui

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestTerminalClipboardTextTrimsPaddingAndTail(t *testing.T) {
	got := terminalClipboardText("first   \r\nsecond\t\n\n   \n")
	if want := "first\nsecond"; got != want {
		t.Fatalf("terminal text = %q, want %q", got, want)
	}
}

func TestCopyTerminalTextReachesTheClipboard(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	opts := app.currentTerminalOptions()
	opts.Conn, opts.Theme = newPeekConn(), terminalTheme(app.T)
	term, err := terminal.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("first line   \r\nsecond line\r\n"))
	tt := myui.NewTester(func(c *myui.Context) {
		app.services = c.Services()
		app.copyTerminalText(term)
	}, 400, 200)
	tt.Frame()
	if got, want := tt.Clipboard(), "first line\nsecond line"; got != want {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
}
