package ui

import (
	"fmt"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// paneFocusTest draws real pane views with in-memory terminals, so typing
// proves which terminal has the keyboard without starting a shell.
type paneFocusTest struct {
	app   *App
	tt    *myui.Tester
	panes []herdr.PaneInfo
	conns []*peekConn
	field string
}

func newPaneFocusTest(t *testing.T) *paneFocusTest {
	t.Helper()
	RegisterFonts()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f := &paneFocusTest{app: NewApp(&herdr.Session{}, ThemeByName("graphite"))}
	f.addPane(t, true)
	f.tt = myui.NewTester(func(c *myui.Context) {
		f.app.W, f.app.H = c.Size()
		myui.Column(c).Fill().Children(func() {
			myui.TextInput(c, &f.field).Label("Panel input").Height(30)
			myui.Row(c).Grow(1).FillWidth().Gap(8).Children(func() {
				for _, p := range f.panes {
					myui.Box(c).Grow(1).FillHeight().Children(func() {
						f.app.paneCard(c, p, 10, 0)
					})
				}
			})
		})
		f.app.overlays(c)
	}, 1000, 600)
	f.tt.SetFocused(true)
	return f
}

func (f *paneFocusTest) addPane(t *testing.T, ready bool) {
	t.Helper()
	for i := range f.panes {
		f.panes[i].Focused = false
	}
	i := len(f.panes) + 1
	p := herdr.PaneInfo{Pane: fmt.Sprintf("p%d", i), Terminal: fmt.Sprintf("t%d", i), Title: "shell", Focused: true}
	f.panes = append(f.panes, p)
	f.conns = append(f.conns, nil)
	key := p.Pane + "|" + p.Terminal
	if ready {
		f.attach(t, i-1)
	} else {
		f.app.starting[key] = true
	}
}

func (f *paneFocusTest) attach(t *testing.T, i int) {
	t.Helper()
	conn := newPeekConn()
	term, err := terminal.New(terminal.Options{
		Conn:  conn,
		Font:  terminal.Font{Family: TerminalFont, Size: 14},
		Theme: terminalTheme(f.app.T),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	p := f.panes[i]
	key := p.Pane + "|" + p.Terminal
	f.app.terms[key] = &paneTerm{key: key, pane: p.Pane, addr: p.Terminal, term: term}
	delete(f.app.starting, key)
	f.conns[i] = conn
}

func (f *paneFocusTest) selectPane(i int) {
	for j := range f.panes {
		f.panes[j].Focused = i == j
	}
	f.tt.Frame()
}

func (f *paneFocusTest) typeInto(t *testing.T, i int, text string) {
	t.Helper()
	before := make([]string, len(f.conns))
	for j, conn := range f.conns {
		if conn != nil {
			before[j] = conn.typed()
		}
	}
	f.tt.Type(text)
	f.tt.Frame()
	for j, conn := range f.conns {
		if conn == nil {
			continue
		}
		p := f.panes[j]
		conn.flushInput(t, f.app.terms[p.Pane+"|"+p.Terminal].term)
		got := strings.TrimPrefix(conn.typed(), before[j])
		want := ""
		if i == j {
			want = text
		}
		if got != want {
			t.Errorf("pane %d received %q, want %q", j+1, got, want)
		}
	}
}

func TestPaneTakesKeyboardWhenCreated(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.typeInto(t, 0, "first")
	f.addPane(t, true)
	f.tt.Frame()
	f.typeInto(t, 1, "split")
	if out := os.Getenv("FOCUS_SHOT_OUT"); out != "" {
		file, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKeyboardFollowsPaneSelection(t *testing.T) {
	f := newPaneFocusTest(t)
	f.addPane(t, true)
	f.selectPane(0)
	f.typeInto(t, 0, "left")
	f.selectPane(1)
	f.typeInto(t, 1, "right")
	f.selectPane(0)
	f.typeInto(t, 0, "back")
}

func TestPaneTakesKeyboardAfterAttachment(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.addPane(t, false)
	f.tt.Frame()
	f.attach(t, 1)
	f.tt.Frame()
	f.typeInto(t, 1, "ready")
}

func TestUnselectedAttachmentLeavesKeyboardInSelectedPane(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.addPane(t, false)
	f.tt.Frame()
	f.selectPane(0)
	f.attach(t, 1)
	f.tt.Frame()
	f.typeInto(t, 0, "still selected")
}

func TestPaneTakesKeyboardAfterTerminalReplacement(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.panes[0].Terminal = "replacement"
	f.attach(t, 0)
	f.tt.Frame()
	f.typeInto(t, 0, "replacement")
}

func TestPaneLeavesKeyboardInPanelInput(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	if err := f.tt.Click("Panel input"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.typeInto(t, -1, "panel")
	if f.field != "panel" {
		t.Fatalf("panel input = %q, want panel", f.field)
	}
}

func TestPaneWaitsForOverlayAndTakesKeyboardOnClose(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.app.OpenOverlay("palette")
	f.tt.Frame()
	f.addPane(t, true)
	f.tt.Frame()
	f.typeInto(t, -1, "search")
	if f.app.palette.Query != "search" {
		t.Fatalf("palette query = %q, want search", f.app.palette.Query)
	}
	f.app.CloseOverlay()
	f.tt.Frame()
	f.typeInto(t, 1, "new pane")

	f.app.OpenOverlay("palette")
	f.tt.Frame()
	f.app.CloseOverlay()
	f.tt.Frame()
	f.typeInto(t, 1, "same pane")
}
