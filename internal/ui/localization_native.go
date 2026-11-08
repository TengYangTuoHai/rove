package ui

import (
	"os"

	"github.com/egoist/mygo"

	"rove/internal/i18n"
)

// StartLocalization connects the UI to MyGo's language model on the main
// thread. Menus, native dialogs and text shaping use that same language.
func (a *App) StartLocalization() {
	a.localizationStarted = true
	a.systemLanguage = i18n.Resolve(mygo.App.PreferredLanguages())
	mygo.App.OnLocaleChanged(func(locale string) {
		a.systemLanguage = i18n.Resolve(mygo.App.PreferredLanguages())
		a.localizer.Store(i18n.New(locale))
		a.Repaint()
	})
	if os.Getenv("MYGO_LOCALE") == "" {
		a.setLanguage(a.settings.Language)
	}
	a.localizer.Store(i18n.New(mygo.App.Locale()))
}
