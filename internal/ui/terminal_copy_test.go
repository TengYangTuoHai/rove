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

// A program that asks for enhanced pastes (mode 5522) must still get the
// paste as text: herdr-core's screen refuses every clipboard read, so an
// offered event would be read back as an error instead of the paste.
func TestPasteTextSkipsPasteEvents(t *testing.T) {
	for _, tc := range []struct {
		name  string
		modes string
		want  string
	}{
		{"bracketed", "\x1b[?5522h\x1b[?2004h", "\x1b[200~omp\x1b[201~"},
		{"plain", "\x1b[?5522h", "omp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := newPeekConn()
			term, err := terminal.New(terminal.Options{Conn: conn})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			term.Feed([]byte(tc.modes))
			pasteTextInto(term, "omp")
			conn.flushInput(t, term)
			conn.mu.Lock()
			got := string(conn.written)
			conn.mu.Unlock()
			if got != tc.want {
				t.Fatalf("pasted %q, want %q", got, tc.want)
			}
		})
	}
}

// The app's Paste shortcut reads the clipboard itself and hands the program
// text, even when the program asked for enhanced pastes: the pane's core
// refuses every clipboard read, so an event would come back as EPERM.
func TestPasteTerminalTextSkipsPasteEvents(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	conn := newPeekConn()
	opts := app.currentTerminalOptions()
	opts.Conn, opts.Theme = conn, terminalTheme(app.T)
	term, err := terminal.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("\x1b[?5522h\x1b[?2004h"))

	tt := myui.NewTester(func(c *myui.Context) { app.services = c.Services() }, 400, 200)
	tt.Frame()
	tt.SetClipboard("omp")
	app.pasteTerminalText(term)
	conn.flushInput(t, term)

	conn.mu.Lock()
	got := string(conn.written)
	conn.mu.Unlock()
	if want := "\x1b[200~omp\x1b[201~"; got != want {
		t.Fatalf("pasted %q, want %q", got, want)
	}
}
