package herdr

import "testing"

func TestReplaceMachinePrunesOnlyRemovedTabViews(t *testing.T) {
	s := performanceSession(2)
	kept := s.tabs["tab-0"]
	kept.focused = "pane-0-1"
	other := "remote"
	remoteView := &tabView{focused: "remote-pane"}
	s.spaces = append(s.spaces, &Space{ID: "remote-space", Machine: &other, Tabs: []*Tab{{ID: "remote-tab", Root: &Node{ID: "remote-pane", Type: "block"}}}})
	s.tabs["remote-tab"] = remoteView
	s.replaceMachine("", []*Space{{ID: "space", Tabs: []*Tab{s.spaces[0].Tabs[0]}}})
	if s.tabs["tab-0"] != kept || s.tabs["tab-0"].focused != "pane-0-1" {
		t.Fatal("retained tab lost its view")
	}
	if _, ok := s.tabs["tab-1"]; ok {
		t.Fatal("removed tab still has a view")
	}
	if s.tabs["remote-tab"] != remoteView {
		t.Fatal("another machine's view was removed")
	}
}
