package ui

import (
	"strings"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestToggleSidebarCollapsesToNothingAndRestoresWidth closes the sidebar to
// no width at all, then opens it to the width it had.
func TestToggleSidebarCollapsesToNothingAndRestoresWidth(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.SidebarWidth = 300
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()
	if got := app.sidebarShown; got < 299 || got > 301 {
		t.Fatalf("open sidebar width = %v, want 300", got)
	}

	app.toggleSidebar()
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	if got := app.sidebarShown; got > 0.5 {
		t.Fatalf("closed sidebar width = %v, want 0", got)
	}

	app.toggleSidebar()
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	if got := app.sidebarShown; got < 299 || got > 301 {
		t.Fatalf("reopened sidebar width = %v, want 300", got)
	}
}

// TestSidebarEdgeDragKeepsItsLimits drags the sidebar's edge past both ends:
// the width stays between the limits, and is the width the sidebar opens
// with again.
func TestSidebarEdgeDragKeepsItsLimits(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()

	start := app.sidebarShown - 2
	tt.Press(start, 200)
	tt.Frame()
	for i := 1; i <= 10; i++ {
		tt.Move(start-float32(i*40), 200)
		tt.Frame()
	}
	tt.Release(start-400, 200)
	tt.Frame()
	if got := app.SidebarWidth; got < sidebarMinWidth-0.01 || got > sidebarMinWidth+0.01 {
		t.Fatalf("dragged left to %v, want %v", got, sidebarMinWidth)
	}
	if got := app.settings.SidebarWidth; got < sidebarMinWidth-0.01 || got > sidebarMinWidth+0.01 {
		t.Fatalf("saved width = %v, want %v", got, sidebarMinWidth)
	}

	edge := app.sidebarShown - 2
	tt.Press(edge, 200)
	tt.Frame()
	for i := 1; i <= 20; i++ {
		tt.Move(edge+float32(i*40), 200)
		tt.Frame()
	}
	tt.Release(edge+800, 200)
	tt.Frame()
	if got := app.SidebarWidth; got < sidebarMaxWidth-0.01 || got > sidebarMaxWidth+0.01 {
		t.Fatalf("dragged right to %v, want %v", got, sidebarMaxWidth)
	}
}

// TestSidebarStatePersistsBetweenLaunches keeps the sidebar's width and
// whether it is open in the app's own file.
func TestSidebarStatePersistsBetweenLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	first.SidebarWidth = 320
	first.toggleSidebar()
	first.saveSettings()

	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if second.SidebarOpen {
		t.Error("the sidebar came back open")
	}
	if second.SidebarWidth != 320 {
		t.Errorf("sidebar width = %v, want 320", second.SidebarWidth)
	}
}

// TestSidebarFoldsPersistBetweenLaunches keeps the folded agents list and
// the folded machines in the app's own file, and forgets a fold that is
// unfolded again.
func TestSidebarFoldsPersistBetweenLaunches(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	first.toggleAgents()
	first.toggleMachine("gpu")
	first.saveSettings()

	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if !second.agentsCollapsed {
		t.Error("the agents list came back unfolded")
	}
	if !second.foldedMachines["gpu"] {
		t.Errorf("the folded machine came back unfolded: %v", second.foldedMachines)
	}

	second.toggleAgents()
	second.toggleMachine("gpu")
	second.saveSettings()

	third := NewApp(nil, ThemeByName("graphite"))
	third.LoadSettings()
	if third.agentsCollapsed {
		t.Error("the unfolded agents list came back folded")
	}
	if third.foldedMachines["gpu"] {
		t.Errorf("the unfolded machine came back folded: %v", third.foldedMachines)
	}
}

// TestSidebarRowsCutLongTextOneLineHigh draws a space's name and its git
// chip in a row too narrow for them: both end in an ellipsis on one line
// instead of wrapping into a second, which overflows their 26 px row.
func TestSidebarRowsCutLongTextOneLineHigh(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	name := "a-space-name-longer-than-the-sidebar-shows"
	branch := "feature/a-branch-name-longer-than-the-sidebar-shows"
	cases := []struct {
		what  string
		width float32
		text  string
		draw  func(c *myui.Context)
	}{
		{"a space's name", 90, name, func(c *myui.Context) {
			sp := &herdr.Space{ID: "sp1", Name: name}
			app.spaceRow(c, sp, []*herdr.Space{sp}, nil)
		}},
		{"a git chip's branch", 100, branch, func(c *myui.Context) {
			app.gitChip(c, branch)
		}},
		{"a machine chip's name", 70, "a-machine-name-longer-than-the-chip-shows", func(c *myui.Context) {
			app.machineChip(c, "a-machine-name-longer-than-the-chip-shows")
		}},
	}
	for _, c := range cases {
		draw := c.draw
		tt := myui.NewTester(func(ctx *myui.Context) {
			myui.Column(ctx).Width(c.width).Children(func() { draw(ctx) })
		}, 400, 200)
		tt.Frame()
		r, ok := tt.Find(c.text)
		if !ok {
			t.Fatalf("%s: %q is not drawn", c.what, c.text)
		}
		if r.H > 20 {
			t.Errorf("%s is %v high at a %v wide row, so it wrapped: %v", c.what, r.H, c.width, r)
		}
	}
}

// TestAgentRowsLineUpWithSpaceRows keeps an agent's row the height of a
// space's, and its text starting where the space name does, so the two lists
// share one surface and one hover.
func TestAgentRowsLineUpWithSpaceRows(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	sp := &herdr.Space{ID: "sp1", Name: "a-space"}
	ag := &herdr.Agent{SpaceName: "a-space", TabName: "a-tab", Status: "idle"}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).PaddingX(8).Gap(2).Children(func() {
			app.spaceRow(c, sp, []*herdr.Space{sp}, nil)
			app.agentRow(c, ag)
		})
	}, 400, 200)
	tt.Frame()
	space, ok := tt.Find("a-space")
	if !ok {
		t.Fatal("the space row is not drawn")
	}
	agent, ok := tt.Find("a-space · a-tab")
	if !ok {
		t.Fatal("the agent row is not drawn")
	}
	if got := agent.Y - space.Y; got != 28 {
		t.Errorf("the agent row starts %v below the space row, want 28: 26 px rows with the 2 px gap", got)
	}
	if got := agent.X - space.X; got != 0 {
		t.Errorf("the agent's name starts %v past the space's, want them in one column", got)
	}
}

// TestAgentRowNamesItsRemoteMachine draws the agents list's rows: a row from
// another machine names it after the row's name, and a row on this computer
// names no machine.
func TestAgentRowNamesItsRemoteMachine(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	draw := func(ag *herdr.Agent) *myui.Tester {
		tt := myui.NewTester(func(c *myui.Context) {
			myui.Column(c).Width(268).PaddingX(8).Children(func() {
				app.agentRow(c, ag)
			})
		}, 400, 120)
		tt.Frame()
		return tt
	}

	remote := draw(&herdr.Agent{SpaceName: "build", Machine: "gpu", Status: "working"})
	texts := remote.Texts()
	if len(texts) != 2 || texts[0] != "build" || texts[1] != "gpu" {
		t.Fatalf("a remote agent's row must name its machine after its name: %q", texts)
	}
	name, _ := remote.Find("build")
	machine, _ := remote.Find("gpu")
	if machine.X < name.X {
		t.Errorf("the machine chip starts at %v, before the name at %v", machine.X, name.X)
	}

	local := draw(&herdr.Agent{SpaceName: "notes", Status: "working"})
	if texts := local.Texts(); len(texts) != 1 || texts[0] != "notes" {
		t.Fatalf("an agent on this computer names no machine: %q", texts)
	}
}

// strp is a pointer to a string, for the terminal records' nullable fields.
func strp(s string) *string { return &s }

// worker is a terminal as herdr-core lists one: a program started from
// another terminal carries that terminal as its parent.
func worker(address, parent, name string) *herdr.Terminal {
	term := &herdr.Terminal{Address: address, State: "live", Name: strp(name)}
	if parent != "" {
		term.Parent = strp(parent)
	}
	return term
}

// TestSubAgentsHangUnderTheirParent draws the tree the agents list adds under
// an agent: its live sub-agents, depth first, with the lines that lead to
// them. A sub-agent a pane already shows is a row of its own, so only its own
// sub-agents are added there, and a finished one is gone.
func TestSubAgentsHangUnderTheirParent(t *testing.T) {
	root := worker("t1", "", "router")
	first := worker("t2", "t1", "worker one")
	first.Status = &herdr.Status{State: "working"}
	shown := worker("t3", "t1", "given a pane")
	deep := worker("t4", "t2", "its own worker")
	inside := worker("t5", "t3", "inside the pane's worker")
	gone := worker("t6", "t1", "finished")
	gone.State = "finished"
	all := []*herdr.Terminal{root, first, shown, deep, inside, gone}

	rows := subAgents(all, map[string]bool{"t3": true}, "t1", nil, 0)
	want := []struct {
		address string
		lines   []string
	}{
		{"t2", []string{"├"}},
		{"t4", []string{"│", "└"}},
		// t3 has a pane: no row of its own here, but its worker hangs under
		// the last line still.
		{"t5", []string{"└"}},
	}
	if len(rows) != len(want) {
		t.Fatalf("sub-agents: got %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if got := rows[i].Term.Address; got != w.address {
			t.Fatalf("row %d: %s, want %s", i, got, w.address)
		}
		if got := strings.Join(rows[i].Lines, ""); got != strings.Join(w.lines, "") {
			t.Fatalf("row %d's lines: %q, want %q", i, got, strings.Join(w.lines, ""))
		}
	}
}

// TestSubAgentWordsReadAsTheListShowsThem keeps a sub-agent's word: an error
// is a block, and done is idle, because it reports to its parent.
func TestSubAgentWordsReadAsTheListShowsThem(t *testing.T) {
	cases := map[string]string{"error": "blocked", "done": "idle", "working": "working", "blocked": "blocked", "": ""}
	for word, want := range cases {
		if got := subWord(word); got != want {
			t.Errorf("subWord(%q) = %q, want %q", word, got, want)
		}
	}
	subs := []subRow{{Term: &herdr.Terminal{Status: &herdr.Status{State: "error"}}}}
	if got := sectionWord("working", subs); got != "blocked" {
		t.Errorf("a worker in error puts its parent under %q, want blocked", got)
	}
	if got := sectionWord("done", nil); got != "done" {
		t.Errorf("an agent with no workers keeps its word: %q", got)
	}
}

// TestSubAgentRowIsIndentedUnderItsParent draws a sub-agent's row: it says the
// worker's name, its program's logo and its status, shifted in from the
// parent's name by the tree line.
func TestSubAgentRowIsIndentedUnderItsParent(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	ag := &herdr.Agent{SpaceName: "a-space", Status: "done"}
	sub := subRow{Term: worker("t2", "t1", "w-core-agents-visible"), Lines: []string{"└"}}
	sub.Term.Status = &herdr.Status{App: "pi", State: "working"}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).PaddingX(8).Gap(2).Children(func() {
			app.agentRow(c, ag)
			app.subAgentRow(c, sub)
		})
	}, 400, 200)
	tt.Frame()

	parent, ok := tt.Find("a-space")
	if !ok {
		t.Fatal("the parent row is not drawn")
	}
	name, ok := tt.Find("w-core-agents-visible")
	if !ok {
		t.Fatal("the sub-agent's row is not drawn")
	}
	if name.Y-parent.Y != 28 {
		t.Errorf("the sub-agent's row starts %v below its parent's, want 28: 26 px rows with the 2 px gap", name.Y-parent.Y)
	}
	if name.X <= parent.X {
		t.Errorf("the sub-agent's name starts at %v, not indented past the parent's at %v", name.X, parent.X)
	}
	if _, ok := tt.Find("└"); !ok {
		t.Error("the tree line that leads to the sub-agent is not drawn")
	}
}

// TestPeekSelectsItsSubAgentRow keeps the row selected while its worker is
// shown, moves the selection with the peek, and clears it when the peek closes.
func TestPeekSelectsItsSubAgentRow(t *testing.T) {
	RegisterFonts()
	for _, theme := range []string{"graphite", "paper"} {
		t.Run(theme, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName(theme))
			parent := &herdr.Agent{SpaceName: "parent", Focused: true}
			rows := []subRow{
				{Term: worker("t2", "t1", "first worker"), Lines: []string{"├"}},
				{Term: worker("remote/t2", "t1", "remote worker"), Lines: []string{"└"}},
			}
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Column(c).Width(268).PaddingX(8).Gap(2).Background(app.T.Chrome).Children(func() {
					app.agentRow(c, parent)
					for _, sub := range rows {
						app.subAgentRow(c, sub)
					}
				})
			}, 400, 200)
			tt.Frame()
			pixel := func(y int) [4]uint32 {
				r, g, b, a := tt.Image().At(12, y).RGBA()
				return [4]uint32{r, g, b, a}
			}
			selected, plain := pixel(13), pixel(41)
			if selected == plain {
				t.Fatal("the focused parent has no distinct selection background")
			}
			check := func(wantFirst, wantRemote [4]uint32) {
				t.Helper()
				tt.Frame()
				if got := pixel(41); got != wantFirst {
					t.Errorf("first worker background = %v, want %v", got, wantFirst)
				}
				if got := pixel(69); got != wantRemote {
					t.Errorf("remote worker background = %v, want %v", got, wantRemote)
				}
			}
			check(plain, plain)
			tt.Click("first worker")
			check(selected, plain) // Selected also takes priority over hover.
			tt.Move(350, 150)
			check(selected, plain)
			tt.Click("remote worker")
			check(plain, selected)
			tt.Move(350, 150)
			check(plain, selected)
			app.OpenOverlay("help")
			check(plain, plain)
			app.OpenOverlay("peek")
			check(plain, selected)
			app.CloseOverlay()
			check(plain, plain)
		})
	}
}

// TestSubAgentRowPeeksAtItsWorker draws a worker's row: a click looks at the
// worker over the tab, as Herdr's agents list does, and gives it no pane
// before the peek's own keep asks for one.
func TestSubAgentRowPeeksAtItsWorker(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	ag := &herdr.Agent{SpaceName: "a-space", Status: "done"}
	sub := subRow{Term: worker("t2", "t1", "w-core-agents-visible"), Lines: []string{"\u2514"}}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).PaddingX(8).Gap(2).Children(func() {
			app.agentRow(c, ag)
			app.subAgentRow(c, sub)
		})
	}, 400, 200)
	tt.Frame()

	name, ok := tt.Find("w-core-agents-visible")
	if !ok {
		t.Fatal("the sub-agent's row is not drawn")
	}
	parent, ok := tt.Find("a-space")
	if !ok {
		t.Fatal("the parent row is not drawn")
	}
	tt.Move(parent.X+2, parent.Y+parent.H/2)
	tt.Frame()
	if got := tt.Cursor(); got != myui.CursorPointer {
		t.Errorf("the cursor is %v over an agent, want pointer", got)
	}

	at := [2]float32{name.X + 2, name.Y + name.H/2}
	tt.Move(at[0], at[1])
	tt.Frame()
	if got := tt.Cursor(); got != myui.CursorPointer {
		t.Errorf("the cursor is %v over a sub-agent, want pointer", got)
	}

	tt.ClickAt(at[0], at[1])
	tt.Frame()
	if app.overlay != "peek" {
		t.Fatalf("the click opened %q, want the peek", app.overlay)
	}
	if addr := app.peek.addr; addr != "t2" {
		t.Errorf("the peek looks at %q, want t2", addr)
	}
}
