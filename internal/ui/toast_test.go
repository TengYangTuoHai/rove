package ui

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func toastSnapshot(app *App) []Toast {
	app.mu.Lock()
	defer app.mu.Unlock()
	return append([]Toast(nil), app.notes...)
}

func TestToastCanBeDismissedFromItsCloseButton(t *testing.T) {
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.W, app.H = 800, 600
	app.toast("Cannot show the terminal", "t1 has finished", "error")
	tt := myui.NewTester(func(c *myui.Context) { app.overlays(c) }, 800, 600)
	tt.Frame()
	close, ok := tt.Find("Dismiss notification")
	if !ok {
		t.Fatal("toast has no labeled close button")
	}
	tt.ClickAt(close.X+close.W/2, close.Y+close.H/2)
	tt.Frame()
	if notes := toastSnapshot(app); len(notes) != 0 {
		t.Fatalf("toast remains after close click: %+v", notes)
	}
}

func TestToastAutomaticallyExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
		app.toast("Cannot show the terminal", "t1 has finished", "error")
		time.Sleep(4 * time.Second)
		synctest.Wait()
		if notes := toastSnapshot(app); len(notes) != 1 {
			t.Fatal("toast disappeared before its reading time elapsed")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if notes := toastSnapshot(app); len(notes) != 0 {
			t.Fatalf("toast did not automatically disappear: %+v", notes)
		}
	})
}

func TestReplacingToastGetsItsOwnReadingTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
		app.toast("Cannot show the terminal", "t1 has finished", "error")
		time.Sleep(4 * time.Second)
		app.toast("Cannot show the terminal", "t2 has finished", "error")
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if notes := toastSnapshot(app); len(notes) != 1 || notes[0].Body != "t2 has finished" {
			t.Fatalf("the old timer removed its replacement: %+v", notes)
		}
		time.Sleep(4 * time.Second)
		synctest.Wait()
		if notes := toastSnapshot(app); len(notes) != 0 {
			t.Fatalf("replacement did not expire: %+v", notes)
		}
	})
}

func TestFailedTerminalDoesNotRepeatToastOnRepaint(t *testing.T) {
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	pane := herdr.PaneInfo{Pane: "p1", Terminal: "t1"}
	key := "p1|t1"
	app.starting[key] = true
	app.terminalFailed(key, "Cannot show the terminal", errors.New("t1 has finished"))
	app.expireToast(toastSnapshot(app)[0].id)
	for range 3 {
		if got := app.terminalFor(pane); got != nil {
			t.Fatal("failed attachment returned a terminal view")
		}
	}
	if app.starting[key] {
		t.Fatal("repainting restarted the failed attachment")
	}
	if notes := toastSnapshot(app); len(notes) != 0 {
		t.Fatal("repainting recreated the dismissed warning")
	}
	// Leaving the pane releases the failure too, allowing a retry on return.
	app.collect()
	if app.failedTerms[key] {
		t.Fatal("failure was retained after leaving the pane")
	}
}
