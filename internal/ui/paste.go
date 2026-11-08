package ui

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"

	"rove/internal/herdr"
)

// maxPastedImage is the most bytes terminal.put writes in one go.
const maxPastedImage = 8 << 20

// imageFormats are the clipboard images a program reads, the one wanted
// first first: most agents take PNG, JPEG, GIF and WebP.
var imageFormats = []transfer.Format{
	transfer.PNG,
	"image/jpeg",
	"image/gif",
	"image/webp",
}

// imageFormat is the image a paste holds, if any: one of imageFormats, else
// another image type (an SVG, say) as it is.
func imageFormat(offered []transfer.Format) (transfer.Format, bool) {
	for _, want := range imageFormats {
		for _, have := range offered {
			if have == want {
				return want, true
			}
		}
	}
	for _, have := range offered {
		if strings.HasPrefix(string(have), "image/") {
			return have, true
		}
	}
	return "", false
}

// imageName is the file a pasted image is written as: its type's own
// suffix, so that the program opens it as the image it is.
func imageName(f transfer.Format) string {
	m := string(f)
	sub := m[strings.IndexByte(m, '/')+1:]
	switch sub {
	case "jpeg", "jpg":
		sub = "jpg"
	case "svg+xml":
		sub = "svg"
	}
	var b strings.Builder
	for _, r := range sub {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "pasted-image"
	}
	return "pasted-image." + b.String()
}

// pasteImage takes a paste that holds an image for a terminal on another
// machine: a program there cannot read this computer's clipboard, so the
// image is written beside the terminal on that machine (terminal.put) and its
// path pasted instead, for the program to open as the image it is. It reports
// whether it took the paste: a paste of text alone, or one for a terminal
// here, whose program reads the clipboard itself, is left to the terminal.
func (a *App) pasteImage(term *terminal.Terminal, addr string, d transfer.Data) bool {
	if term == nil || herdr.TerminalMachine(addr) == "" {
		return false
	}
	format, ok := imageFormat(d.Formats())
	if !ok {
		return false
	}
	image, err := d.Read(format)
	if err != nil || len(image) == 0 {
		return false
	}
	if len(image) > maxPastedImage {
		a.toast(a.tr("Cannot paste the image"), a.tr("The image is larger than 8 MB."), "error")
		return true
	}
	go func() {
		put, err := a.S.Put(addr, imageName(format), image)
		if err != nil {
			a.toast(a.tr("Cannot paste the image"), err.Error(), "error")
			return
		}
		term.Paste(put.Path)
	}()
	return true
}
