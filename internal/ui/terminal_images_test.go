package ui

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// dsh-tui uploads compressed RGBA in chunks, then places the image separately
// below text. Exercise that protocol in both of Rove's terminal surfaces.
func TestTerminalImagesInPaneAndPeek(t *testing.T) {
	RegisterFonts()
	for _, surface := range []string{"pane", "peek"} {
		t.Run(surface, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			conn := &terminalSettingsConn{peekConn: newPeekConn(), writes: make(chan struct{}, 1)}
			opts := app.currentTerminalOptions()
			opts.Conn, opts.Theme = conn, terminalTheme(app.T)
			term, err := terminal.New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			app.terms["image-pane|t1"] = &paneTerm{term: term}
			app.W, app.H = 1280, 820
			if surface == "peek" {
				app.overlay = "peek"
				app.peek = peekState{addr: "t1", term: term}
			}
			tt := myui.NewTester(func(c *myui.Context) {
				if surface == "peek" {
					app.overlays(c)
				} else {
					app.terminalView(c, herdr.PaneInfo{Pane: "image-pane", Terminal: "t1", Focused: true})
				}
			}, 1280, 820)
			tt.Frame()
			term.Feed([]byte("\x1b[?1049h\x1b[?25l\x1b_Gi=31,s=1,v=1,a=q,t=d,f=32;AAAAAA==\x1b\\"))
			conn.waitFor(t, "\x1b_Gi=31;OK\x1b\\")

			var compressed bytes.Buffer
			writer := zlib.NewWriter(&compressed)
			pixels := bytes.Repeat([]byte{251, 43, 127, 255}, 32*32)
			if _, err := writer.Write(pixels); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			payload := base64.StdEncoding.EncodeToString(compressed.Bytes())
			var upload strings.Builder
			for start := 0; start < len(payload); start += 16 {
				end := min(start+16, len(payload))
				more := 0
				if end < len(payload) {
					more = 1
				}
				control := ""
				if start == 0 {
					control = "a=t,i=7,f=32,s=32,v=32,o=z,q=2,"
				}
				fmt.Fprintf(&upload, "\x1b_G%sm=%d;%s\x1b\\", control, more, payload[start:end])
			}
			upload.WriteString("\x1b[3;4H\x1b_Ga=p,i=7,p=1,c=4,r=2,z=-1073741824,C=1,q=1\x1b\\")
			// Stream reads can split an APC header, payload or terminator.
			data := []byte(upload.String())
			for len(data) > 0 {
				n := min(len(data), 7)
				term.Feed(data[:n])
				data = data[n:]
			}
			tt.Frame()
			count := func(img image.Image) int {
				n := 0
				for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
					for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
						r, g, b, _ := img.At(x, y).RGBA()
						if r>>8 == 251 && g>>8 == 43 && b>>8 == 127 {
							n++
						}
					}
				}
				return n
			}
			if got := count(tt.Image()); got < 500 {
				t.Fatalf("image painted only %d pixels", got)
			}
			term.Feed([]byte("\x1b_Ga=d,d=I,i=7,q=2\x1b\\"))
			tt.Frame()
			if got := count(tt.Image()); got != 0 {
				t.Fatalf("deleted image still painted %d pixels", got)
			}
		})
	}
}
