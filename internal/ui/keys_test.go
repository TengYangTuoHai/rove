package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseBindingSuperIsPhysicalSuper(t *testing.T) {
	for _, tc := range []struct {
		binding string
		mods    ui.Modifiers
	}{
		{"super-d", ui.Super},
		{"super-shift-d", ui.Super | ui.Shift},
		{"ctrl-d", ui.Ctrl},
		{"cmd-d", ui.Cmd},
	} {
		t.Run(tc.binding, func(t *testing.T) {
			mods, key, ok := parseBinding(tc.binding)
			if !ok || mods != tc.mods || key != ui.KeyD {
				t.Fatalf("parseBinding(%q) = %v, %v, %v", tc.binding, mods, key, ok)
			}
		})
	}
}
