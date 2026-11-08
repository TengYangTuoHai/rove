package herdr

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGitRefreshOnlyNotifiesOnChanges(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "initial")
	changes := 0
	s := &Session{git: map[string]gitInfo{}, gitBusy: map[string]bool{}, savedAt: time.Now().Add(time.Hour), OnChange: func() { changes++ }}
	s.readGit("", dir)
	first := s.git[gitKey("", dir)]
	if first.Branch == "" || first.Repo != filepath.Base(dir) || changes != 1 {
		t.Fatalf("first refresh: %+v, changes %d", first, changes)
	}
	s.readGit("", dir)
	if changes != 1 {
		t.Fatalf("unchanged refresh notified %d times", changes)
	}
	run("checkout", "-q", "-b", "performance-test")
	s.readGit("", dir)
	if got := s.git[gitKey("", dir)].Branch; got != "performance-test" || changes != 2 {
		t.Fatalf("branch %q, changes %d", got, changes)
	}
}
