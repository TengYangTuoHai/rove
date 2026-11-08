package ui

import (
	"path/filepath"
	"testing"

	myui "github.com/egoist/mygo/ui"
)

func BenchmarkUnchangedSettings(b *testing.B) {
	app := NewApp(nil, ThemeByName("graphite"))
	app.settingsPath = filepath.Join(b.TempDir(), "settings.json")
	app.saveSettings()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.saveSettings()
	}
}

// BenchmarkLogoFrame measures the work of drawing the logo with MyGo's
// headless renderer. Native GPU rendering and full-window costs differ.
func BenchmarkLogoFrame(b *testing.B) {
	app := NewApp(nil, ThemeByName("graphite"))
	tt := myui.NewTester(func(c *myui.Context) { app.logo(c, 20) }, 64, 64)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}
