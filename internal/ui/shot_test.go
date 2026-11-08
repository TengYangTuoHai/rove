package ui

import (
	"image/png"
	"os"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// TestShot renders the window without one, for comparing against Herdr's own
// screenshots. SHOT_OUT names the file, SHOT_OVERLAY an overlay to open.
func TestShot(t *testing.T) {
	out := os.Getenv("SHOT_OUT")
	if out == "" {
		t.Skip("set SHOT_OUT")
	}
	RegisterFonts()
	session, err := herdr.Connect("", nil)
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(nil, ThemeByName("graphite"))
	app.S = session
	if overlay := os.Getenv("SHOT_OVERLAY"); overlay != "" {
		app.OpenOverlay(overlay)
	}
	if peek := os.Getenv("SHOT_PEEK"); peek != "" {
		app.peekTerminal(peek)
		time.Sleep(2 * time.Second)
	}
	if os.Getenv("SHOT_SIDEBAR") == "collapsed" {
		app.SidebarOpen = false
		app.sidebarShown = 0
	}
	if section := os.Getenv("SHOT_SECTION"); section != "" {
		app.settings.Section = section
	}
	if search := os.Getenv("SHOT_SEARCH"); search != "" {
		app.settings.SearchOpen = true
		app.settings.Search = search
	}
	if name := os.Getenv("SHOT_THEME"); name != "" {
		app.settings.Theme = name
		app.applyTheme()
	}
	if os.Getenv("SHOT_SOUND_OFF") != "" {
		app.settings.AlertSound = false
	}
	if os.Getenv("SHOT_COLLAPSE") != "" {
		app.agentsCollapsed = true
	}
	if panel := os.Getenv("SHOT_PANEL"); panel != "" {
		app.OpenPanel(panel)
	}
	if machine := os.Getenv("SHOT_SPACE"); machine != "" {
		if _, _, err := session.NewSpace("", "", machine, nil); err != nil {
			t.Logf("new space on %s: %v", machine, err)
		}
		time.Sleep(1200 * time.Millisecond)
	}
	if target := os.Getenv("SHOT_TARGET"); target != "" {
		app.mach.query = target
		app.addMachine(target)
		time.Sleep(8 * time.Second)
	}
	if action := os.Getenv("SHOT_ACTION"); action != "" {
		app.Run(action)
		time.Sleep(1500 * time.Millisecond)
	}
	if file := os.Getenv("SHOT_FILE"); file != "" {
		app.OpenPanel("files")
		app.files = filesPanel{Root: app.panelRoot(), Open: map[string]bool{}}
		app.openFile(app.panelRoot(), app.panelMachine(), file)
		time.Sleep(1500 * time.Millisecond)
	}
	tt := myui.NewTester(app.View, 1280, 820)
	tt.SetScale(2)
	for i := 0; i < 8; i++ {
		time.Sleep(400 * time.Millisecond)
		tt.Frame()
	}
	img := tt.Image()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}
