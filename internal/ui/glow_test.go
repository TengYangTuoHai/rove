package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// The wash belongs behind pane surfaces, including the header controls and
// terminal output nearest the strongest part of the top-right glow.
func TestThemeGlowStaysBehindTerminal(t *testing.T) {
	RegisterFonts()
	for _, name := range []string{"petal", "dusk", "neutral", "Catppuccin Latte", "Catppuccin Mocha", "plain"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%v", name, dark), func(t *testing.T) {
				th := themeForAppearance(name, dark)
				app := NewApp(&herdr.Session{}, th)
				pane := herdr.PaneInfo{Pane: "p1", Terminal: "t1", Title: "Terminal readability", Focused: true}
				term, err := terminal.New(terminal.Options{
					Conn: newPeekConn(), Font: terminal.Font{Family: TerminalFont, Size: 14}, Theme: terminalTheme(th),
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { term.Close() })
				app.terms["p1|t1"] = &paneTerm{term: term}
				tt := myui.NewTester(func(c *myui.Context) {
					app.W, app.H = c.Size()
					th.windowFrame(c).Children(func() {
						app.sidebar(c)
						myui.Box(c).Grow(1).FillHeight().Padding(12).Children(func() {
							app.paneCard(c, pane, 10, 0)
						})
					})
				}, 1280, 720)
				tt.SetScale(1)
				tt.Frame()
				tt.Move(1200, 27)
				term.Feed([]byte("\x1b[?25l\x1b[1;75Htop-right: readable\r\n\x1b[2;75H\x1b[31merror\x1b[0m \x1b[32mok\x1b[0m \x1b[34minfo\x1b[0m"))
				tt.Frame()
				withGlow := tt.Image()
				if dir := os.Getenv("THEME_SHOT_DIR"); dir != "" {
					saveThemeShot(t, tt, filepath.Join(dir, fmt.Sprintf("glow-%s-%v.png", name, dark)))
				}
				glow := th.Glow
				th.Glow = myui.LinearGradient{}
				tt.Frame()
				withoutGlow := tt.Image()
				th.Glow = glow
				if len(glow.Stops) > 0 && glow.Stops[0].Color.A > 0 && withGlow.At(1278, 2) == withoutGlow.At(1278, 2) {
					t.Fatal("glow disappeared from the exposed window background")
				}
				// Include the header's buttons, terminal text and empty background.
				// Leave the rounded edge outside the sample; the cursor is hidden.
				for y := 16; y < 160; y++ {
					for x := 900; x < 1250; x++ {
						at := image.Pt(x, y)
						if withGlow.At(at.X, at.Y) != withoutGlow.At(at.X, at.Y) {
							t.Fatalf("glow paints over pane at %v: with %v, without %v", at, withGlow.At(x, y), withoutGlow.At(x, y))
						}
					}
				}
			})
		}
	}
}
