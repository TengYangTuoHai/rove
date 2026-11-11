package ui

import (
	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// terminalFailureView leaves an inaccessible pane actionable without replacing a
// running terminal. A finished terminal can be replaced only on explicit request.
func (a *App) terminalFailureView(c *ui.Context, p herdr.PaneInfo) {
	key := p.Pane + "|" + p.Terminal
	a.mu.Lock()
	failed, recovering := a.failedTerms[key], a.recovering[key]
	a.mu.Unlock()
	if !failed {
		return
	}
	record := a.S.Terminal(p.Terminal)
	finished := record != nil && record.State == "finished"
	ui.Box(c).Fill().Padding(12).Center().Children(func() {
		ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
			message := a.tr("The terminal could not be opened.")
			if finished {
				message = a.tr("This terminal has finished. A new shell cannot restore the previous process.")
			}
			ui.Text(c, message).FontSize(a.fontSize(12)).TextColor(a.T.Dim).Wrap().TextAlign(ui.Center)
			if recovering {
				ui.Text(c, a.tr("Starting a new shell…")).FontSize(a.fontSize(12)).TextColor(a.T.Dim)
			} else if finished {
				a.smallButton(c, a.tr("Start a new shell"), func() { a.recoverPane(p) })
			} else {
				a.smallButton(c, a.tr("Retry"), func() { a.retryTerminal(key) })
			}
		})
	})
}

func (a *App) retryTerminal(key string) {
	a.mu.Lock()
	delete(a.failedTerms, key)
	a.mu.Unlock()
	a.Repaint()
}

func (a *App) recoverPane(p herdr.PaneInfo) {
	key := p.Pane + "|" + p.Terminal
	a.mu.Lock()
	if a.recovering[key] {
		a.mu.Unlock()
		return
	}
	a.recovering[key] = true
	a.mu.Unlock()
	a.Repaint()
	go func() {
		_, err := a.S.RecoverPane(p.Pane, p.Terminal)
		a.mu.Lock()
		delete(a.recovering, key)
		a.mu.Unlock()
		if err != nil {
			a.toast(a.tr("Cannot start a new shell"), err.Error(), "error")
		}
		a.Repaint()
	}()
}
