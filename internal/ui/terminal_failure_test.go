package ui

import (
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestFinishedPaneOffersNewShell(t *testing.T) {
	RegisterFonts()
	t.Setenv("PANE_FINISHED_TEST", "1")
	s := fullscreenSession(t)
	app := NewApp(s, ThemeByName("graphite"))
	pane := herdr.PaneInfo{Pane: "broken", Terminal: "tbroken"}
	app.failedTerms["broken|tbroken"] = true
	tt := myui.NewTester(func(c *myui.Context) { app.terminalFailureView(c, pane) }, 600, 300)
	tt.Frame()
	if !tt.HasText("Start a new shell") {
		t.Fatalf("finished terminal has no recovery action: %q", tt.Texts())
	}
	if tt.HasText("Retry") {
		t.Fatal("finished terminal offered only a retry")
	}
	button, ok := tt.Find("Start a new shell")
	if !ok {
		t.Fatal("recovery button is not clickable")
	}
	tt.ClickAt(button.X+button.W/2, button.Y+button.H/2)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		panes := s.Panes("broken")
		if len(panes) == 1 && panes[0].Terminal == "tnew" {
			return
		}
		if notes := toastSnapshot(app); len(notes) > 0 {
			t.Fatalf("recovery failed: %s", notes[0].Body)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("recovery did not replace the finished terminal in its pane")
}

func TestFailedPaneOffersRetryWithoutReplacingTerminal(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	pane := herdr.PaneInfo{Pane: "p1", Terminal: "t1"}
	key := "p1|t1"
	app.failedTerms[key] = true
	tt := myui.NewTester(func(c *myui.Context) { app.terminalFailureView(c, pane) }, 600, 300)
	tt.Frame()
	if tt.HasText("Start a new shell") {
		t.Fatal("offered to replace a terminal whose state is unknown")
	}
	button, ok := tt.Find("Retry")
	if !ok {
		t.Fatalf("failed pane has no retry button: %q", tt.Texts())
	}
	tt.ClickAt(button.X+button.W/2, button.Y+button.H/2)
	app.mu.Lock()
	failed := app.failedTerms[key]
	app.mu.Unlock()
	if failed {
		t.Fatal("retry did not clear the failed attachment")
	}
}
