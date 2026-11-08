package ui

import (
	"encoding/json"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestClosePaneRender uses trees captured by herdr's ClosePane regression
// tests. It draws the real pane cards without connecting to the user's core.
func TestClosePaneRender(t *testing.T) {
	dir := os.Getenv("PANE_CLOSE_EVIDENCE")
	if dir == "" {
		t.Skip("set PANE_CLOSE_EVIDENCE and run herdr's TestClosePane tests first")
	}
	t.Setenv("ROVE_HOME", t.TempDir())
	RegisterFonts()
	files, err := filepath.Glob(filepath.Join(dir, "*-before.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no ClosePane fixtures: %v", err)
	}
	for _, before := range files {
		name := strings.TrimSuffix(filepath.Base(before), "-before.json")
		t.Run(name, func(t *testing.T) {
			read := func(path string) *herdr.Node {
				t.Helper()
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var n herdr.Node
				if err := json.Unmarshal(b, &n); err != nil {
					t.Fatal(err)
				}
				return &n
			}
			root := read(before)
			panes := map[string]herdr.PaneInfo{}
			for _, id := range []string{"a", "b", "c"} {
				panes[id] = herdr.PaneInfo{Pane: id, Tab: "tab", Title: "Pane " + id}
			}
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Column(c).Fill().Padding(10).Background(app.T.Background).Children(func() {
					app.renderNode(c, "tab", root, panes, true, "row")
				})
			}, 1000, 700)
			save := func(stage string) {
				t.Helper()
				f, err := os.Create(filepath.Join(dir, name+"-"+stage+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
			}
			save("before")
			prior := map[string]myui.Rect{}
			for id := range panes {
				r, ok := tt.Find("Pane " + id)
				if !ok {
					t.Fatalf("pane %s missing before close", id)
				}
				prior[id] = r
			}
			root = read(filepath.Join(dir, name+"-after.json"))
			tt.Frame()
			save("after")
			// In a nested split the survivor starts at the same outer
			// boundary, so closing below/right only fills that split.
			if strings.Contains(name, "nested") {
				if tt.HasText("Pane c") {
					t.Fatal("closed pane is still rendered")
				}
				for _, id := range []string{"a", "b"} {
					r, ok := tt.Find("Pane " + id)
					if !ok || math.Abs(float64(r.X-prior[id].X)) > 0.1 || math.Abs(float64(r.Y-prior[id].Y)) > 0.1 {
						t.Fatalf("pane %s moved from %+v to %+v", id, prior[id], r)
					}
				}
			} else {
				closed := name[len(name)-1:]
				if tt.HasText("Pane " + closed) {
					t.Fatal("closed pane is still rendered")
				}
				// Removing a divider returns its gap to the weighted pool;
				// the far sibling moves by at most that gap, not a pane share.
				id := "c"
				if closed == "c" {
					id = "a"
				}
				r, ok := tt.Find("Pane " + id)
				if !ok || math.Abs(float64(r.X-prior[id].X)) > float64(paneGap) || math.Abs(float64(r.Y-prior[id].Y)) > float64(paneGap) {
					t.Fatalf("unaffected pane %s moved from %+v to %+v", id, prior[id], r)
				}
			}
		})
	}
}
