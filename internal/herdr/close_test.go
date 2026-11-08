package herdr

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func closeLayoutSession(t *testing.T, root *Node, focused string) *Session {
	t.Helper()
	t.Setenv("ROVE_HOME", t.TempDir())
	return &Session{
		spaces:    []*Space{{ID: "sp", Tabs: []*Tab{{ID: "tab", Root: root}}}},
		active:    "sp",
		activeTab: map[string]string{"sp": "tab"},
		tabs:      map[string]*tabView{"tab": {focused: focused, zoomed: focused, before: []string{"a"}}},
		terminals: map[string]*Terminal{},
	}
}

func closeWeight(w float64) *Size { return &Size{Weight: &w} }

func closeBlock(id string, size *Size) *Node {
	return &Node{ID: id, Type: "block", Size: size}
}

func checkCloseWeight(t *testing.T, n *Node, want float64) {
	t.Helper()
	if n == nil || n.Size == nil || n.Size.Weight == nil || math.Abs(*n.Size.Weight-want) > 1e-9 {
		t.Fatalf("node %+v: want weight %v", n, want)
	}
}

// A collapsed split occupies the same room in its parent, even though the
// surviving pane used a different weight along the split's other axis.
func TestClosePaneKeepsOuterSize(t *testing.T) {
	for _, axis := range []string{"row", "column"} {
		for _, size := range []struct {
			name string
			size *Size
		}{{"weighted", closeWeight(0.3)}, {"default", nil}, {"fixed", &Size{PX: new(240.0)}}} {
			t.Run(axis+"/"+size.name, func(t *testing.T) {
				inner := "column"
				if axis == "column" {
					inner = "row"
				}
				root := &Node{ID: "tab", Type: axis, Children: []*Node{
					closeBlock("a", closeWeight(0.7)),
					{ID: "split", Type: inner, Size: size.size, Children: []*Node{
						closeBlock("b", closeWeight(0.8)), closeBlock("c", closeWeight(0.2)),
					}},
				}}
				s := closeLayoutSession(t, root, "c")
				writeCloseTree(t, axis+"-nested-"+size.name+"-before", root)
				s.ClosePane("c")
				got := s.Tree("tab")
				if got.find("c") != nil || got.find("split") != nil || got.find("b") == nil {
					t.Fatalf("split did not collapse: %+v", got)
				}
				want, _ := json.Marshal(size.size)
				actual, _ := json.Marshal(got.find("b").Size)
				if string(actual) != string(want) {
					t.Fatalf("survivor's outer size = %s, want %s", actual, want)
				}
				checkCloseWeight(t, got.find("a"), 0.7)
				if s.tabs["tab"].focused != "a" || s.tabs["tab"].zoomed != "" {
					t.Fatalf("focus/zoom did not recover: %+v", s.tabs["tab"])
				}
				back := fromDoc(captured(s.spaces, nil), nil)[0].Tabs[0].Root
				actual, _ = json.Marshal(back.find("b").Size)
				if string(actual) != string(want) {
					t.Fatalf("saved outer size = %s, want %s", actual, want)
				}
				writeCloseTree(t, axis+"-nested-"+size.name+"-after", got)
			})
		}
	}
}

// Only the immediately preceding sibling takes the freed share; closing the
// first child grows the following one. Other panes keep their weights.
func TestClosePaneGrowsAdjacentSibling(t *testing.T) {
	for _, axis := range []string{"row", "column"} {
		for _, tc := range []struct {
			pane string
			want map[string]float64
		}{
			{"a", map[string]float64{"b": 0.5, "c": 0.5}},
			{"b", map[string]float64{"a": 0.5, "c": 0.5}},
			{"c", map[string]float64{"a": 0.2, "b": 0.8}},
		} {
			t.Run(axis+"/"+tc.pane, func(t *testing.T) {
				root := &Node{ID: "tab", Type: axis, Children: []*Node{
					closeBlock("a", closeWeight(0.2)), closeBlock("b", closeWeight(0.3)), closeBlock("c", closeWeight(0.5)),
				}}
				s := closeLayoutSession(t, root, tc.pane)
				writeCloseTree(t, axis+"-flat-"+tc.pane+"-before", root)
				s.ClosePane(tc.pane)
				got := s.Tree("tab")
				if got.find(tc.pane) != nil || len(got.blocks()) != 2 {
					t.Fatalf("pane was not removed: %+v", got)
				}
				for id, weight := range tc.want {
					checkCloseWeight(t, got.find(id), weight)
				}
				writeCloseTree(t, axis+"-flat-"+tc.pane+"-after", got)
			})
		}
	}
}

func TestClosePaneDefaultWeights(t *testing.T) {
	root := &Node{ID: "tab", Type: "row", Children: []*Node{
		closeBlock("a", nil), closeBlock("b", nil), closeBlock("c", closeWeight(3)),
	}}
	s := closeLayoutSession(t, root, "b")
	s.ClosePane("b")
	checkCloseWeight(t, root.find("a"), 2)
	checkCloseWeight(t, root.find("c"), 3)
	s.ClosePane("missing")
	checkCloseWeight(t, root.find("a"), 2)
}

func TestClosePaneKeepsSurvivingSubtree(t *testing.T) {
	root := &Node{ID: "tab", Type: "row", Children: []*Node{
		closeBlock("a", closeWeight(0.6)),
		{ID: "outer", Type: "column", Size: closeWeight(0.4), Children: []*Node{
			closeBlock("b", closeWeight(0.25)),
			{ID: "inner", Type: "row", Size: closeWeight(0.75), Children: []*Node{
				closeBlock("c", closeWeight(0.2)), closeBlock("d", closeWeight(0.8)),
			}},
		}},
	}}
	s := closeLayoutSession(t, root, "d")
	s.ClosePane("b")
	checkCloseWeight(t, root.find("inner"), 0.4)
	checkCloseWeight(t, root.find("c"), 0.2)
	checkCloseWeight(t, root.find("d"), 0.8)
	checkCloseWeight(t, root.find("a"), 0.6)
	if s.tabs["tab"].focused != "d" || s.tabs["tab"].zoomed != "d" {
		t.Fatalf("closing an unfocused pane disturbed focus/zoom: %+v", s.tabs["tab"])
	}
	// A second collapse takes the same outer room; the next close gives
	// it to the root's remaining child, and the final close ends the tab.
	s.ClosePane("c")
	checkCloseWeight(t, root.find("d"), 0.4)
	s.ClosePane("d")
	if len(root.blocks()) != 1 || root.blocks()[0].ID != "a" {
		t.Fatalf("unexpected remaining panes: %+v", root.blocks())
	}
	s.ClosePane("a")
	if len(s.spaces) != 0 || len(s.tabs) != 0 {
		t.Fatal("last pane did not close its tab and empty space")
	}
}

func TestClosePaneFixedSiblings(t *testing.T) {
	root := &Node{ID: "tab", Type: "row", Children: []*Node{
		closeBlock("a", &Size{PX: new(120.0)}), closeBlock("b", &Size{PX: new(240.0)}), closeBlock("c", closeWeight(1)),
	}}
	s := closeLayoutSession(t, root, "b")
	s.ClosePane("b")
	if size := root.find("a").Size; size == nil || size.PX == nil || *size.PX != 360 {
		t.Fatalf("fixed neighbor did not take the removed width: %+v", size)
	}
	checkCloseWeight(t, root.find("c"), 1)
}

// Optional fixtures let the UI's actual pane renderer inspect the trees this
// command produced, without attaching to or editing the person's core.
func writeCloseTree(t *testing.T, name string, root *Node) {
	t.Helper()
	if dir := os.Getenv("PANE_CLOSE_EVIDENCE"); dir != "" {
		b, err := json.Marshal(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
