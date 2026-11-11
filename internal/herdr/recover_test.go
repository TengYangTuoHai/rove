package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"rove/internal/core"
)

type recoveryFake struct {
	doc              json.RawMessage
	state            string
	onGet            func(int) string
	onRun            func()
	setErr           error
	runErr           error
	confirmCommit    bool
	docGets          int
	gets, runs, sets int
	killed           []string
	cwd, machine     string
	labels           map[string]string
	setValue         json.RawMessage
}

func (f *recoveryFake) ops() recoverOps {
	return recoverOps{
		call: func(_ context.Context, method string, p any) (json.RawMessage, error) {
			switch method {
			case "doc.get":
				f.docGets++
				if f.confirmCommit && f.docGets > 1 {
					return json.Marshal(map[string]any{"version": 2, "value": f.setValue})
				}
				return append(json.RawMessage(nil), f.doc...), nil
			case "terminal.get":
				f.gets++
				state := f.state
				if f.onGet != nil {
					state = f.onGet(f.gets)
				}
				return json.Marshal(map[string]any{"address": p.(map[string]any)["terminal"], "state": state, "cwd": "/last", "foreground": map[string]any{"cwd": "/foreground"}})
			case "doc.set":
				f.sets++
				f.setValue = append(json.RawMessage(nil), p.(map[string]any)["value"].(json.RawMessage)...)
				if f.setErr != nil {
					return nil, f.setErr
				}
				return json.RawMessage(`{"version":2}`), nil
			default:
				return nil, fmt.Errorf("unexpected call %s", method)
			}
		},
		run: func(argv []string, cwd string, labels map[string]string, on string) (string, error) {
			f.runs++
			f.cwd, f.machine, f.labels = cwd, on, labels
			if len(argv) != 0 {
				return "", errors.New("expected shell")
			}
			if f.onRun != nil {
				f.onRun()
			}
			if f.runErr != nil {
				return "", f.runErr
			}
			return qualify("t2", machineTag(on)), nil
		},
		kill: func(addr string) { f.killed = append(f.killed, addr) },
	}
}

func recoverySession(t *testing.T, machine string) (*Session, *recoveryFake, string, string) {
	t.Helper()
	t.Setenv("ROVE_HOME", t.TempDir())
	tag := machineTag(machine)
	pane, old := qualify("p1", tag), qualify("t1", tag)
	root := &Node{ID: qualify("tab", tag), Type: "row", Children: []*Node{{ID: pane, Type: "block", Size: closeWeight(.3), Content: &NodeContent{Terminal: old}}}}
	s := &Session{spaces: []*Space{{ID: qualify("space", tag), Folder: "/fallback", Machine: tag, Tabs: []*Tab{{ID: qualify("tab", tag), Root: root}}}},
		activeTab: map[string]string{}, tabs: map[string]*tabView{}, query: map[string]string{}, terminals: map[string]*Terminal{}, git: map[string]gitInfo{}, gitBusy: map[string]bool{}, versions: map[string]uint64{machine: 1}}
	f := &recoveryFake{state: "finished", doc: json.RawMessage(`{"version":1,"value":{"custom":{"keep":9007199254740993},"spaces":[{"id":"space","extra":true,"tabs":[{"id":"tab","root":{"id":"tab","type":"row","children":[{"id":"p1","type":"block","size":{"weight":0.3},"content":{"terminal":"t1","other":"keep"},"hidden":true}]}}]}]}}`)}
	return s, f, pane, old
}

func TestRecoverPanePersistsOnlyReference(t *testing.T) {
	for _, machine := range []string{"", "host"} {
		t.Run("machine="+machine, func(t *testing.T) {
			s, f, pane, old := recoverySession(t, machine)
			addr, err := s.recoverPane(context.Background(), pane, old, f.ops())
			if err != nil || addr != qualify("t2", machineTag(machine)) {
				t.Fatalf("address %q: %v", addr, err)
			}
			if f.runs != 1 || f.gets != 2 || f.sets != 1 || len(f.killed) != 0 || f.cwd != "/foreground" || f.machine != machine {
				t.Fatalf("unexpected calls: %+v", f)
			}
			if f.labels["herdr.space"] != qualify("space", machineTag(machine)) || f.labels["herdr.tab"] != qualify("tab", machineTag(machine)) {
				t.Fatalf("labels: %v", f.labels)
			}
			if s.findPaneLocked(pane).terminal() != addr || s.versions[machine] != 2 {
				t.Fatal("model not updated")
			}
			for _, part := range []string{`"terminal":"t2"`, `"hidden":true`, `"other":"keep"`, `9007199254740993`, `"weight":0.3`} {
				if !strings.Contains(string(f.setValue), part) {
					t.Fatalf("document lost %s: %s", part, f.setValue)
				}
			}
			// A delayed read started before the commit must not roll the model back.
			s.applyDoc(machine, f.doc)
			if s.findPaneLocked(pane).terminal() != addr {
				t.Fatal("stale document rolled back recovery")
			}
		})
	}
}

func TestRecoverPaneRequiresCurrentFinishedState(t *testing.T) {
	for _, state := range []string{"live", "", "lost"} {
		t.Run(state, func(t *testing.T) {
			s, f, pane, old := recoverySession(t, "")
			f.state = state
			if _, err := s.recoverPane(context.Background(), pane, old, f.ops()); err == nil || f.runs != 0 || f.sets != 0 {
				t.Fatalf("state %q: err=%v fake=%+v", state, err, f)
			}
		})
	}
	s, f, pane, old := recoverySession(t, "")
	f.onGet = func(n int) string {
		if n == 1 {
			return "finished"
		}
		return "live"
	}
	if _, err := s.recoverPane(context.Background(), pane, old, f.ops()); err == nil || f.sets != 0 || len(f.killed) != 1 || f.killed[0] != "t2" {
		t.Fatalf("restart: err=%v fake=%+v", err, f)
	}
}

func TestRecoverPaneConcurrentChangesAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		change     func(*Session, *recoveryFake, string)
		wantKilled bool
		uncertain  bool
	}{
		{"deleted locally", func(s *Session, f *recoveryFake, pane string) {
			f.onRun = func() { s.mu.Lock(); s.spaces = nil; s.mu.Unlock() }
		}, true, false},
		{"replaced locally", func(s *Session, f *recoveryFake, pane string) {
			f.onRun = func() { s.mu.Lock(); s.findPaneLocked(pane).Content.Terminal = "other"; s.mu.Unlock() }
		}, true, false},
		{"document conflict", func(s *Session, f *recoveryFake, pane string) {
			f.setErr = &core.Error{Message: "conflict", Data: json.RawMessage(`{"kind":"conflict"}`)}
		}, true, false},
		{"connection lost during commit", func(s *Session, f *recoveryFake, pane string) { f.setErr = errors.New("connection lost") }, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, pane, old := recoverySession(t, "")
			tc.change(s, f, pane)
			addr, err := s.recoverPane(context.Background(), pane, old, f.ops())
			if err == nil || (len(f.killed) != 0) != tc.wantKilled || (addr == "t2") != tc.uncertain {
				t.Fatalf("addr=%q err=%v fake=%+v", addr, err, f)
			}
		})
	}
}

func TestRecoverPaneUnknownOutcomeBlocksRetry(t *testing.T) {
	for _, at := range []string{"run", "set", "remote unavailable"} {
		t.Run(at, func(t *testing.T) {
			s, f, pane, old := recoverySession(t, "")
			switch at {
			case "run":
				f.runErr = errors.New("disconnected")
			case "set":
				f.setErr = errors.New("disconnected")
			case "remote unavailable":
				f.setErr = &core.Error{Message: "unavailable", Data: json.RawMessage(`{"kind":"unavailable"}`)}
			}
			_, err := s.recoverPane(context.Background(), pane, old, f.ops())
			if err == nil {
				t.Fatal("expected unknown outcome")
			}
			_, err = s.recoverPane(context.Background(), pane, old, f.ops())
			if err == nil || f.runs != 1 || len(f.killed) != 0 {
				t.Fatalf("retry: %v, fake=%+v", err, f)
			}
		})
	}
}

func TestRecoverPaneConfirmsLostWriteReply(t *testing.T) {
	s, f, pane, old := recoverySession(t, "")
	f.setErr, f.confirmCommit = errors.New("lost reply"), true
	addr, err := s.recoverPane(context.Background(), pane, old, f.ops())
	if err != nil || addr != "t2" || s.findPaneLocked(pane).terminal() != "t2" || len(f.killed) != 0 {
		t.Fatalf("addr=%q err=%v fake=%+v", addr, err, f)
	}
}

func TestRecoverPaneConcurrentRequests(t *testing.T) {
	s, f, pane, old := recoverySession(t, "")
	started, resume := make(chan struct{}), make(chan struct{})
	f.onRun = func() { close(started); <-resume }
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := s.recoverPane(context.Background(), pane, old, f.ops())
		results <- err
	}()
	<-started
	go func() {
		defer wg.Done()
		_, err := s.recoverPane(context.Background(), pane, old, f.ops())
		results <- err
	}()
	close(resume)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 || f.runs != 1 || f.sets != 1 {
		t.Fatalf("success=%d fake=%+v", success, f)
	}
}

func TestRecoverPaneRejectsOutdatedDocumentAndOverride(t *testing.T) {
	for _, override := range []bool{false, true} {
		s, f, pane, old := recoverySession(t, "")
		if override {
			s.query[pane] = "some-other-terminal"
		} else {
			f.doc = json.RawMessage(`{"version":1,"value":{"spaces":[]}}`)
		}
		if _, err := s.recoverPane(context.Background(), pane, old, f.ops()); err == nil || f.runs != 0 {
			t.Fatalf("override=%v err=%v", override, err)
		}
	}
}
