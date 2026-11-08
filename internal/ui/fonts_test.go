package ui

import (
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"
	"golang.org/x/image/font/sfnt"

	"rove/internal/assets"
)

// Prompt and TUI icons live in Unicode's private-use areas, so system
// fallback fonts cannot be relied on to supply them on a fresh install.
func TestBundledTerminalFontGlyphs(t *testing.T) {
	names, err := fs.Glob(assets.Files, "fonts/JetBrainsMono*.ttf")
	if err != nil || len(names) == 0 {
		t.Fatalf("terminal fonts: %v, %v", names, err)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(assets.Files, name)
			if err != nil {
				t.Fatal(err)
			}
			font, err := sfnt.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			var buf sfnt.Buffer
			for _, r := range "A→✓─⠋\ue0a0\ue0b0\ue627\uea60\uf013\uf408\U000f07c0" {
				glyph, err := font.GlyphIndex(&buf, r)
				if err != nil || glyph == 0 {
					t.Errorf("missing glyph %U: index=%d, err=%v", r, glyph, err)
				}
			}
		})
	}
}

// TestTerminalFontShot renders actual terminal cells, including all four
// font styles and system fallback text. FONT_SHOT_OUT keeps the screenshot.
func TestTerminalFontShot(t *testing.T) {
	out := os.Getenv("FONT_SHOT_OUT")
	if out == "" {
		t.Skip("set FONT_SHOT_OUT")
	}
	RegisterFonts()
	term, err := terminal.New(terminal.Options{
		Conn:  newPeekConn(),
		Font:  terminal.Font{Family: TerminalFont, Size: 16},
		Theme: terminalTheme(ThemeByName("graphite")),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := myui.NewTester(func(c *myui.Context) {
		terminal.View(c, term).Fill()
	}, 850, 300)
	tt.SetScale(2)
	tt.Frame()
	const icons = "\ue0a0 \ue0b0 \ue627 \uea60 \uf013 \uf408 \U000f07c0"
	var output strings.Builder
	output.WriteString("\x1b[?25lJetBrains Mono - bundled Nerd Font glyphs\r\n\r\n")
	for _, style := range []struct{ label, sgr string }{
		{"Regular", "0"}, {"Bold", "1"}, {"Italic", "3"}, {"Bold italic", "1;3"},
	} {
		fmt.Fprintf(&output, "%-12s\x1b[%sm%s\x1b[0m  ABC 012 -> =>\r\n", style.label, style.sgr, icons)
	}
	output.WriteString("\r\nSymbols     → ✓ ─ │ ┌ ┐ └ ┘ ⠋ ⠙ ⠹ ⠸\r\n")
	output.WriteString("Fallback    中文 日本語 한국어 😀 🚀\r\n")
	output.WriteString("Alignment   |1234567890|1234567890|\r\n")
	term.Feed([]byte(output.String()))
	tt.Frame()
	if !strings.Contains(term.Text(), icons) {
		t.Fatal("terminal did not preserve the icon codepoints")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}
