package ui

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"rove/internal/assets"
)

// Every program icon must draw: a file MyGo cannot parse shows nothing at all,
// in the agents list, the tabs and the pane headers.
func TestProgramIconsParse(t *testing.T) {
	entries, err := fs.ReadDir(assets.Files, "programicons")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		data, err := fs.ReadFile(assets.Files, "programicons/"+e.Name())
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if _, err := ui.ParseSVG(data); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
		n++
	}
	if n < 30 {
		t.Fatalf("only %d program icons", n)
	}
}
