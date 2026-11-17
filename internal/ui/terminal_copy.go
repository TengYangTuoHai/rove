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

// pasteEventsOff clears the Kitty paste-events mode (OSC 5522) in the view:
// what the program reads back after a paste event is not this view's to
// answer. A pane shows a terminal that herdr-core keeps: the program's
// output goes through the core's own screen too, and that screen has no
// clipboard, so it refuses every clipboard read with EPERM. A program that
// asks for the paste itself (omp's enhanced paste) is offered an event,
// reads it back, gets the core's EPERM and drops the paste. With the mode
// off the program gets the paste as text, bracketed when it asked (mode
// 2004), which needs no read.
const pasteEventsOff = "\x1b[?5522l"

// pasteTextInto pastes text, as an ordinary paste: the program reads it as
// input instead of reading the clipboard itself (see pasteEventsOff).
func pasteTextInto(term *terminal.Terminal, text string) {
	if term == nil || text == "" {
		return
	}
	term.Feed([]byte(pasteEventsOff))
	term.Paste(text)
}

// pasteTerminalText pastes the local clipboard into the terminal.
func (a *App) pasteTerminalText(term *terminal.Terminal) {
	if term == nil {
		return
	}
	if text := a.services.ReadClipboard(); text != "" {
		pasteTextInto(term, text)
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
