// Rove is a window for herdr-core: spaces, tabs and panes of terminals and
// agents, drawn with MyGo's native UI.
package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
	myui "rove/internal/ui"
)

func main() {
	myui.RegisterFonts()
	theme := myui.ThemeByName("graphite")
	app := myui.NewApp(nil, theme)
	app.LoadSettings()
	mygo.App.WhenReady(func() {
		app.StartLocalization()
		myui.InstallMenu()
		point := mygo.Point{X: 12, Y: 16}
		app.Win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "Rove",
			Width:                1280,
			Height:               820,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       42,
			TrafficLightPosition: &point,
			BackgroundColor:      "light-dark(#f6f7f9, #131518)",
			Content:              ui.View(app.View),
		})
		// Start in the desktop's appearance, and follow it while the
		// appearance asks for the system's.
		app.SetSystemAppearance(mygo.Theme.IsDark())
		app.StartAppearance()
		mygo.Theme.OnUpdated(func() {
			if mygo.Theme.Source() != mygo.ThemeSystem {
				return // the window forces light or dark
			}
			app.SetSystemAppearance(mygo.Theme.IsDark())
		})
		session, err := herdr.Connect("", app.Repaint)
		if err != nil {
			app.Fail(err)
			return
		}
		app.Attach(session)
		app.ServeControl()
		if overlay := os.Getenv("ROVE_OVERLAY"); overlay != "" {
			app.OpenOverlay(overlay)
		}
		if shot := os.Getenv("ROVE_SHOT"); shot != "" {
			go func() {
				delay := 4 * time.Second
				if v := os.Getenv("ROVE_SHOT_DELAY"); v != "" {
					if secs, err := strconv.Atoi(v); err == nil {
						delay = time.Duration(secs) * time.Second
					}
				}
				time.Sleep(delay)
				png, err := app.Win.CapturePage()
				if err != nil {
					log.Printf("capture: %v", err)
				} else if err := os.WriteFile(shot, png, 0o644); err != nil {
					log.Printf("capture: %v", err)
				} else {
					log.Printf("captured %s", shot)
				}
				if os.Getenv("ROVE_EXIT") != "" {
					app.Win.Close()
				}
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}
