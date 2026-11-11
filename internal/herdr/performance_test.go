package herdr

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func performanceSession(tabs int) *Session {
	s := &Session{active: "space", activeTab: map[string]string{"space": "tab-0"}, tabs: map[string]*tabView{}, terminals: map[string]*Terminal{}, query: map[string]string{}}
	sp := &Space{ID: "space"}
	for i := 0; i < tabs; i++ {
		id := fmt.Sprintf("tab-%d", i)
		root := &Node{ID: id, Type: "row"}
		for j := 0; j < 2; j++ {
			pane := fmt.Sprintf("pane-%d-%d", i, j)
			addr := fmt.Sprintf("term-%d-%d", i, j)
			root.Children = append(root.Children, &Node{ID: pane, Type: "block", Content: &NodeContent{Terminal: addr}})
			s.terminals[addr] = &Terminal{Address: addr, Argv: []string{"bash"}}
		}
		sp.Tabs = append(sp.Tabs, &Tab{ID: id, Root: root})
		s.tabs[id] = &tabView{}
	}
	s.spaces = []*Space{sp}
	return s
}

// BenchmarkIdleTerminalSnapshot models one unchanged periodic terminal poll.
func BenchmarkIdleTerminalSnapshot(b *testing.B) {
	s := performanceSession(100)
	terms := make([]*Terminal, 0, len(s.terminals))
	for _, term := range s.terminals {
		terms = append(terms, term)
	}
	raw, err := json.Marshal(struct {
		Terminals []*Terminal `json:"terminals"`
	}{terms})
	if err != nil {
		b.Fatal(err)
	}
	s.savedAt = time.Now().Add(time.Hour)
	s.applySnapshot(raw)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.applySnapshot(raw) {
			s.changed()
		} else {
			s.refreshGit()
		}
		s.pruneGit()
	}
}

// BenchmarkGitRefresh runs the real local git queries against this repository.
func BenchmarkGitRefresh(b *testing.B) {
	dir, err := filepath.Abs("../..")
	if err != nil {
		b.Fatal(err)
	}
	s := &Session{git: map[string]gitInfo{}, gitBusy: map[string]bool{}, savedAt: time.Now().Add(time.Hour)}
	s.readGit("", dir)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.readGit("", dir)
	}
}

// BenchmarkStaleGitCache tracks cache entries after model maintenance.
func BenchmarkStaleGitCache(b *testing.B) {
	var retained int
	for i := 0; i < b.N; i++ {
		s := &Session{git: make(map[string]gitInfo)}
		for j := 0; j < 1000; j++ {
			s.git[fmt.Sprintf("path-%d", j)] = gitInfo{}
		}
		s.reindex()
		s.pruneGit()
		retained = len(s.git)
	}
	b.ReportMetric(float64(retained), "retained-entries/op")
}

// BenchmarkReplacedTabs measures repeated external document replacement.
func BenchmarkReplacedTabs(b *testing.B) {
	var retained int
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := performanceSession(1)
		for j := 0; j < 1000; j++ {
			id := fmt.Sprintf("replacement-%d", j)
			tab := &Tab{ID: id, Root: &Node{ID: id, Type: "block"}}
			s.replaceMachine("", []*Space{{ID: "space", Tabs: []*Tab{tab}}})
			s.ensureFocusLocked()
		}
		retained = len(s.tabs)
	}
	b.ReportMetric(float64(retained), "retained-tabs/op")
}

// BenchmarkPaneWeights measures the model work on each divider movement while
// keeping the number of visible panes constant as background tabs accumulate.
func BenchmarkPaneWeights(b *testing.B) {
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("%d-tabs", count), func(b *testing.B) {
			s := performanceSession(count)
			s.savedAt = time.Now().Add(time.Hour)
			weights := map[string]float64{"pane-0-0": 0.4, "pane-0-1": 0.6}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.SetWeights("tab-0", weights)
			}
		})
	}
}

// BenchmarkPaneKeys measures the model traversal used to collect terminal views.
func BenchmarkPaneKeys(b *testing.B) {
	for _, count := range []int{10, 100} {
		b.Run(fmt.Sprintf("%d-tabs", count), func(b *testing.B) {
			s := performanceSession(count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = s.PaneKeys()
			}
		})
	}
}
