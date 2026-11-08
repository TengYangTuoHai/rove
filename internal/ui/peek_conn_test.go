package ui

import (
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

type peekSizedStream struct {
	*peekConn
	reader io.Reader
	sizes  chan [4]int
	themes chan *terminal.Theme
}

func (c *peekSizedStream) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *peekSizedStream) ResizePixels(cols, rows, cellW, cellH int) error {
	c.sizes <- [4]int{cols, rows, cellW, cellH}
	return nil
}
func (c *peekSizedStream) UseTheme(th *terminal.Theme) error {
	c.themes <- th
	return nil
}

func peekReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("peek connection did not answer")
		var zero T
		return zero
	}
}

// The first snapshot is painted at the popup's measured grid, including on a
// Retina screen and with app zoom, with no small screen reflowing into it.
func TestPeekFirstSnapshotFitsPopup(t *testing.T) {
	RegisterFonts()
	for _, tc := range []struct {
		name        string
		scale, zoom float32
	}{{"standard", 1, 1}, {"retina", 2, 1}, {"zoom", 2, 1.25}} {
		t.Run(tc.name, func(t *testing.T) {
			opened := make(chan [4]int, 1)
			reader, writer := io.Pipe()
			defer writer.Close()
			stream := &peekSizedStream{peekConn: newPeekConn(), reader: reader, sizes: make(chan [4]int, 8), themes: make(chan *terminal.Theme, 8)}
			conn := newPeekConnection(func(cols, rows, cellW, cellH int) (peekStream, error) {
				opened <- [4]int{cols, rows, cellW, cellH}
				var screen strings.Builder
				screen.WriteString("\x1b[?1049h\x1b[?25l\x1b[48;2;45;70;90m\x1b[2J")
				for row := 1; row <= rows; row++ {
					fmt.Fprintf(&screen, "\x1b[%d;1H|%s|", row, strings.Repeat(".", cols-2))
				}
				fmt.Fprintf(&screen, "\x1b[1;3HFIRST SNAPSHOT %dx%d\x1b[%d;%dHEND", cols, rows, rows, cols-3)
				go func() { _, _ = io.WriteString(writer, screen.String()) }()
				return stream, nil
			})
			term, err := terminal.New(terminal.Options{
				Conn: conn, Font: terminal.Font{Family: TerminalFont, Size: 14},
				Theme: terminalTheme(ThemeByName("graphite")), Padding: terminal.Padding{X: 14, Y: 12},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.overlay, app.peek = "peek", peekState{addr: "t2", term: term}
			app.W, app.H = 1280/tc.zoom, 820/tc.zoom
			show := false
			tt := myui.NewTester(func(c *myui.Context) {
				c.SetZoom(tc.zoom)
				if show {
					app.overlays(c)
				}
			}, 1280, 820)
			tt.SetScale(tc.scale)
			select {
			case size := <-opened:
				t.Fatalf("attached before any layout at %v", size)
			default:
			}
			show = true
			tt.Frame()
			size := peekReceive(t, opened)
			// Zoom enlarges cells in the same physical popup, so fewer rows fit.
			if size[0] <= 80 || size[1] < int(24/tc.zoom) {
				t.Fatalf("initial grid = %v, want the popup's larger grid at zoom %g", size, tc.zoom)
			}
			if cols, rows := term.Size(); cols != size[0] || rows != size[1] {
				t.Fatalf("initial grid = %v, rendered terminal = %dx%d", size, cols, rows)
			}
			deadline := time.Now().Add(3 * time.Second)
			for !strings.Contains(term.Text(), "END") {
				if time.Now().After(deadline) {
					t.Fatal("initial snapshot did not arrive")
				}
				time.Sleep(time.Millisecond)
			}
			lines := strings.Split(strings.TrimRight(term.Text(), "\n"), "\n")
			if len(lines) != size[1] || len(lines[0]) != size[0] || !strings.HasSuffix(lines[len(lines)-1], "END|") {
				t.Fatalf("first snapshot wrapped at the wrong size: grid=%v, text=%q", size, term.Text())
			}
			tt.Frame()
			img := tt.Image()
			filled := 0
			for y := 640 * tc.scale; y < 660*tc.scale; y++ {
				for x := 1080 * tc.scale; x < 1100*tc.scale; x++ {
					r, g, b, _ := img.At(int(x), int(y)).RGBA()
					if r>>8 == 45 && g>>8 == 70 && b>>8 == 90 {
						filled++
					}
				}
			}
			if filled < int(200*tc.scale*tc.scale) {
				t.Fatal("first snapshot did not fill the lower right of the popup")
			}
			if out := os.Getenv("PEEK_SIZING_SHOT_DIR"); out != "" {
				f, err := os.Create(filepath.Join(out, tc.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, img)
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("first snapshot: %d×%d, cell %d×%d pixels", size[0], size[1], size[2], size[3])
			if err := conn.ResizePixels(size[0]+10, size[1]+5, size[2], size[3]); err != nil {
				t.Fatal(err)
			}
			if got := peekReceive(t, stream.sizes); got[0] != size[0]+10 || got[1] != size[1]+5 {
				t.Fatalf("later resize = %v", got)
			}
		})
	}
}

func TestPeekConnectionKeepsInputAndTheme(t *testing.T) {
	stream := &peekSizedStream{peekConn: newPeekConn(), sizes: make(chan [4]int, 2), themes: make(chan *terminal.Theme, 2)}
	opens := 0
	conn := newPeekConnection(func(int, int, int, int) (peekStream, error) {
		opens++
		return stream, nil
	})
	defer conn.Close()
	th := terminalTheme(ThemeByName("paper"))
	_ = conn.UseTheme(th)
	written := make(chan error, 1)
	go func() { _, err := conn.Write([]byte("early keys")); written <- err }()
	if err := conn.ResizePixels(120, 36, 9, 19); err != nil {
		t.Fatal(err)
	}
	if got := peekReceive(t, stream.themes); got != th {
		t.Fatal("theme changed before attachment was lost")
	}
	if err := peekReceive(t, written); err != nil || stream.typed() != "early keys" {
		t.Fatalf("input before attachment = %q, %v", stream.typed(), err)
	}
	_ = conn.ResizePixels(130, 40, 9, 19)
	if opens != 1 {
		t.Fatalf("resizing attached %d times", opens)
	}
	_ = conn.Close()
	peekReceive(t, stream.closed)
	if _, err := conn.Write([]byte("late keys")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after close = %v", err)
	}
}

func TestPeekConnectionClosesBeforeLayout(t *testing.T) {
	conn := newPeekConnection(func(int, int, int, int) (peekStream, error) {
		t.Error("a closed peek attached")
		return nil, errors.New("unexpected attachment")
	})
	read := make(chan error, 1)
	go func() { _, err := conn.Read(make([]byte, 1)); read <- err }()
	_ = conn.Close()
	if err := peekReceive(t, read); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("read after close = %v", err)
	}
	if err := conn.ResizePixels(120, 36, 9, 19); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("resize after close = %v", err)
	}
}

func TestPeekConnectionClosesDuringAttach(t *testing.T) {
	started, finish, resized := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	stream := &peekSizedStream{peekConn: newPeekConn()}
	conn := newPeekConnection(func(int, int, int, int) (peekStream, error) {
		close(started)
		<-finish
		return stream, nil
	})
	go func() { resized <- conn.ResizePixels(120, 36, 9, 19) }()
	peekReceive(t, started)
	_ = conn.Close()
	close(finish)
	if err := peekReceive(t, resized); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("attach after close = %v", err)
	}
	peekReceive(t, stream.closed)
}

func TestPeekConnectionReportsAttachFailure(t *testing.T) {
	want := errors.New("attach failed")
	conn := newPeekConnection(func(int, int, int, int) (peekStream, error) { return nil, want })
	defer conn.Close()
	if err := conn.ResizePixels(120, 36, 9, 19); !errors.Is(err, want) {
		t.Fatalf("resize = %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, want) {
		t.Fatalf("read = %v", err)
	}
}
