package ui

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"rove/internal/herdr"
)

// The same gesture can scroll locally, report a mouse wheel, or send arrow
// keys, depending on modes set by the program. A fullscreen program's history
// is not the terminal's scrollback: simply suppressing reports loses access to it.
func TestPeekScrollRouting(t *testing.T) {
	for _, tc := range []struct {
		name, modes, report string
		local               bool
	}{
		{name: "native", local: true},
		{name: "mouse", modes: "\x1b[?1000h\x1b[?1006h", report: "mouse"},
		{name: "mouse_disabled_again", modes: "\x1b[?1000h\x1b[?1006h\x1b[?1000l", local: true},
		{name: "fullscreen_mouse", modes: "\x1b[?1049h\x1b[?1000h\x1b[?1006h", report: "mouse"},
		{name: "fullscreen_arrows", modes: "\x1b[?1049h\x1b[?1007h", report: "arrows"},
		{name: "fullscreen_no_reports", modes: "\x1b[?1049h\x1b[?1007l"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterFonts()
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			conn := newPeekConn()
			term, err := terminal.New(terminal.Options{
				Conn: conn, Font: terminal.Font{Family: TerminalFont, Size: 14},
				Theme:   terminalTheme(ThemeByName("graphite")),
				Padding: terminal.Padding{X: 14, Y: 12}, PaddingColor: terminal.PaddingExtend,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.overlay, app.peek = "peek", peekState{addr: "t2", term: term}
			tt := peekWindow(t, app)
			var output strings.Builder
			for i := range 200 {
				fmt.Fprintf(&output, "history %03d\r\n", i)
			}
			output.WriteString(tc.modes)
			output.WriteString("\x1b[?25l")
			if strings.Contains(tc.name, "fullscreen") {
				output.WriteString("\x1b[Hfullscreen agent viewport (older content lives in the agent)")
			}
			term.Feed([]byte(output.String()))
			tt.Frame()
			tt.Move(640, 410)
			before := bytes.Clone(tt.Image().Pix)
			tt.Scroll(640, 410, 0, -240)
			changed := !bytes.Equal(before, tt.Image().Pix)
			if changed != tc.local {
				t.Fatalf("local viewport changed = %v, want %v", changed, tc.local)
			}

			// Send is ordered after the reports on the terminal's input queue;
			// observing it avoids mistaking an asynchronous write for no report.
			const barrier = "scroll-routing-barrier"
			term.Send([]byte(barrier))
			deadline := time.Now().Add(2 * time.Second)
			for !strings.HasSuffix(conn.typed(), barrier) {
				if time.Now().After(deadline) {
					t.Fatal("terminal input queue did not reach the barrier")
				}
				time.Sleep(time.Millisecond)
			}
			got := strings.TrimSuffix(conn.typed(), barrier)
			switch tc.report {
			case "mouse":
				if !strings.HasPrefix(got, "\x1b[<64;") || strings.Count(got, "\x1b[<64;") < 2 {
					t.Fatalf("expected repeated SGR wheel-up reports, got %q", got)
				}
			case "arrows":
				if strings.Count(got, "\x1b[A") < 2 || strings.ReplaceAll(got, "\x1b[A", "") != "" {
					t.Fatalf("expected up-arrow reports, got %q", got)
				}
			default:
				if got != "" {
					t.Fatalf("local scrolling sent input to the program: %q", got)
				}
			}
			t.Logf("local viewport changed=%v; program input=%q", changed, got)
			if out := os.Getenv("PEEK_SCROLL_SHOT_DIR"); out != "" {
				f, err := os.Create(filepath.Join(out, "routing-"+tc.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
