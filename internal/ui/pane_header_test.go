package ui

import (
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestPaneHeaderLayout(t *testing.T) {
	RegisterFonts()
	pane := herdr.PaneInfo{
		Pane: "test", Terminal: "terminal", Status: "working",
		Title: "π ⋮ ◩ HIST: Add CPU-sampled camera brightness histograms",
		Cwd:   "/home/dev/workspaces/robotics/long-project-directory/uav_fusion_detect",
	}
	for _, tc := range []struct {
		name                string
		width, fontSize     int
		status, statusLabel string
		control             string
	}{
		{"regular", 436, 13, "working", "Working", "Split right"},
		{"narrow", 170, 13, "blocked", "Needs you", "Pane"},
		{"minimum width", int(paneMinWidth), 13, "done", "Done", "Pane"},
		{"large text", 436, 26, "working", "Working", "Split right"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName("paper"))
			app.setLanguage("en")
			app.settings.InterfaceSize = tc.fontSize
			pane.Status = tc.status
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Column(c).Width(float32(tc.width)).Children(func() {
					app.paneHeader(c, pane)
					myui.Box(c).Height(10).FillWidth().Label("Terminal surface")
				})
			}, 500, 180)
			tt.Frame()
			bounds := func(label string) myui.Rect {
				t.Helper()
				rect, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q in %q", label, tt.Texts())
				}
				return rect
			}
			title, folder := bounds(pane.Title), bounds(pane.Cwd)
			status, terminal := bounds(tc.statusLabel), bounds("Terminal surface")
			if title.X != folder.X || title.Y+title.H > folder.Y || folder.Y+folder.H > terminal.Y ||
				title.X+title.W > status.X || status.X+status.W > float32(tc.width) {
				t.Fatalf("header overlaps or overflows: title %v, folder %v, status %v, terminal %v",
					title, folder, status, terminal)
			}
			if _, ok := tt.Find(tc.control); ok {
				t.Fatal("pane controls show before hover")
			}
			tt.Move(15, 12)
			tt.Frame()
			control := bounds(tc.control)
			if control.Y < title.Y+title.H || control.Y+control.H > terminal.Y ||
				control.X+control.W > float32(tc.width) || control.X < bounds(pane.Cwd).X+bounds(pane.Cwd).W {
				t.Fatalf("hover controls overlap or overflow: %v", control)
			}
			if bounds(pane.Title) != title || bounds(tc.statusLabel) != status {
				t.Fatal("hover moves the title or status")
			}
		})
	}
}

func TestPaneHeaderFocusAndActions(t *testing.T) {
	RegisterFonts()
	session := fullscreenSession(t)
	session.SelectTab("split")
	session.FocusPane("left")
	app := NewApp(session, ThemeByName("graphite"))
	app.setLanguage("en")
	width := float32(400)
	tt := myui.NewTester(func(c *myui.Context) {
		app.W, app.H = c.Size()
		for _, pane := range session.Panes("split") {
			if pane.Pane == "right" {
				pane.Title = "Right pane"
				myui.Column(c).Width(width).Children(func() { app.paneHeader(c, pane) })
			}
		}
		app.overlays(c)
	}, 500, 280)
	tt.Frame()
	tt.Move(80, 12)
	tt.ClickAt(80, 12)
	for _, pane := range session.Panes("split") {
		if pane.Pane == "right" && !pane.Focused {
			t.Fatal("clicking the title did not focus its pane")
		}
	}
	if err := tt.Click("Zoom pane"); err != nil {
		t.Fatal(err)
	}
	if session.Zoomed() != "right" {
		t.Fatal("zoom button did not zoom its own pane")
	}
	width = 170
	tt.Frame()
	tt.Move(80, 12)
	if err := tt.Click("Pane"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Copy terminal text", "Find in terminal", "Split right", "Split down", "Zoom pane", "Close pane"} {
		if !tt.HasText(label) {
			t.Errorf("compact pane menu is missing %q", label)
		}
	}
	if err := tt.Click("Zoom pane"); err != nil {
		t.Fatal(err)
	}
	if session.Zoomed() != "" || app.overlay != "" {
		t.Fatal("zoom action did not restore the split and close the menu")
	}
	tt.RightClickAt(80, 12)
	if app.overlay != "menu" || !tt.HasText("Close pane") {
		t.Fatal("right-click did not open the pane menu")
	}
}
