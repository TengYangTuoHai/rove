package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// Native buttons sit over the content. Header actions must stay outside their
// measured area, including Linux desktops that put controls on the left.
func TestHeadersLeaveRoomForNativeWindowControls(t *testing.T) {
	RegisterFonts()
	for _, tc := range []struct {
		name string
		bar  myui.TitleBar
	}{
		{"windows", myui.TitleBar{Height: 42, Right: 138}},
		{"linux-right", myui.TitleBar{Height: 48, Right: 120}},
		{"linux-left", myui.TitleBar{Height: 48, Left: 120}},
		{"linux-left-wide", myui.TitleBar{Height: 48, Left: 204}},
		{"linux-both", myui.TitleBar{Height: 54, Left: 40, Right: 80}},
		{"fullscreen-or-no-buttons", myui.TitleBar{}},
	} {
		for _, open := range []bool{true, false} {
			name := tc.name + "-collapsed"
			if open {
				name = tc.name + "-sidebar"
			}
			t.Run(name, func(t *testing.T) {
				app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
				app.SidebarOpen = open
				if !open {
					app.sidebarShown = 0
				}
				tt := myui.NewTester(app.View, 1000, 600)
				tt.SetTitleBar(tc.bar)
				if dir := os.Getenv("CHROME_SHOT_DIR"); dir != "" {
					writeChromeShot(t, tt, filepath.Join(dir, name+".png"))
				}
				labels := []string{"Files", "Changes"}
				if open {
					labels = append(labels, "Search", "Hide sidebar", "Rove")
				} else {
					labels = append(labels, "Show sidebar")
				}
				check := func(bar myui.TitleBar) {
					t.Helper()
					for _, label := range labels {
						r, ok := tt.Find(label)
						if !ok || r.W <= 0 || r.H <= 0 {
							t.Errorf("missing visible header action %q", label)
							continue
						}
						if r.Y < bar.Height && (r.X < bar.Left || r.X+r.W > 1000-bar.Right) {
							t.Errorf("%s at %+v overlaps native controls (left %v, right %v)", label, r, bar.Left, bar.Right)
						}
						if label == "Hide sidebar" && r.X+r.W > app.sidebarShown*app.Zoom {
							t.Errorf("sidebar toggle at %+v is clipped by the sidebar", r)
						}
					}
					if open {
						r, ok := tt.Find("New space")
						if !ok || r.Y < bar.Height {
							t.Errorf("sidebar content starts inside the native title bar: %+v", r)
						}
					}
				}
				check(tc.bar)
				// Full screen hides the controls, then restoring the window brings
				// their reservation back without restarting the app.
				tt.SetTitleBar(myui.TitleBar{})
				check(myui.TitleBar{})
				tt.SetTitleBar(tc.bar)
				check(tc.bar)
				app.Zoom = 1.5
				tt.Frame()
				check(tc.bar)
				label := "Show sidebar"
				if open {
					label = "Hide sidebar"
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if app.SidebarOpen == open {
					t.Fatal("sidebar toggle no longer responds beside the native controls")
				}
			})
		}
	}
}

// A desktop theme can request a taller header than Rove's usual tab row.
func TestNativeTitleBarHeightKeepsContentBelowControls(t *testing.T) {
	for _, header := range []string{"brand", "tabs"} {
		t.Run(header, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			tt := myui.NewTester(func(c *myui.Context) {
				app.W, app.H = c.Size()
				myui.Column(c).Fill().Children(func() {
					if header == "brand" {
						app.brand(c)
					} else {
						app.tabsBar(c)
					}
					myui.Text(c, "Content below header")
				})
			}, 1000, 600)
			tt.SetTitleBar(myui.TitleBar{Height: 64, Left: 120})
			r, ok := tt.Find("Content below header")
			if !ok || r.Y < 64 {
				t.Fatalf("content at %+v overlaps the 64 DIP native title bar", r)
			}
		})
	}
}

func writeChromeShot(t *testing.T, tt *myui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
