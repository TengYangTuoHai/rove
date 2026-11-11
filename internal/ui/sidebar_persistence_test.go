package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// sidebarDocumentCore keeps documents across child core processes, so a new
// Session must read the saved order rather than a previous Session's model.
type sidebarDocumentCore struct {
	path string
	docs map[string]struct {
		Version uint64          `json:"version"`
		Value   json.RawMessage `json:"value"`
	}
}

func newSidebarDocumentCore() *sidebarDocumentCore {
	path := os.Getenv("SIDEBAR_PERSIST_TEST_DOCUMENT")
	if path == "" {
		return nil
	}
	f := &sidebarDocumentCore{path: path}
	text, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(text, &f.docs); err != nil {
		panic(err)
	}
	return f
}

func (f *sidebarDocumentCore) respond(payload []byte, send func(uint32, []byte)) {
	var req struct {
		ID     uint64
		Method string
		Params struct {
			On      string
			Version uint64
			Value   json.RawMessage
		}
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		panic(err)
	}
	var result any = map[string]any{}
	var rpcErr any
	switch req.Method {
	case "doc.get":
		result = f.docs[req.Params.On]
	case "doc.set":
		d := f.docs[req.Params.On]
		if d.Version != req.Params.Version {
			rpcErr = map[string]any{"code": -32000, "message": "document conflict", "data": map[string]any{"kind": "conflict"}}
			break
		}
		d.Version++
		d.Value = req.Params.Value
		f.docs[req.Params.On] = d
		text, err := json.Marshal(f.docs)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(f.path, text, 0600); err != nil {
			panic(err)
		}
		// The real core publishes the document event before its write reply.
		event, _ := json.Marshal(map[string]any{"method": "event", "params": map[string]any{"type": "doc", "machine": req.Params.On}})
		send(1, append(event, '\n'))
		result = map[string]any{"version": d.Version}
	case "machine.list":
		var machines []*herdr.Machine
		for name := range f.docs {
			if name != "" {
				machines = append(machines, &herdr.Machine{Name: name, State: "connected"})
			}
		}
		result = map[string]any{"machines": machines}
	case "terminal.list", "events.subscribe":
		result = map[string]any{"channel": 1, "terminals": []any{}}
	}
	reply := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rpcErr != nil {
		reply["error"] = rpcErr
	} else {
		reply["result"] = result
	}
	text, _ := json.Marshal(reply)
	send(0, text)
}

func TestSidebarDragPersistsAcrossCoreRestart(t *testing.T) {
	RegisterFonts()
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%t", remote), func(t *testing.T) {
			t.Setenv("ROVE_HOME", t.TempDir())
			t.Setenv("FULLSCREEN_TEST_CORE", "1")
			path := filepath.Join(t.TempDir(), "documents.json")
			t.Setenv("SIDEBAR_PERSIST_TEST_DOCUMENT", path)
			docs := map[string]any{"": map[string]any{"version": 1, "value": map[string]any{"spaces": dragSpaces("alpha", "bravo", "charlie")}}}
			prefix := ""
			if remote {
				docs["remote"] = docs[""]
				docs[""] = map[string]any{"version": 1, "value": map[string]any{"spaces": dragSpaces("local")}}
				prefix = "remote/"
			}
			text, err := json.Marshal(docs)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, text, 0600); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			s, err := herdr.Connect(exe, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Core.Close)
			app := NewApp(s, ThemeByName("graphite"))
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Column(c).Width(268).Height(350).Children(func() { app.spaces(c) })
			}, 360, 400)
			tt.Frame()
			order := []string{"alpha", "bravo", "charlie"}
			// Consecutive drops exercise both directions, every source slot, and
			// document echoes arriving alongside writes for another machine.
			for _, move := range []struct {
				id string
				to int
			}{{"alpha", 3}, {"alpha", 1}, {"alpha", 2}, {"alpha", 3}, {"alpha", 2}, {"alpha", 1}} {
				tt.Advance(600 * time.Millisecond)
				from := slices.Index(order, move.id)
				source, ok := tt.Find(move.id)
				if !ok {
					t.Fatalf("source %q not drawn: %v", move.id, tt.Texts())
				}
				target, ok := tt.Find(order[move.to-1])
				if !ok {
					t.Fatal("destination not drawn")
				}
				y := target.Y - 2
				if move.to-1 > from {
					y = target.Y + target.H + 2
				}
				tt.Press(source.X+4, source.Y+source.H/2)
				tt.Move(100, y)
				tt.Release(100, y)
				tt.Frame()
				order = slices.Delete(order, from, from+1)
				order = slices.Insert(order, move.to-1, move.id)
				want := make([]string, 0, len(order)+1)
				if remote {
					want = append(want, "local")
				}
				for _, id := range order {
					want = append(want, prefix+id)
				}
				check := func(session *herdr.Session, stage string) {
					t.Helper()
					var got []string
					for _, sp := range session.Places() {
						got = append(got, sp.ID)
					}
					if !slices.Equal(got, want) {
						t.Fatalf("%s after moving %s to %d: order %v, want %v", stage, move.id, move.to, got, want)
					}
				}
				check(s, "live session")
				params := map[string]any{"name": "herdr-app"}
				if remote {
					params["on"] = "remote"
				}
				raw, err := s.Core.Call(context.Background(), "doc.get", params)
				if err != nil {
					t.Fatal(err)
				}
				var saved struct {
					Value struct {
						Spaces []*herdr.Space
					}
				}
				if err := json.Unmarshal(raw, &saved); err != nil {
					t.Fatal(err)
				}
				var storedIDs []string
				for _, sp := range saved.Value.Spaces {
					storedIDs = append(storedIDs, sp.ID)
				}
				if !slices.Equal(storedIDs, order) {
					t.Fatalf("core document order = %v, want %v", storedIDs, order)
				}
				func() {
					next, err := herdr.Connect(exe, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer next.Core.Close()
					check(next, "restarted core")
				}()
			}
		})
	}
}
