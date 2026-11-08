package ui

import (
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestDropIndexAt(t *testing.T) {
	// Rows are 26 tall: the top half drops before the target, the bottom
	// half after it, and a target below the dragged row moves up a place.
	cases := []struct {
		name     string
		y        float32
		from, to int
		want     int
	}{
		{"top half, moving up", 5, 3, 1, 1},
		{"bottom half, moving up", 21, 3, 1, 2},
		{"top half, moving down", 5, 1, 3, 2},
		{"bottom half, moving down", 21, 1, 3, 3},
		{"just above the middle", 12, 1, 2, 1}, // already there: no move
		{"just below the middle", 14, 1, 2, 2},
	}
	for _, c := range cases {
		if got := dropIndexAt(c.y, 26, c.from, c.to); got != c.want {
			t.Errorf("%s: dropIndexAt(%v, %d, %d) = %d, want %d", c.name, c.y, c.from, c.to, got, c.want)
		}
	}
}

// TestSplitHandleDragsTheDivider drags the line between the spaces and
// agents lists down the window: the spaces list grows by what the pointer
// travelled, not by its horizontal travel, which stays at zero.
func TestSplitHandleDragsTheDivider(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()

	listHeight := app.H - 46 - sidebarFooterHeight - 1
	start := 46 + listHeight*app.splitRatio
	before := listHeight * app.splitRatio

	tt.Press(120, start)
	tt.Frame()
	for i := 1; i <= 2; i++ {
		tt.Move(120, start+float32(i*20))
	}

	want := (before + 40) / listHeight
	if got := app.splitRatio; got < want-0.001 || got > want+0.001 {
		t.Fatalf("splitRatio = %v, want %v", got, want)
	}
}

// TestSplitHandleStopsAtItsLimits keeps the agents list showing, and the
// spaces list readable, however far the pointer goes.
func TestSplitHandleStopsAtItsLimits(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()

	listHeight := app.H - 46 - sidebarFooterHeight - 1
	start := 46 + listHeight*app.splitRatio

	tt.Press(120, start)
	tt.Frame()
	for i := 1; i <= 30; i++ {
		tt.Move(120, start+float32(i*40))
	}
	minSpaces, _ := splitLimits(listHeight)
	if got := listHeight * app.splitRatio; got < minSpaces-0.001 || got > listHeight-0.001 {
		t.Fatalf("spaces height = %v, outside %v..%v", got, minSpaces, listHeight)
	}
}

// TestPaneWeightsMoveWithThePointer drags the divider between two panes: the
// DIPs it travels are what one pane gains and the other loses, in the weights
// the panes are sized by.
func TestPaneWeightsMoveWithThePointer(t *testing.T) {
	even := []paneSpan{{ID: "a", Weight: 0.5, Span: 396}, {ID: "b", Weight: 0.5, Span: 396}}
	cases := []struct {
		name  string
		cells []paneSpan
		at    int
		delta float32
		want  map[string]float64
	}{
		{"a pane grows, the other shrinks", even, 0, 40, map[string]float64{"a": 0.550505, "b": 0.449495}},
		// 792 DIPs a weight: 80 DIPs left for the second pane is a weight of 0.101.
		{"the second pane stops at its smallest", even, 0, 400, map[string]float64{"a": 0.89899, "b": 0.10101}},
		{"the first pane stops at its smallest", even, 0, -400, map[string]float64{"a": 0.10101, "b": 0.89899}},
		// 500 DIPs of weights: a third pane keeps its 0.5 and its room.
		{
			"a third pane keeps its share",
			[]paneSpan{{ID: "a", Weight: 0.5, Span: 396}, {ID: "b", Weight: 0.5, Span: 396}, {ID: "c", Weight: 0.5, Span: 396}},
			1, -40, map[string]float64{"b": 0.449495, "c": 0.550505},
		},
		// A pane fixed to px takes the weight that keeps its 200 DIPs: 200 × 0.5 / 396 = 0.2525, and 40 DIPs are 0.0505 of it.
		{
			"a px pane becomes a weight",
			[]paneSpan{{ID: "a", Weight: 0, Span: 200}, {ID: "b", Weight: 0.5, Span: 396}, {ID: "c", Weight: 0.5, Span: 396}},
			0, 40, map[string]float64{"a": 0.30303, "b": 0.449495},
		},
		// Nothing else shares: a weight is a DIP.
		{
			"two px panes move in px",
			[]paneSpan{{ID: "a", Weight: 0, Span: 300}, {ID: "b", Weight: 0, Span: 500}},
			0, 50, map[string]float64{"a": 350, "b": 450},
		},
	}
	for _, c := range cases {
		got, ok := paneWeights(c.cells, c.at, c.delta, 80)
		if !ok {
			t.Errorf("%s: no weights", c.name)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: moved %v, want %v", c.name, got, c.want)
			continue
		}
		for id, want := range c.want {
			if v := got[id]; v < want-0.001 || v > want+0.001 {
				t.Errorf("%s: %s = %v, want %v", c.name, id, v, want)
			}
		}
	}
	if _, ok := paneWeights(even, 1, 40, 80); ok {
		t.Fatal("a divider past the last child moved something")
	}
}
