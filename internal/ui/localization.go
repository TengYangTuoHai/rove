package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"rove/internal/i18n"
)

var languageCodes = []string{"system", "en", "zh", "ja"}
var languageNames = []string{"System", "English", "中文", "日本語"}

func validLanguage(code string) bool {
	for _, supported := range languageCodes {
		if code == supported {
			return true
		}
	}
	return false
}

// tr translates only application messages. User titles, paths, file contents
// and terminal output are never passed through a catalog.
func (a *App) tr(id string, args ...any) string {
	l := a.localizer.Load()
	if l == nil {
		l = i18n.New(a.systemLanguage)
	}
	return l.Text(id, args...)
}

func (a *App) setLanguage(code string) {
	if !validLanguage(code) {
		code = "system"
	}
	a.settings.Language = code
	locale := code
	if code == "system" {
		locale = a.systemLanguage
	}
	a.localizer.Store(i18n.New(locale))
	if a.localizationStarted {
		override := code
		if override == "system" {
			override = ""
		}
		mygo.App.SetLocale(override)
	}
	a.Repaint()
}

func (a *App) languageName() string {
	for i, code := range languageCodes {
		if code == a.settings.Language {
			return a.tr(languageNames[i])
		}
	}
	return a.tr("System")
}

func (a *App) settingsLanguage(c *ui.Context) {
	at := 0
	for i, code := range languageCodes {
		if code == a.settings.Language {
			at = i
		}
	}
	a.settingsSegmentedOn(c, languageNames, &at, func(i int) { a.setLanguage(languageCodes[i]) })
}
