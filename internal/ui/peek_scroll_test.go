package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// peekScrollFixture draws a popup or pane over the same background, with the
// same terminal grid and populated history, without a core or a live program.
func peekScrollFixture(tb testing.TB, popup bool, scale float32, theme string) (*terminal.Terminal, *myui.Tester) {
	tb.Helper()
	RegisterFonts()
	tb.Setenv("XDG_CONFIG_HOME", tb.TempDir())
	term, err := terminal.New(terminal.Options{
		Conn: newPeekConn(), Font: terminal.Font{Family: TerminalFont, Size: 14},
		Theme:   terminalTheme(ThemeByName(theme)),
		Padding: terminal.Padding{X: 14, Y: 12}, PaddingColor: terminal.PaddingExtend,
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = term.Close() })
	app := NewApp(&herdr.Session{}, ThemeByName(theme))
	app.W, app.H = 1280, 820
	app.overlay, app.peek = "peek", peekState{addr: "t2", term: term}
	pane := herdr.PaneInfo{Pane: "p1", Terminal: "t2"}
	app.terms["p1|t2"] = &paneTerm{term: term}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Box(c).Fill().Background(app.T.Canvas)
		if popup {
			app.overlays(c)
		} else {
			// The pane header is 4 DIPs shorter than the peek's header.
			myui.Column(c).Width(app.W * 0.8).Height(app.H*0.8 + 4).
				Absolute().Left(app.W * 0.1).Top(app.H * 0.1).Children(func() {
				app.paneCard(c, pane, 12, 0)
			})
		}
	}, 1280, 820)
	tt.SetScale(scale)
	var history strings.Builder
	history.WriteString("\x1b[?25l")
	for i := range 3000 {
		fmt.Fprintf(&history, "\x1b[32m%06d\x1b[0m  terminal scroll history %s\r\n", i, strings.Repeat("abcdef ", i%12+1))
	}
	term.Feed([]byte(history.String()))
	tt.Frame()
	tt.Move(640, 410)
	return term, tt
}

// Scrolling repaints the history while the bar stays still, and returning to
// the bottom restores the same screen, at standard and Retina scales.
func TestPeekScrollRepaintsHistory(t *testing.T) {
	for _, tc := range []struct {
		name, theme string
		scale       float32
	}{{"standard", "graphite", 1}, {"retina", "graphite", 2}, {"light", "paper", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			_, tt := peekScrollFixture(t, true, tc.scale, tc.theme)
			region := func(r image.Rectangle) []byte {
				img := tt.Image()
				var out []byte
				for y := r.Min.Y; y < r.Max.Y; y++ {
					start := img.PixOffset(r.Min.X, y)
					out = append(out, img.Pix[start:start+r.Dx()*4]...)
				}
				return out
			}
			scale := int(tc.scale)
			bar := image.Rect(140*scale, 85*scale, 1130*scale, 114*scale)
			body := image.Rect(160*scale, 150*scale, 1100*scale, 700*scale)
			barBefore, bodyBefore := region(bar), region(body)
			tt.Scroll(640, 410, 0, -240)
			if bytes.Equal(bodyBefore, region(body)) {
				t.Fatal("scrolling did not change the rendered history")
			}
			if !bytes.Equal(barBefore, region(bar)) {
				t.Fatal("scrolling changed the popup's header")
			}
			if out := os.Getenv("PEEK_SCROLL_SHOT_DIR"); out != "" {
				f, err := os.Create(filepath.Join(out, tc.name+"-scrolled.png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			tt.Scroll(640, 410, 0, 240)
			if !bytes.Equal(bodyBefore, region(body)) {
				t.Fatal("returning to the bottom did not restore the screen")
			}
		})
	}
}

// BenchmarkPeekScroll compares a popup with a pane at the same window and
// terminal size, scrolling the same history on a Retina display. It measures
// MyGo's headless software renderer, rather than native GPU frame latency.
func BenchmarkPeekScroll(b *testing.B) {
	paneTerm, pane := peekScrollFixture(b, false, 2, "graphite")
	popupTerm, popup := peekScrollFixture(b, true, 2, "graphite")
	cols, rows := paneTerm.Size()
	if pc, pr := popupTerm.Size(); pc != cols || pr != rows {
		b.Fatalf("different grids: pane %dx%d, popup %dx%d", cols, rows, pc, pr)
	}
	b.Logf("terminal grid: %dx%d", cols, rows)
	var paneTime, popupTime time.Duration
	measure := func(tt *myui.Tester, dy float32) time.Duration {
		start := time.Now()
		tt.Scroll(640, 410, 0, dy)
		return time.Since(start)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(-48)
		if i/200%2 != 0 {
			dy = -dy
		}
		// Interleave both views and alternate their order, so changes in
		// machine load affect the two measurements together.
		if i%2 == 0 {
			paneTime += measure(pane, dy)
			popupTime += measure(popup, dy)
		} else {
			popupTime += measure(popup, dy)
			paneTime += measure(pane, dy)
		}
	}
	b.ReportMetric(float64(paneTime.Nanoseconds())/float64(b.N), "pane-ns/scroll")
	b.ReportMetric(float64(popupTime.Nanoseconds())/float64(b.N), "popup-ns/scroll")
	b.ReportMetric(float64(popupTime)/float64(paneTime), "popup/pane")
}
