package ui

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"slices"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// sidebarDragTest uses the production row/group renderer with an in-memory
// model. A returned move is applied on the next build, as Session.Places and
// Session.Machines reflect a confirmed drop in the real sidebar.
type sidebarDragTest struct {
	app      *App
	tt       *myui.Tester
	places   []*herdr.Space
	machines []*herdr.Machine
	scroll   myui.ScrollState
	moves    []struct {
		value sideDrag
		to    int
	}
}

func newSidebarDragTest(t *testing.T, places []*herdr.Space, machines []*herdr.Machine, height float32) *sidebarDragTest {
	t.Helper()
	RegisterFonts()
	f := &sidebarDragTest{app: NewApp(&herdr.Session{}, ThemeByName("graphite")), places: places, machines: machines}
	f.tt = myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).Background(f.app.T.Chrome).Children(func() {
			myui.Box(c).Height(40)
			viewport := myui.Scroll(c).Height(height).Padding(0, 8, 14, 8).TrackScroll(&f.scroll)
			var layout sideReorder
			viewport.Children(func() {
				for i, sp := range f.places {
					sp.Index = i + 1
				}
				layout = f.app.spaceItems(c, f.places, f.machines, nil)
			})
			if d, to := layout.update(viewport, &f.scroll); to > 0 {
				f.moves = append(f.moves, struct {
					value sideDrag
					to    int
				}{d, to})
				if d.Kind == "space" {
					from := slices.IndexFunc(f.places, func(sp *herdr.Space) bool { return sp.ID == d.ID })
					sp := f.places[from]
					f.places = slices.Delete(f.places, from, from+1)
					f.places = slices.Insert(f.places, to-1, sp)
				} else {
					from := slices.IndexFunc(f.machines, func(m *herdr.Machine) bool { return m.Name == d.ID })
					m := f.machines[from]
					f.machines = slices.Delete(f.machines, from, from+1)
					f.machines = slices.Insert(f.machines, to-1, m)
				}
				c.Invalidate()
			}
		})
	}, 360, int(height)+80)
	f.tt.Frame()
	return f
}

func dragSpaces(names ...string) []*herdr.Space {
	out := make([]*herdr.Space, len(names))
	for i, name := range names {
		out[i] = &herdr.Space{ID: name, Name: name}
	}
	return out
}

func (f *sidebarDragTest) rect(t *testing.T, text string) myui.Rect {
	t.Helper()
	r, ok := f.tt.Find(text)
	if !ok {
		t.Fatalf("%q is not drawn; texts: %q", text, f.tt.Texts())
	}
	return r
}

func (f *sidebarDragTest) press(t *testing.T, text string) {
	t.Helper()
	r := f.rect(t, text)
	f.tt.Press(r.X+4, r.Y+r.H/2)
}

func nearY(t *testing.T, got, want float32) {
	t.Helper()
	if math.Abs(float64(got-want)) > 0.5 {
		t.Fatalf("row Y = %.3f, want %.3f", got, want)
	}
}

func TestSidebarDragOpensStableGapAndCommitsWithoutJump(t *testing.T) {
	f := newSidebarDragTest(t, dragSpaces("alpha", "bravo", "charlie"), nil, 260)
	before := f.rect(t, "bravo").Y
	f.press(t, "alpha")
	// Below the last midpoint: both neighbors move up to open the end slot.
	f.tt.Move(100, 115)
	f.tt.Advance(50 * time.Millisecond)
	mid := f.rect(t, "bravo").Y
	if mid >= before || mid <= before-28 {
		t.Fatalf("neighbor did not animate between slots: before %v, now %v", before, mid)
	}
	if len(f.moves) != 0 || f.places[0].ID != "alpha" {
		t.Fatal("hover committed a move")
	}
	f.tt.Advance(600 * time.Millisecond)
	nearY(t, f.rect(t, "bravo").Y, before-28)
	for range 8 {
		f.tt.Advance(40 * time.Millisecond)
		nearY(t, f.rect(t, "bravo").Y, before-28)
	}
	if path := os.Getenv("SIDEBAR_DRAG_SHOT_OUT"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	f.tt.Release(100, 115)
	nearY(t, f.rect(t, "bravo").Y, before-28)
	if len(f.moves) != 1 || f.moves[0].to != 3 || f.places[2].ID != "alpha" {
		t.Fatalf("drop = %+v, places = %+v", f.moves, f.places)
	}
	f.tt.Advance(300 * time.Millisecond)
	nearY(t, f.rect(t, "bravo").Y, before-28)
	// A normal click still selects the row and never commits another move.
	if err := f.tt.Click("bravo"); err != nil {
		t.Fatal(err)
	}
	if len(f.moves) != 1 {
		t.Fatalf("click reordered rows: %+v", f.moves)
	}
}

func TestSidebarDragCommitsDuringAnimation(t *testing.T) {
	f := newSidebarDragTest(t, dragSpaces("alpha", "bravo", "charlie"), nil, 260)
	before := f.rect(t, "bravo").Y
	f.press(t, "alpha")
	f.tt.Move(100, 115)
	f.tt.Advance(40 * time.Millisecond)
	f.tt.Frame() // sample at the exact release time, between display refreshes
	shown := f.rect(t, "bravo").Y
	f.tt.Release(100, 115)
	nearY(t, f.rect(t, "bravo").Y, shown)
	f.tt.Advance(600 * time.Millisecond)
	nearY(t, f.rect(t, "bravo").Y, before-28)
	if len(f.moves) != 1 {
		t.Fatalf("drop committed %d moves, want one", len(f.moves))
	}
	// Dropping back at its own position does not write the model again.
	f.press(t, "bravo")
	f.tt.Move(100, 55)
	f.tt.Release(100, 55)
	if len(f.moves) != 1 {
		t.Fatalf("self-drop committed another move: %+v", f.moves)
	}
}

func TestSidebarDragReversesAndCancels(t *testing.T) {
	for _, cancel := range []string{"escape", "outside", "capture"} {
		t.Run(cancel, func(t *testing.T) {
			f := newSidebarDragTest(t, dragSpaces("alpha", "bravo", "charlie"), nil, 260)
			before := f.rect(t, "bravo").Y
			avatar := f.tt.Image().At(20, 106)
			f.press(t, "charlie")
			f.tt.Move(100, 50)
			f.tt.Advance(50 * time.Millisecond)
			if got := f.rect(t, "bravo").Y; got <= before {
				t.Fatalf("upward drag did not push neighbor down: %v <= %v", got, before)
			}
			// Reverse before the first spring finishes, then cancel.
			f.tt.Move(100, 150)
			f.tt.Advance(30 * time.Millisecond)
			switch cancel {
			case "escape":
				f.tt.Key(0, myui.KeyEscape)
			case "outside":
				f.tt.Move(320, 150)
				f.tt.Release(320, 150)
			case "capture":
				f.tt.CancelPress()
			}
			f.tt.Advance(600 * time.Millisecond)
			nearY(t, f.rect(t, "bravo").Y, before)
			if got := f.tt.Image().At(20, 106); got != avatar {
				t.Fatalf("cancel left source hidden: avatar %v, want %v", got, avatar)
			}
			if len(f.moves) != 0 {
				t.Fatalf("cancel committed: %+v", f.moves)
			}
			f.tt.Release(100, 150)
			f.press(t, "charlie")
			f.tt.Move(100, 50)
			f.tt.Release(100, 50)
			if len(f.moves) != 1 || f.moves[0].to != 1 {
				t.Fatalf("next upward drag = %+v, want first slot", f.moves)
			}
		})
	}
}

func TestSidebarDragRejectsOtherMachines(t *testing.T) {
	spaces := dragSpaces("alpha", "bravo", "remote")
	spaces[2].Machine = strp("gpu")
	machines := []*herdr.Machine{{Name: "gpu", State: "connected"}, {Name: "empty", State: "connected"}}
	f := newSidebarDragTest(t, spaces, machines, 330)
	before := f.rect(t, "alpha").Y
	f.press(t, "bravo")
	r := f.rect(t, "gpu")
	f.tt.Move(100, r.Y+r.H/2)
	f.tt.Advance(400 * time.Millisecond)
	nearY(t, f.rect(t, "alpha").Y, before)
	f.tt.Release(100, r.Y+r.H/2)
	if len(f.moves) != 0 {
		t.Fatalf("space crossed machines: %+v", f.moves)
	}
	// The last nonempty space list must not extend over a later empty group.
	f.press(t, "remote")
	r = f.rect(t, "empty")
	f.tt.Move(100, r.Y+r.H/2)
	f.tt.Release(100, r.Y+r.H/2)
	if len(f.moves) != 0 {
		t.Fatalf("space accepted another machine's empty section: %+v", f.moves)
	}
	// Its own header remains a valid first-slot target.
	f.press(t, "bravo")
	r = f.rect(t, "This computer")
	f.tt.Move(100, r.Y+r.H/2)
	f.tt.Release(100, r.Y+r.H/2)
	if len(f.moves) != 1 || f.moves[0].to != 1 {
		t.Fatalf("own header drop = %+v", f.moves)
	}
}

func TestSidebarDragKeepsMachineConstraintWhileHeadersLoad(t *testing.T) {
	spaces := dragSpaces("alpha", "bravo", "remote")
	spaces[2].Machine = strp("gpu")
	f := newSidebarDragTest(t, spaces, nil, 260)
	f.press(t, "alpha")
	f.tt.Move(100, 115)
	f.tt.Release(100, 115)
	if len(f.moves) != 0 {
		t.Fatalf("flat sidebar allowed a cross-machine move: %+v", f.moves)
	}
	f.press(t, "bravo")
	f.tt.Move(100, 50)
	f.tt.Release(100, 50)
	if len(f.moves) != 1 || f.moves[0].to != 1 {
		t.Fatalf("flat sidebar rejected a same-machine move: %+v", f.moves)
	}
}

func TestSidebarMachineDragMovesWholeExpandedGroup(t *testing.T) {
	spaces := dragSpaces("local", "remote one", "remote two", "other")
	spaces[1].Machine, spaces[2].Machine, spaces[3].Machine = strp("gpu"), strp("gpu"), strp("build")
	machines := []*herdr.Machine{{Name: "gpu", State: "connected"}, {Name: "build", State: "connected"}}
	f := newSidebarDragTest(t, spaces, machines, 330)
	local, before := f.rect(t, "local").Y, f.rect(t, "build").Y
	childOffset := f.rect(t, "other").Y - before
	f.press(t, "gpu")
	f.tt.Move(100, 268)
	f.tt.Advance(600 * time.Millisecond)
	nearY(t, f.rect(t, "build").Y, before-84) // header + two rows, including gaps
	nearY(t, f.rect(t, "local").Y, local)
	nearY(t, f.rect(t, "other").Y-f.rect(t, "build").Y, childOffset)
	f.tt.Release(100, 268)
	if len(f.moves) != 1 || f.moves[0].value.Kind != "machine" || f.moves[0].to != 2 {
		t.Fatalf("machine drop = %+v", f.moves)
	}
	nearY(t, f.rect(t, "build").Y, before-84)
}

func TestSidebarMachineDragUpPastFoldedGroup(t *testing.T) {
	spaces := dragSpaces("local", "remote one", "remote two", "other")
	spaces[1].Machine, spaces[2].Machine, spaces[3].Machine = strp("gpu"), strp("gpu"), strp("build")
	machines := []*herdr.Machine{{Name: "gpu", State: "connected"}, {Name: "build", State: "connected"}}
	f := newSidebarDragTest(t, spaces, machines, 330)
	f.app.foldedMachines["gpu"] = true
	f.tt.Frame()
	f.tt.Advance(600 * time.Millisecond)
	local, before := f.rect(t, "local").Y, f.rect(t, "gpu").Y
	f.press(t, "build")
	f.tt.Move(100, before)
	f.tt.Advance(600 * time.Millisecond)
	nearY(t, f.rect(t, "gpu").Y, before+56)
	nearY(t, f.rect(t, "local").Y, local)
	f.tt.Release(100, before)
	if len(f.moves) != 1 || f.moves[0].to != 1 || f.machines[0].Name != "build" {
		t.Fatalf("upward machine move = %+v", f.moves)
	}
	// A remote group cannot be put before the local section.
	f.press(t, "gpu")
	r := f.rect(t, "This computer")
	f.tt.Move(100, r.Y+r.H/2)
	f.tt.Release(100, r.Y+r.H/2)
	if len(f.moves) != 1 {
		t.Fatalf("machine moved ahead of the local group: %+v", f.moves)
	}
}

func TestSidebarDragTracksScrollAndReducedMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		t.Run(fmt.Sprint(reduced), func(t *testing.T) {
			spaces := dragSpaces("first")
			for i := 1; i < 18; i++ {
				spaces = append(spaces, &herdr.Space{ID: fmt.Sprint(i), Name: fmt.Sprintf("space %02d", i)})
			}
			f := newSidebarDragTest(t, spaces, nil, 180)
			f.tt.SetPreferences(myui.Preferences{ReduceMotion: reduced})
			f.press(t, "first")
			f.tt.Move(100, 215)
			for range 12 {
				f.tt.Advance(20 * time.Millisecond)
			}
			if f.scroll.Y < 50 {
				t.Fatalf("drag near edge did not scroll: %+v", f.scroll)
			}
			f.tt.Release(100, 215)
			if len(f.moves) != 1 || f.moves[0].to <= 6 {
				t.Fatalf("scrolling did not advance destination: %+v", f.moves)
			}
			if reduced {
				// All rows are already at their final layout coordinates.
				before := f.rect(t, "space 01").Y
				f.tt.Advance(100 * time.Millisecond)
				nearY(t, f.rect(t, "space 01").Y, before)
			}
		})
	}
}
