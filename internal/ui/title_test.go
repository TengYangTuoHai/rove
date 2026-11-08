package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestAgentNamesFollowTitles redraws the same rows after the sessions change
// their titles, then clears the titles to restore the original names.
func TestAgentNamesFollowTitles(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	ag := &herdr.Agent{SpaceName: "project", TabName: "review", Status: "working", Title: "Review session"}
	sub := subRow{Term: worker("t2", "t1", "worker-name"), Lines: []string{"└"}}
	sub.Term.Title = "Worker session"
	sub.Term.Status = &herdr.Status{App: "pi", State: "working"}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).Padding(12).Gap(2).Background(app.T.Chrome).Children(func() {
			app.agentRow(c, ag)
			app.subAgentRow(c, sub)
		})
	}, 300, 100)
	check := func(file, parent, child string) {
		t.Helper()
		tt.Frame()
		for _, name := range []string{parent, child} {
			bounds, ok := tt.Find(name)
			if !ok {
				t.Fatalf("missing title %q in %q", name, tt.Texts())
			}
			if bounds.H > 20 {
				t.Errorf("title %q wrapped to %v high", name, bounds.H)
			}
		}
		if dir := os.Getenv("TITLE_SHOT_DIR"); dir != "" {
			f, err := os.Create(filepath.Join(dir, file))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	check("title-before.png", "Review session", "Worker session")
	ag.Title = "Review complete"
	sub.Term.Title = "Worker complete"
	check("title-after.png", "Review complete", "Worker complete")
	for _, old := range []string{"Review session", "Worker session"} {
		if tt.HasText(old) {
			t.Errorf("old title %q survived the redraw", old)
		}
	}
	sub.Term.Status.Title = "Agent-reported task"
	check("title-status.png", "Review complete", "Agent-reported task")
	ag.Title = "A session title much longer than the available sidebar width"
	sub.Term.Status.Title = "A worker title much longer than the available sidebar width"
	check("title-long.png", ag.Title, sub.Term.Status.Title)
	ag.Title = ""
	sub.Term.Title = ""
	sub.Term.Status.Title = ""
	check("title-fallback.png", "project · review", "worker-name")
}
