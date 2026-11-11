package ui

import (
	"errors"
	"io"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

type endingStream struct {
	*peekSizedStream
	err error
}

func (s *endingStream) EndError() error { return s.err }

func TestPeekConnectionPreservesStreamEndAfterClose(t *testing.T) {
	for _, want := range []error{io.EOF, errors.New("connection interrupted")} {
		stream := &endingStream{peekSizedStream: &peekSizedStream{peekConn: newPeekConn()}, err: want}
		conn := newPeekConnection(func(int, int, int, int) (peekStream, error) { return stream, nil })
		if err := conn.ResizePixels(80, 24, 8, 16); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		if got := conn.EndError(); got != want {
			t.Fatalf("EndError after close = %v, want %v", got, want)
		}
	}
}

// A real UI terminal attaches over the framed fixture: a lost stream keeps the
// pane and can be retried, whereas a confirmed process exit still closes it.
func TestTerminalEndKeepsInterruptedPaneAndCanRetry(t *testing.T) {
	RegisterFonts()
	for _, mode := range []string{"signaled", "disconnect", "exited"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("STREAM_END_TEST", mode)
			s := fullscreenSession(t)
			s.SelectTab("split")
			app := NewApp(s, ThemeByName("graphite"))
			t.Cleanup(func() {
				app.mu.Lock()
				var terms []*paneTerm
				for key, entry := range app.terms {
					delete(app.terms, key)
					terms = append(terms, entry)
				}
				app.mu.Unlock()
				for _, entry := range terms {
					_ = entry.term.Close()
				}
			})
			pane := herdr.PaneInfo{Pane: "left", Terminal: "tleft"}
			key := "left|tleft"
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Box(c).Fill().Children(func() { app.paneCard(c, pane, 10, 0) })
			}, 800, 500)
			deadline := time.Now().Add(5 * time.Second)
			for {
				tt.Frame()
				panes := s.Panes("split")
				app.mu.Lock()
				failed := app.failedTerms[key]
				app.mu.Unlock()
				if mode == "exited" && len(panes) == 1 {
					if failed {
						t.Fatal("process exit was reported as a disconnect")
					}
					return
				}
				if mode != "exited" && failed {
					if len(panes) != 2 || panes[0].Terminal != "tleft" {
						t.Fatalf("interrupted pane was removed or replaced: %+v", panes)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("terminal %s did not finish", mode)
				}
				time.Sleep(time.Millisecond)
			}
			tt.Frame()
			button, ok := tt.Find("Retry")
			if !ok {
				t.Fatalf("no retry button: %v", tt.Texts())
			}
			if mode == "disconnect" {
				return
			}
			tt.ClickAt(button.X+button.W/2, button.Y+button.H/2)
			deadline = time.Now().Add(5 * time.Second)
			for {
				tt.Frame()
				app.mu.Lock()
				entry, failed := app.terms[key], app.failedTerms[key]
				app.mu.Unlock()
				if entry != nil && !failed && len(entry.term.Text()) > 0 {
					return
				}
				if time.Now().After(deadline) {
					t.Fatal("retry did not reattach to the same terminal")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestLateTerminalFailuresDoNotReplaceCurrentAttempt(t *testing.T) {
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	key := "p1|t1"
	old := newPeekConnection(nil)
	current := newPeekConnection(nil)
	entry := &paneTerm{conn: current}
	app.terms[key] = entry
	app.terminalFailedFor(key, old, "failure", errors.New("old attach failed"))
	if app.terms[key] != entry || app.failedTerms[key] || len(app.notes) != 0 {
		t.Fatal("old connection failure replaced the current terminal")
	}

	delete(app.terms, key)
	app.starting[key] = true
	app.startTokens[key] = 2
	app.terminalStartFailed(key, 1, "failure", errors.New("old start failed"))
	if !app.starting[key] || app.failedTerms[key] || len(app.notes) != 0 {
		t.Fatal("old start failure replaced the current attempt")
	}
	app.terminalStartFailed(key, 2, "failure", errors.New("current start failed"))
	if app.starting[key] || !app.failedTerms[key] || len(app.notes) != 1 {
		t.Fatal("current start failure was not reported")
	}
}
