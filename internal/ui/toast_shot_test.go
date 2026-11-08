package ui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestToastShot captures the real window renderer with the reported warning,
// without connecting to or changing the person's running core session.
func TestToastShot(t *testing.T) {
	dir := os.Getenv("TOAST_SHOT_DIR")
	if dir == "" {
		t.Skip("set TOAST_SHOT_DIR to capture notification evidence")
	}
	RegisterFonts()
	for _, theme := range []string{"paper", "graphite"} {
		app := NewApp(&herdr.Session{}, ThemeByName(theme))
		app.toast("Cannot show the terminal", "t1 has finished", "error")
		tt := myui.NewTester(app.View, 800, 600)
		tt.Frame()
		save := func(name string) {
			t.Helper()
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		save(theme + ".png")
		if theme != "paper" {
			continue
		}
		for frame := range 36 {
			if frame == 8 {
				if err := tt.Click("Dismiss notification"); err != nil {
					t.Fatal(err)
				}
				if tt.HasText("t1 has finished") {
					t.Fatal("dismissed toast is still visible")
				}
			}
			if frame == 12 {
				app.toast("Cannot show the terminal", "t1 has finished", "error")
			}
			tt.Frame()
			save(fmt.Sprintf("frame-%03d.png", frame))
			time.Sleep(250 * time.Millisecond)
		}
		tt.Frame()
		if tt.HasText("t1 has finished") {
			t.Fatal("toast is still visible after its timeout")
		}
		save("expired.png")
	}
}
