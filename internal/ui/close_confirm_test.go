package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestCloseWarningsScope(t *testing.T) {
	block := func(id, addr string) *herdr.Node {
		return &herdr.Node{ID: id, Type: "block", Content: &herdr.NodeContent{Terminal: addr}}
	}
	spaces := []*herdr.Space{
		{ID: "local", Tabs: []*herdr.Tab{
			{ID: "busy", Root: &herdr.Node{Type: "row", Children: []*herdr.Node{block("p1", "t1"), block("p2", "t2")}}},
			{ID: "idle", Root: block("p3", "t3")},
		}},
		{ID: "remote", Tabs: []*herdr.Tab{{ID: "remote/tab", Root: block("p4", "remote/t1")}}},
	}
	terms := map[string]*herdr.Terminal{
		"t1":        {State: "live", Foreground: &herdr.Foreground{Name: "sleep"}},
		"t2":        {State: "live", Argv: []string{"claude"}},
		"t3":        {State: "live", Argv: []string{"zsh"}},
		"remote/t1": {State: "live", Argv: []string{"python"}},
	}
	cases := []struct {
		command, target string
		want            int
	}{
		{"pane.close", "p1", 1},
		{"pane.close", "p3", 0},
		{"pane.close", "p4", 1},
		{"pane.close", "missing", 0},
		{"tab.close", "busy", 2},
		{"tab.close", "idle", 0},
		{"space.close", "local", 2},
		{"space.close", "remote", 1},
	}
	for _, tc := range cases {
		t.Run(tc.command+"/"+tc.target, func(t *testing.T) {
			got := closeWarnings(tc.command, tc.target, spaces, func(addr string) *herdr.Terminal { return terms[addr] })
			if len(got) != tc.want {
				t.Fatalf("warnings = %v, want %d affected terminals", got, tc.want)
			}
		})
	}
	terms["t1"].State = "finished"
	if got := closeWarnings("pane.close", "p1", spaces, func(addr string) *herdr.Terminal { return terms[addr] }); len(got) != 0 {
		t.Fatalf("finished process prompted: %v", got)
	}
}

func newCloseConfirmTest(t *testing.T, theme string, width int) (*App, *myui.Tester, *int) {
	t.Helper()
	RegisterFonts()
	t.Setenv("ROVE_HOME", t.TempDir())
	a := NewApp(&herdr.Session{}, ThemeByName(theme))
	closed := new(int)
	a.closing = closeState{kind: "pane", names: []string{"claude"}, run: func() { *closed++ }}
	a.OpenOverlay("close-confirm")
	tt := myui.NewTester(a.View, width, 600)
	tt.SetFocused(true)
	tt.Frame()
	return a, tt, closed
}

func TestCloseConfirmInteractions(t *testing.T) {
	for _, action := range []string{"cancel", "escape", "enter", "confirm", "backdrop", "repeat shortcut"} {
		t.Run(action, func(t *testing.T) {
			a, tt, closed := newCloseConfirmTest(t, "graphite", 900)
			if !tt.HasText("Close pane?") || !tt.HasText("claude") {
				t.Fatalf("missing warning: %v", tt.Texts())
			}
			switch action {
			case "cancel":
				if err := tt.Click("Cancel"); err != nil {
					t.Fatal(err)
				}
			case "escape":
				tt.Key(0, myui.KeyEscape)
			case "enter":
				tt.Key(0, myui.KeyEnter)
			case "confirm":
				if err := tt.Click("Close pane"); err != nil {
					t.Fatal(err)
				}
			case "backdrop":
				tt.ClickAt(880, 580)
			case "repeat shortcut":
				a.Run("pane.close")
				a.Run("tab.next")
				a.Run("app.palette")
			}
			tt.Frame()
			want := 0
			if action == "confirm" {
				want = 1
			}
			if *closed != want {
				t.Fatalf("closed %d times, want %d", *closed, want)
			}
			if action == "backdrop" || action == "repeat shortcut" {
				if a.overlay != "close-confirm" {
					t.Fatal("dialog lost the pending close")
				}
			} else if a.overlay != "" || a.closing.run != nil {
				t.Fatal("dialog retained a pending close after dismissal")
			}
		})
	}
}

func TestCloseConfirmKeepsTerminalKeyboard(t *testing.T) {
	f := newPaneFocusTest(t)
	f.tt.Frame()
	f.app.closing = closeState{kind: "pane", names: []string{"claude"}, run: func() { t.Fatal("unexpected close") }}
	f.app.OpenOverlay("close-confirm")
	f.tt.Frame()
	f.tt.Type("pending")
	f.tt.Frame()
	f.conns[0].flushInput(t, f.app.terms["p1|t1"].term)
	if got := f.conns[0].typed(); got != "" {
		t.Fatalf("terminal received input while confirmation was open: %q", got)
	}
	if err := f.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.typeInto(t, 0, "resumed")
}

func TestCloseConfirmRender(t *testing.T) {
	for _, theme := range []string{"graphite", "paper"} {
		for _, width := range []int{900, 360} {
			t.Run(theme+"/"+strconv.Itoa(width), func(t *testing.T) {
				a, tt, _ := newCloseConfirmTest(t, theme, width)
				a.closing.kind = "space"
				a.closing.names = []string{"claude", "npm", "python"}
				tt.Frame()
				for _, label := range []string{"Close space?", "Cancel", "Close space"} {
					r, ok := tt.Find(label)
					if !ok || r.X < 0 || r.X+r.W > float32(width) || r.Y < 0 || r.Y+r.H > 600 {
						t.Fatalf("%q outside viewport: %+v", label, r)
					}
				}
				if dir := os.Getenv("CLOSE_CONFIRM_EVIDENCE"); dir != "" {
					name := theme + "-wide.png"
					if width == 360 {
						name = theme + "-narrow.png"
					}
					f, err := os.Create(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					if err := png.Encode(f, tt.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}
