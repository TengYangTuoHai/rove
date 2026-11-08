package ui

import (
	"io/fs"
	"log"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"rove/internal/assets"
)

// RegisterFonts adds JetBrains Mono with Nerd Fonts' prompt and TUI icons
// for terminals, and Geist for the panels. Keep the terminal family name so
// existing settings and terminal views use the bundled glyphs.
func RegisterFonts() {
	for _, font := range []struct{ family, name string }{
		{TerminalFont, "fonts/JetBrainsMonoNerdFont-Regular.ttf"},
		{TerminalFont, "fonts/JetBrainsMonoNerdFont-Bold.ttf"},
		{TerminalFont, "fonts/JetBrainsMonoNerdFont-Italic.ttf"},
		{TerminalFont, "fonts/JetBrainsMonoNerdFont-BoldItalic.ttf"},
		{UIFont, "fonts/Geist.ttf"},
	} {
		data, err := fs.ReadFile(assets.Files, font.name)
		if err != nil {
			log.Printf("font %s: %v", font.name, err)
			continue
		}
		if err := ui.RegisterFont(data, font.family); err != nil {
			log.Printf("font %s: %v", font.name, err)
		}
	}
}

func (a *App) interfaceFont() string {
	if a.settings.InterfaceFont == "" {
		return UIFont
	}
	return a.settings.InterfaceFont
}

// fontSize preserves the type hierarchy around the chosen 13 DIP base.
func (a *App) fontSize(size float32) float32 {
	if a.settings.InterfaceSize == 0 {
		return size
	}
	return size * float32(a.settings.InterfaceSize) / 13
}

func (a *App) currentTerminalFont() terminal.Font {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.terminalStyle
}

// applyTerminalFont changes attached views in place, including hidden panes
// and peek. New attachments take terminalStyle under the same lock.
func (a *App) applyTerminalFont() {
	font := terminal.Font{Family: a.settings.TerminalFont, Size: float32(a.settings.TerminalSize)}
	a.mu.Lock()
	defer a.mu.Unlock()
	if font.Family == a.terminalStyle.Family && font.Size == a.terminalStyle.Size {
		return
	}
	a.terminalStyle = font
	for _, entry := range a.terms {
		entry.term.SetFont(font)
	}
	if a.peek.term != nil {
		a.peek.term.SetFont(font)
	}
}

func (a *App) fontNames(mono bool) []string {
	if a.fontFamilies == nil {
		a.fontFamilies = ui.FontFamilies()
	}
	names := []string{"system-ui", UIFont}
	if mono {
		// Nerd Font icon advances make the bundled terminal family fail
		// system monospace detection, despite its fixed-width text cells.
		names = []string{"monospace", TerminalFont}
	}
	for _, family := range a.fontFamilies {
		if family.Name != names[1] && (!mono || family.Monospaced) {
			names = append(names, family.Name)
		}
	}
	return names
}
