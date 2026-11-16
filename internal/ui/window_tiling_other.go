//go:build !darwin

package ui

type windowFillFrame struct{}

func (a *App) titleBarDoubleClicked() {}

func (a *App) toggleWindowMaximize() { a.Win.ToggleMaximize() }
