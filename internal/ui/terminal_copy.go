package ui

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
)

// copyTerminalText copies the focused terminal's visible screen and
// scrollback. Programs that take the mouse, as full-screen tools do, leave
// ordinary drags to themselves; this is the reliable copy path there.
func (a *App) copyTerminalText(term *terminal.Terminal) {
	if term == nil {
		return
	}
	text := terminalClipboardText(term.Text())
	if text == "" {
		return
	}
	a.services.WriteClipboard(text)
}

// pasteTerminalText pastes the local clipboard into the terminal.
func (a *App) pasteTerminalText(term *terminal.Terminal) {
	if term == nil {
		return
	}
	if text := a.services.ReadClipboard(); text != "" {
		term.Paste(text)
	}
}

// terminalClipboardText cleans a terminal dump for the clipboard: no
// trailing padding on a row, and no empty rows after the last line.
func terminalClipboardText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// focusedTerminal finds the terminal the keyboard and copy commands act on.
func (a *App) focusedTerminal() *terminal.Terminal {
	if a.S == nil {
		return nil
	}
	p := a.S.FocusedPane()
	if p.Terminal == "" {
		return nil
	}
	a.mu.Lock()
	entry := a.terms[p.Pane+"|"+p.Terminal]
	a.mu.Unlock()
	if entry != nil {
		return entry.term
	}
	return a.terminalFor(p)
}
