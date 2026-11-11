package herdr

import "testing"

func TestClosePaneIfTerminalDoesNotCloseChangedOrOverriddenPane(t *testing.T) {
	root := &Node{ID: "tab", Type: "row", Children: []*Node{{ID: "p", Type: "block", Content: &NodeContent{Terminal: "new"}}}}
	s := closeLayoutSession(t, root, "p")
	s.query = map[string]string{}
	if s.ClosePaneIfTerminal("p", "old") || s.findPaneLocked("p") == nil {
		t.Fatal("stale terminal closed its replacement")
	}
	s.query["p"] = "shown"
	if s.ClosePaneIfTerminal("p", "new") || s.ClosePaneIfTerminal("p", "shown") || s.findPaneLocked("p") == nil {
		t.Fatal("temporary terminal override closed its pane")
	}
	delete(s.query, "p")
	if !s.ClosePaneIfTerminal("p", "new") || len(s.spaces) != 0 || len(s.tabs) != 0 {
		t.Fatal("matched last pane did not close its tab and space")
	}
	if s.ClosePaneIfTerminal("p", "new") {
		t.Fatal("closed a missing pane twice")
	}
}

func TestClosePaneIfTerminalPreservesOtherPanes(t *testing.T) {
	root := &Node{ID: "tab", Type: "row", Children: []*Node{
		{ID: "p", Type: "block", Content: &NodeContent{Terminal: "old"}},
		{ID: "other", Type: "block", Content: &NodeContent{Terminal: "other-terminal"}},
	}}
	s := closeLayoutSession(t, root, "p")
	if !s.ClosePaneIfTerminal("p", "old") || root.find("p") != nil || root.find("other") == nil {
		t.Fatal("closing matched pane changed its neighbor")
	}
}
