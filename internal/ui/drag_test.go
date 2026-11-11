package ui

import (
	"encoding/json"
	"math"
	"net"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

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

// A held divider stays held through zero-delta frames. Saving the final
// weights must not wait for the core, even when its doc.set reply is stalled.
func TestPaneDividerSavesOnlyAfterReleaseWithoutBlockingFrames(t *testing.T) {
	RegisterFonts()
	for _, tc := range []struct{ axis, echo string }{
		{"row", "before-ack"}, {"row", "after-ack"},
		{"column", "before-ack"}, {"column", "after-ack"},
	} {
		t.Run(tc.axis+"/"+tc.echo, func(t *testing.T) {
			gate, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			t.Setenv("PANE_DRAG_TEST_GATE", gate.Addr().String())
			t.Setenv("PANE_DRAG_TEST_AXIS", tc.axis)
			t.Setenv("PANE_DRAG_TEST_ECHO", tc.echo)
			s := fullscreenSession(t)
			changes := make(chan struct{}, 16)
			s.OnChange = func() { changes <- struct{}{} }
			s.SelectTab("split")
			app := NewApp(s, ThemeByName("graphite"))
			app.SidebarOpen, app.sidebarShown = false, 0
			// Keep this test about layout and persistence, without attachments.
			app.starting["left|tleft"], app.starting["right|tright"] = true, true
			tt := myui.NewTester(app.View, 1000, 600)
			step := func(name string, f func()) {
				t.Helper()
				done := make(chan struct{})
				go func() { f(); close(done) }()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					s.Core.Close()
					<-done
					t.Fatalf("%s blocked on a core document write", name)
				}
			}
			x, y := float32(500), float32(300)
			dx, dy := float32(40), float32(0)
			extent := float32(1000 - 16 - paneGap)
			if tc.axis == "column" {
				y, dx, dy = 307, 0, 40
				extent = 600 - 42 - 28 - 16 - paneGap
			}
			step("press", func() { tt.Press(x, y) })
			if !app.paneDrag {
				t.Fatal("stationary press was not kept as an active pane drag")
			}
			step("move", func() { tt.Move(x+dx, y+dy) })
			for range 3 {
				step("held frame", tt.Frame)
				if !app.paneDrag {
					t.Fatal("a zero-delta frame ended the pane drag")
				}
			}
			step("move again", func() { tt.Move(x+2*dx, y+2*dy) })
			want := 1 + 160/float64(extent)
			left := s.Tree("split").Children[0]
			if left.Size == nil || left.Size.Weight == nil || math.Abs(*left.Size.Weight-want) > 0.001 {
				t.Fatalf("divider lost pointer travel: size %+v, want weight %v", left.Size, want)
			}
			step("release", func() { tt.Release(x+2*dx, y+2*dy) })
			step("released frame", tt.Frame)
			if app.paneDrag {
				t.Fatal("pane drag stayed active after release")
			}
			takeSave := func(want float64) net.Conn {
				t.Helper()
				_ = gate.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
				conn, err := gate.Accept()
				if err != nil {
					t.Fatalf("release did not save the layout: %v", err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				var doc struct {
					Spaces []*herdr.Space `json:"spaces"`
				}
				if err := json.NewDecoder(conn).Decode(&doc); err != nil {
					t.Fatal(err)
				}
				got := doc.Spaces[0].Tabs[0].Root.Children[0]
				if got.Size == nil || got.Size.Weight == nil || math.Abs(*got.Size.Weight-want) > 0.001 {
					t.Fatalf("saved weights differ from the released layout: %+v", got.Size)
				}
				return conn
			}
			acknowledge := func(conn net.Conn) {
				t.Helper()
				// Discard redraws from pointer moves, then wait for the doc event
				// to be applied, including the echo sent before the write reply.
				for len(changes) > 0 {
					<-changes
				}
				if _, err := conn.Write([]byte("\n")); err != nil {
					t.Fatal(err)
				}
				select {
				case <-changes:
				case <-time.After(2 * time.Second):
					t.Fatal("core document echo was not processed")
				}
			}
			first := takeSave(want)
			// The first doc.set is still pending while the next drag begins.
			step("frame during save", tt.Frame)
			step("second press", func() { tt.Press(x+2*dx, y+2*dy) })
			step("second move", func() { tt.Move(x+3*dx, y+3*dy) })
			want = 1 + 240/float64(extent)
			acknowledge(first)
			left = s.Tree("split").Children[0]
			if left.Size == nil || left.Size.Weight == nil || math.Abs(*left.Size.Weight-want) > 0.001 {
				t.Fatalf("first save's echo overwrote the next drag: size %+v, want weight %v", left.Size, want)
			}
			step("second release", func() { tt.Release(x+3*dx, y+3*dy) })
			acknowledge(takeSave(want))
			step("idle after save", tt.Frame)
			step("click without movement", func() {
				tt.Press(x+3*dx, y+3*dy)
				tt.Release(x+3*dx, y+3*dy)
			})
			_ = gate.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond))
			if extra, err := gate.Accept(); err == nil {
				_ = extra.Close()
				t.Fatal("idle frames or a click saved the layout again")
			} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
				t.Fatalf("checking for duplicate saves: %v", err)
			}
		})
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
