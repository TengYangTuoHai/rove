package herdr

import (
	"reflect"
	"testing"
)

func TestPaneKeysTracksVisibleAndReplacedTerminals(t *testing.T) {
	s := performanceSession(3)
	s.query["pane-0-1"] = "replacement"
	delete(s.tabs, "tab-2") // Panes omits tabs whose view is not initialized.
	visible, live := s.PaneKeys()
	wantVisible := map[string]bool{"pane-0-0|term-0-0": true, "pane-0-1|replacement": true}
	wantLive := map[string]bool{"pane-0-0|term-0-0": true, "pane-0-1|replacement": true, "pane-1-0|term-1-0": true, "pane-1-1|term-1-1": true}
	if !reflect.DeepEqual(visible, wantVisible) || !reflect.DeepEqual(live, wantLive) {
		t.Fatalf("visible = %v, live = %v", visible, live)
	}
	s.activeTab["space"] = "tab-1"
	visible, live = s.PaneKeys()
	wantVisible = map[string]bool{"pane-1-0|term-1-0": true, "pane-1-1|term-1-1": true}
	if !reflect.DeepEqual(visible, wantVisible) || !reflect.DeepEqual(live, wantLive) {
		t.Fatalf("after tab switch: visible = %v, live = %v", visible, live)
	}
}
