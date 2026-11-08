package ui

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestMachineDotSaysWhetherItAnswers draws a machine's header once for a
// machine that answers and once for one that does not: the dot must take the
// color of that state, not the one grey it used to wear for every machine.
func TestMachineDotSaysWhetherItAnswers(t *testing.T) {
	RegisterFonts()
	th := ThemeByName("graphite")
	for _, c := range []struct {
		state string
		want  ui.Color
	}{
		{"connected", th.Ansi[2]},
		{"unreachable", th.Blocked},
	} {
		app := NewApp(&herdr.Session{}, th)
		machines := []*herdr.Machine{{Name: "gpu", State: c.state}}
		tt := ui.NewTester(func(ctx *ui.Context) {
			ui.Column(ctx).FillWidth().Padding(8).Children(func() {
				app.machineGroup(ctx, "gpu", nil, machines, nil)
			})
		}, 260, 60)
		tt.SetScale(2)
		tt.Frame()
		if !hasColor(tt.Image(), c.want) {
			t.Errorf("a %s machine's header draws no dot in its color %v", c.state, c.want)
		}
	}
}

// hasColor reports whether any pixel of img is exactly c.
func hasColor(img image.Image, c ui.Color) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if uint8(r>>8) == c.R && uint8(g>>8) == c.G && uint8(bl>>8) == c.B && a != 0 {
				return true
			}
		}
	}
	return false
}
