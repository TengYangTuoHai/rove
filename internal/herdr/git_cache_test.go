package herdr

import "testing"

func TestPruneGitKeepsCurrentMachineAndDirectory(t *testing.T) {
	s := performanceSession(1)
	s.spaces[0].Folder = "/repo"
	s.terminals["term-0-0"].Cwd = "/repo/sub"
	s.terminals["term-0-1"].Foreground = &Foreground{Cwd: "/other"}
	s.git = map[string]gitInfo{
		gitKey("", "/repo/sub"): {}, gitKey("", "/other"): {},
		gitKey("", "/old"): {}, gitKey("remote", "/other"): {},
	}
	s.pruneGit()
	if len(s.git) != 2 {
		t.Fatalf("cache after pruning: %v", s.git)
	}
	if _, ok := s.git[gitKey("", "/repo/sub")]; !ok {
		t.Fatal("terminal directory was removed")
	}
	if _, ok := s.git[gitKey("", "/other")]; !ok {
		t.Fatal("foreground directory was removed")
	}
}
