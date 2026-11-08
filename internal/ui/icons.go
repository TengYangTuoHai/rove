package ui

import (
	"io/fs"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"rove/internal/assets"
)

var (
	iconOnce sync.Once
	icons    = map[string]*ui.SVG{}
)

func loadIcons() {
	for _, dir := range []string{"icons", "programicons"} {
		entries, err := fs.ReadDir(assets.Files, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".svg") {
				continue
			}
			data, err := fs.ReadFile(assets.Files, dir+"/"+e.Name())
			if err != nil {
				continue
			}
			svg, err := ui.ParseSVG(data)
			if err != nil {
				continue
			}
			name := e.Name()[:len(e.Name())-4]
			if dir == "programicons" {
				name = "program:" + name
			}
			icons[name] = svg
		}
	}
}

// iconSVG returns the named icon, nil when the app does not carry it.
func iconSVG(name string) *ui.SVG {
	iconOnce.Do(loadIcons)
	return icons[name]
}
