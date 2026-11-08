package ui

import (
	"encoding/json"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/assets"
	"rove/internal/herdr"
)

func feedFindOutput(term *terminal.Terminal) {
	var out strings.Builder
	for i := range 200 {
		if i%50 == 7 {
			out.WriteString("error: build failed\r\n")
		} else {
			fmt.Fprintf(&out, "ok %d\r\n", i)
		}
	}
	term.Feed([]byte(out.String()))
}

func waitForFind(t *testing.T, tt *myui.Tester, term *terminal.Terminal, total int) {
	t.Helper()
	for range 50 {
		tt.Frame()
		st := term.SearchStatus()
		if !st.Searching && st.Total == total {
			tt.Frame()
			return
		}
	}
	t.Fatalf("search = %+v, want %d matches; texts %q", term.SearchStatus(), total, tt.Texts())
}

func saveFindShot(t *testing.T, tt *myui.Tester, suffix string) {
	t.Helper()
	if path := os.Getenv("FIND_SHOT_OUT"); path != "" {
		f, err := os.Create(path + suffix + ".png")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTerminalFindScrollback(t *testing.T) {
	f := newPaneFocusTest(t)
	term := f.app.terms["p1|t1"].term
	f.tt.Frame()
	feedFindOutput(term)
	f.app.openFindFor("p1|t1", term)
	f.tt.Frame()
	f.tt.Type("ERROR")
	waitForFind(t, f.tt, term, 4)
	if !f.tt.HasText("1 of 4") || f.conns[0].typed() != "" {
		t.Fatalf("find did not take keyboard: texts %q, typed %q", f.tt.Texts(), f.conns[0].typed())
	}
	saveFindShot(t, f.tt, "-dark")
	for _, mods := range []myui.Modifiers{0, 0, myui.Shift} {
		f.tt.Key(mods, myui.KeyEnter)
		f.tt.Frame()
	}
	if !f.tt.HasText("2 of 4") {
		t.Fatalf("Enter/Shift+Enter: %q", f.tt.Texts())
	}
	if err := f.tt.Click("Next match (Enter)"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if !f.tt.HasText("3 of 4") {
		t.Fatalf("next button: %q", f.tt.Texts())
	}
	if err := f.tt.Click("Previous match (Shift+Enter)"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if !f.tt.HasText("2 of 4") {
		t.Fatalf("previous button: %q", f.tt.Texts())
	}
	// Escape works even when a button owns the keyboard, and cannot reach the PTY.
	f.tt.Key(0, myui.KeyEscape)
	f.tt.Frame()
	f.tt.Frame()
	if f.app.find.key != "" || term.SearchStatus().Text != "" || f.conns[0].typed() != "" {
		t.Fatalf("Escape left search/input behind: %+v, typed %q", term.SearchStatus(), f.conns[0].typed())
	}
	f.typeInto(t, 0, "shell")
	f.app.openFindFor("p1|t1", term)
	f.tt.Frame()
	f.tt.Type("missing") // Reopening selects the old query for replacement.
	waitForFind(t, f.tt, term, 0)
	if f.app.find.query != "missing" || !f.tt.HasText("No matches") {
		t.Fatalf("reopened query = %q, texts %q", f.app.find.query, f.tt.Texts())
	}
	f.tt.Key(myui.Cmd, myui.KeyA)
	f.tt.Key(0, myui.KeyBackspace)
	f.tt.Frame()
	if term.SearchStatus().Text != "" || f.tt.HasText("No matches") {
		t.Fatalf("clearing query kept search: %+v, texts %q", term.SearchStatus(), f.tt.Texts())
	}
}

func TestTerminalFindMovesBetweenViews(t *testing.T) {
	f := newPaneFocusTest(t)
	f.addPane(t, true)
	first := f.app.terms["p1|t1"].term
	second := f.app.terms["p2|t2"].term
	feedFindOutput(first)
	feedFindOutput(second)
	f.selectPane(0)
	f.app.openFindFor("p1|t1", first)
	f.tt.Frame()
	f.tt.Type("error")
	waitForFind(t, f.tt, first, 4)
	f.selectPane(1)
	f.app.openFindFor("p2|t2", second)
	waitForFind(t, f.tt, second, 4)
	if first.SearchStatus().Text != "" || second.SearchStatus().Text != "error" {
		t.Fatalf("moving search: first %+v, second %+v", first.SearchStatus(), second.SearchStatus())
	}
	f.app.collect() // This empty session no longer has either pane.
	if f.app.find.key != "" || second.SearchStatus().Text != "" {
		t.Fatal("removing the searched view kept its search")
	}
}

func TestTerminalFindFocusAcrossPanesAndOverlay(t *testing.T) {
	f := newPaneFocusTest(t)
	f.addPane(t, true)
	f.selectPane(0)
	term := f.app.terms["p1|t1"].term
	f.app.openFindFor("p1|t1", term)
	f.tt.Frame()
	f.tt.Type("error")
	f.tt.Frame()
	f.selectPane(1)
	f.typeInto(t, 1, "second")
	f.selectPane(0)
	f.typeInto(t, 0, "first")
	f.app.OpenOverlay("palette")
	f.tt.Frame()
	f.app.CloseOverlay()
	f.tt.Frame()
	f.tt.Type("new query")
	f.tt.Frame()
	if f.app.find.query != "new query" || f.conns[0].typed() != "first" {
		t.Fatalf("overlay did not return to search: query %q, terminal %q", f.app.find.query, f.conns[0].typed())
	}
}

func TestTerminalFindFollowsNewOutput(t *testing.T) {
	f := newPaneFocusTest(t)
	term := f.app.terms["p1|t1"].term
	term.Feed([]byte("构建失败\r\n"))
	f.app.openFindFor("p1|t1", term)
	f.tt.Frame()
	f.tt.Type("构建失败")
	waitForFind(t, f.tt, term, 1)
	term.Feed([]byte("构建失败\r\n"))
	waitForFind(t, f.tt, term, 2)
	selected := term.SearchStatus().Selected
	f.tt.Key(0, myui.KeyEnter)
	f.tt.Frame()
	if term.SearchStatus().Selected == selected {
		t.Fatal("next match did not move after new output")
	}
	f.tt.Key(0, myui.KeyEnter)
	f.tt.Frame()
	if term.SearchStatus().Selected != selected {
		t.Fatalf("search did not wrap: %q", f.tt.Texts())
	}
}

func TestTerminalFindWaitsForAttachment(t *testing.T) {
	f := newPaneFocusTest(t)
	f.addPane(t, false)
	f.app.openFindFor("p2|t2", nil)
	f.tt.Frame()
	f.attach(t, 1)
	term := f.app.terms["p2|t2"].term
	feedFindOutput(term)
	f.tt.Frame()
	f.tt.Type("error")
	waitForFind(t, f.tt, term, 4)
	if f.conns[1].typed() != "" {
		t.Fatal("the attachment took the search field's keyboard")
	}
}

func findShortcut(t *testing.T, tt *myui.Tester, action string) {
	t.Helper()
	mods, key, ok := parseBinding(KeyFor(action))
	if !ok {
		t.Fatalf("no shortcut for %s", action)
	}
	tt.Key(mods, key)
}

func TestPeekTerminalFindShortcuts(t *testing.T) {
	f := newPaneFocusTest(t)
	term := f.app.terms["p1|t1"].term
	feedFindOutput(term)
	delete(f.app.terms, "p1|t1") // A peek has its own attachment, outside the pane map.
	f.app.overlay = "peek"
	f.app.peek = peekState{addr: "worker", term: term}
	tt := myui.NewTester(f.app.View, 1000, 600)
	tt.SetFocused(true)
	tt.Frame()
	findShortcut(t, tt, "terminal.find")
	tt.Frame()
	tt.Type("error")
	waitForFind(t, tt, term, 4)
	findShortcut(t, tt, "terminal.find_next")
	tt.Frame()
	if !tt.HasText("2 of 4") {
		t.Fatalf("next shortcut: %q", tt.Texts())
	}
	findShortcut(t, tt, "terminal.find_previous")
	tt.Frame()
	if !tt.HasText("1 of 4") {
		t.Fatalf("previous shortcut: %q", tt.Texts())
	}
	tt.Key(0, myui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if f.app.find.key != "" || f.app.overlay != "peek" {
		t.Fatal("Escape should close search and keep the peek open")
	}
	tt.Type("shell")
	tt.Frame()
	f.conns[0].flushInput(t, term)
	if f.conns[0].typed() != "shell" {
		t.Fatalf("peek did not regain keyboard: %q", f.conns[0].typed())
	}
	findShortcut(t, tt, "terminal.find_next")
	waitForFind(t, tt, term, 4)
	if f.app.find.query != "error" {
		t.Fatal("Find Next did not reopen the last query")
	}
	f.app.CloseOverlay()
	if f.app.find.key != "" || term.SearchStatus().Text != "" {
		t.Fatal("closing peek kept search")
	}
}

func TestTerminalFindNarrowLight(t *testing.T) {
	f := newPaneFocusTest(t)
	f.app.T = ThemeByName("paper")
	term := f.app.terms["p1|t1"].term
	term.SetTheme(terminalTheme(f.app.T), nil)
	feedFindOutput(term)
	f.app.openFindFor("p1|t1", term)
	f.app.find.query = "error"
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Box(c).Fill().Children(func() { f.app.paneCard(c, f.panes[0], 10, 0) })
	}, 280, 400)
	waitForFind(t, tt, term, 4)
	bar, ok := tt.Find("Find in terminal")
	if !ok || bar.X < 0 || bar.X+bar.W > 280 {
		t.Fatalf("find bar outside pane: %+v", bar)
	}
	for _, label := range []string{"Terminal search", "Next match (Enter)", "Previous match (Shift+Enter)", "Close terminal search (Escape)"} {
		r, ok := tt.Find(label)
		if !ok || r.W <= 0 || r.X < bar.X || r.X+r.W > bar.X+bar.W+1 {
			t.Fatalf("%s outside find bar: %+v, bar %+v", label, r, bar)
		}
	}
	saveFindShot(t, tt, "-light-narrow")
}

func TestTerminalFindDefaultBindings(t *testing.T) {
	for file, want := range map[string]map[string]string{
		"keys.macos.json": {"cmd-f": "terminal.find", "cmd-g": "terminal.find_next", "cmd-shift-g": "terminal.find_previous"},
		"keys.json":       {"ctrl-shift-f": "terminal.find", "f3": "terminal.find_next", "shift-f3": "terminal.find_previous"},
	} {
		data, err := fs.ReadFile(assets.Files, file)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]map[string]string
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		for key, action := range want {
			if doc["global"][key] != action {
				t.Errorf("%s %s = %q, want %s", file, key, doc["global"][key], action)
			}
		}
	}
}

func TestTerminalFindKeepsSettingsSearch(t *testing.T) {
	a := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	a.OpenOverlay("settings")
	a.Run("terminal.find")
	if !a.settings.SearchOpen || a.find.key != "" {
		t.Fatal("Find did not stay in settings")
	}
}
