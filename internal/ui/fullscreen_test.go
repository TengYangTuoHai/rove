package ui

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/core"
	"rove/internal/herdr"
)

// The fixture speaks the real core protocol in a child test process. It gives
// each attachment a first screen at exactly the grid requested by the app.
func TestMain(m *testing.M) {
	if os.Getenv("FULLSCREEN_TEST_CORE") == "1" && len(os.Args) == 2 && os.Args[1] == "connect" {
		runFullscreenCore()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runFullscreenCore() {
	in := bufio.NewReader(os.Stdin)
	_, _ = in.ReadString('\n')
	fmt.Fprintln(os.Stdout, `{"jsonrpc":"2.0","id":0,"result":{}}`)
	sendFrame := func(channel uint32, kind byte, payload []byte) {
		header := make([]byte, 9)
		binary.LittleEndian.PutUint32(header, channel)
		header[4] = kind
		binary.LittleEndian.PutUint32(header[5:], uint32(len(payload)))
		_, _ = os.Stdout.Write(append(header, payload...))
	}
	send := func(channel uint32, payload []byte) { sendFrame(channel, core.FrameData, payload) }
	leaf := func(id string) *herdr.Node {
		return &herdr.Node{ID: id, Type: "block", Content: &herdr.NodeContent{Terminal: "t" + id}}
	}
	doc := map[string]any{"spaces": []any{map[string]any{"id": "space", "name": "Layout test", "tabs": []any{
		map[string]any{"id": "split", "name": "Split", "root": &herdr.Node{ID: "split", Type: "row", Children: []*herdr.Node{leaf("left"), leaf("right")}}},
		map[string]any{"id": "full", "name": "Full", "root": &herdr.Node{ID: "full", Type: "row", Children: []*herdr.Node{leaf("full")}}},
	}}}}
	spaces := doc["spaces"].([]any)
	space := spaces[0].(map[string]any)
	tabs := space["tabs"].([]any)
	for i := range maxHiddenTerminals + 2 {
		id := fmt.Sprintf("cached-%d", i)
		tabs = append(tabs, map[string]any{"id": id, "root": leaf(id)})
	}
	tabs = append(tabs, map[string]any{"id": "broken", "root": leaf("broken")})
	space["tabs"] = tabs
	if axis := os.Getenv("PANE_DRAG_TEST_AXIS"); axis != "" {
		tabs[0].(map[string]any)["root"].(*herdr.Node).Type = axis
	}
	channel := uint32(10)
	endedTestStream := false
	docVersion := 1
	sidebarDocs := newSidebarDocumentCore()
	for {
		header := make([]byte, 9)
		if _, err := io.ReadFull(in, header); err != nil {
			os.Exit(0)
		}
		payload := make([]byte, binary.LittleEndian.Uint32(header[5:]))
		if _, err := io.ReadFull(in, payload); err != nil {
			os.Exit(0)
		}
		if binary.LittleEndian.Uint32(header) != 0 {
			continue
		}
		var req struct {
			ID     uint64
			Method string
			Params struct {
				Cols, Rows int
				Terminal   string
				Value      json.RawMessage
			}
		}
		_ = json.Unmarshal(payload, &req)
		if sidebarDocs != nil {
			sidebarDocs.respond(payload, send)
			continue
		}
		if req.Method == "terminal.attach" && req.Params.Terminal == "tbroken" {
			reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32000, "message": "terminal unavailable"}})
			send(0, reply)
			continue
		}
		var result any = map[string]any{}
		switch req.Method {
		case "doc.get":
			result = map[string]any{"version": docVersion, "value": doc}
		case "doc.set":
			if os.Getenv("PANE_DRAG_TEST_ECHO") == "before-ack" {
				send(1, []byte("{\"method\":\"event\",\"params\":{\"type\":\"doc\"}}\n"))
			}
			if gate := os.Getenv("PANE_DRAG_TEST_GATE"); gate != "" {
				conn, err := net.Dial("tcp", gate)
				if err != nil {
					os.Exit(1)
				}
				_, _ = conn.Write(append(req.Params.Value, '\n'))
				_, _ = bufio.NewReader(conn).ReadByte()
				_ = conn.Close()
			}
			if os.Getenv("PANE_FINISHED_TEST") == "1" || os.Getenv("PANE_DRAG_TEST_GATE") != "" {
				_ = json.Unmarshal(req.Params.Value, &doc)
				docVersion++
				result = map[string]any{"version": docVersion}
			}
		case "terminal.get":
			if os.Getenv("PANE_FINISHED_TEST") == "1" {
				result = &herdr.Terminal{Address: req.Params.Terminal, State: "finished"}
			}
		case "terminal.run":
			if os.Getenv("PANE_FINISHED_TEST") == "1" {
				result = map[string]any{"address": "tnew"}
			}
		case "terminal.list", "events.subscribe":
			terminals := []any{}
			if os.Getenv("PANE_FINISHED_TEST") == "1" {
				terminals = append(terminals, &herdr.Terminal{Address: "tbroken", State: "finished"})
			}
			if os.Getenv("TAB_CLOSE_TEST_BUSY") == "1" {
				terminals = append(terminals, &herdr.Terminal{Address: "tfull", State: "live", Argv: []string{"sleep", "60"}})
			}
			result = map[string]any{"channel": 1, "terminals": terminals}
		case "terminal.attach":
			channel++
			result = map[string]any{"channel": channel}
		}
		reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		send(0, reply)
		if req.Method == "doc.set" && os.Getenv("PANE_DRAG_TEST_ECHO") == "after-ack" {
			send(1, []byte("{\"method\":\"event\",\"params\":{\"type\":\"doc\"}}\n"))
		}
		if req.Method == "terminal.attach" {
			var screen strings.Builder
			screen.WriteString("\x1b[?1049h\x1b[?25l\x1b[48;2;45;70;90m\x1b[2J")
			for row := 1; row <= req.Params.Rows; row++ {
				fmt.Fprintf(&screen, "\x1b[%d;1H|%s|", row, strings.Repeat(".", max(req.Params.Cols-2, 0)))
			}
			fmt.Fprintf(&screen, "\x1b[1;3HFIRST %dx%d\x1b[%d;%dHEND", req.Params.Cols, req.Params.Rows, req.Params.Rows, req.Params.Cols-3)
			send(channel, []byte(screen.String()))
			if req.Params.Terminal == "tleft" && !endedTestStream {
				switch os.Getenv("STREAM_END_TEST") {
				case "disconnect":
					os.Exit(0)
				case "signaled", "exited":
					endedTestStream = true
					reason := os.Getenv("STREAM_END_TEST")
					payload, _ := json.Marshal(map[string]any{"reason": reason})
					sendFrame(channel, core.FrameClose, payload)
				}
			}
		}
	}
}

func fullscreenSession(t *testing.T) *herdr.Session {
	t.Helper()
	t.Setenv("ROVE_HOME", t.TempDir())
	t.Setenv("FULLSCREEN_TEST_CORE", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s, err := herdr.Connect(exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Core.Close)
	return s
}

func TestSwitchSplitToFullRetainsMeasuredTerminal(t *testing.T) {
	RegisterFonts()
	for _, tc := range []struct {
		name        string
		scale, zoom float32
	}{{"standard", 1, 1}, {"retina", 2, 1}, {"zoom", 2, 1.25}} {
		t.Run(tc.name, func(t *testing.T) {
			s := fullscreenSession(t)
			app := NewApp(s, ThemeByName("graphite"))
			t.Cleanup(func() {
				app.mu.Lock()
				defer app.mu.Unlock()
				for key, entry := range app.terms {
					delete(app.terms, key)
					_ = entry.term.Close()
				}
			})
			s.SelectTab("split")
			tt := myui.NewTester(func(c *myui.Context) {
				c.SetZoom(tc.zoom)
				app.collect()
				myui.Box(c).Fill().Children(func() { app.paneArea(c) })
			}, 1000, 600)
			tt.SetScale(tc.scale)
			wait := func(key string) *paneTerm {
				t.Helper()
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					tt.Frame()
					app.mu.Lock()
					entry := app.terms[key]
					app.mu.Unlock()
					if entry != nil && strings.Contains(entry.term.Text(), "FIRST") {
						return entry
					}
					time.Sleep(time.Millisecond)
				}
				t.Fatalf("no first screen for %s", key)
				return nil
			}
			left := wait("left|tleft")
			right := wait("right|tright")
			if dir := os.Getenv("FULLSCREEN_SHOT_DIR"); dir != "" {
				writeFullscreenShot(t, tt, filepath.Join(dir, tc.name+"-split.png"))
			}
			s.SelectTab("full")
			full := wait("full|tfull")
			text := full.term.Text()
			lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
			if !strings.HasSuffix(lines[len(lines)-1], "END|") || !strings.Contains(lines[0], fmt.Sprintf("FIRST %dx%d", len(lines[0]), len(lines))) {
				t.Fatalf("first screen wrapped before the full-width layout: %q", text)
			}
			if len(lines[0]) <= 90 {
				t.Fatalf("full-width grid is too narrow: %d columns", len(lines[0]))
			}
			if dir := os.Getenv("FULLSCREEN_SHOT_DIR"); dir != "" {
				tt.Frame()
				writeFullscreenShot(t, tt, filepath.Join(dir, tc.name+"-first.png"))
			}
			for range 3 {
				s.SelectTab("split")
				tt.Frame()
				if wait("left|tleft") != left || wait("right|tright") != right {
					t.Fatal("switching tabs recreated the split terminals")
				}
				s.SelectTab("full")
				tt.Frame()
				if wait("full|tfull") != full {
					t.Fatal("switching tabs recreated the full-width terminal")
				}
			}
			tt.Frame()
			img := tt.Image()
			r, g, b, _ := img.At(int(950*tc.scale), int(550*tc.scale)).RGBA()
			if r>>8 != 45 || g>>8 != 70 || b>>8 != 90 {
				t.Fatal("terminal's first screen does not fill the lower right")
			}
			if dir := os.Getenv("FULLSCREEN_SHOT_DIR"); dir != "" {
				writeFullscreenShot(t, tt, filepath.Join(dir, tc.name+".png"))
			}
			// A pane removed from the document releases its cached terminal.
			s.CloseTab("split")
			app.collect()
			if app.terms["left|tleft"] != nil || app.terms["right|tright"] != nil {
				t.Fatal("removed panes kept their terminal views")
			}
			t.Logf("full-width first screen: %d×%d; retained both split views and full view across 3 switches", len(lines[0]), len(lines))
		})
	}
}

func writeFullscreenShot(t *testing.T, tt *myui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestHiddenTerminalCacheIsBounded(t *testing.T) {
	s := fullscreenSession(t)
	s.SelectTab("split")
	app := NewApp(s, ThemeByName("graphite"))
	for _, tab := range s.Places()[0].Tabs {
		for _, p := range s.Panes(tab.ID) {
			term, err := terminal.New(terminal.Options{Conn: newPeekConn()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = term.Close() })
			key := p.Pane + "|" + p.Terminal
			app.termUsed++
			app.terms[key] = &paneTerm{key: key, pane: p.Pane, term: term, lastUsed: app.termUsed}
		}
	}
	app.collect()
	if len(app.terms) != maxHiddenTerminals+2 {
		t.Fatalf("cache has %d entries, want %d hidden plus 2 visible", len(app.terms), maxHiddenTerminals)
	}
	if app.terms["left|tleft"] == nil || app.terms["right|tright"] == nil {
		t.Fatal("cache evicted a visible terminal")
	}
	if app.terms["full|tfull"] != nil || app.terms["cached-0|tcached-0"] != nil {
		t.Fatal("cache did not evict the least recently viewed terminals")
	}
}

func TestMeasuredPaneAttachFailureKeepsPane(t *testing.T) {
	RegisterFonts()
	s := fullscreenSession(t)
	s.SelectTab("broken")
	app := NewApp(s, ThemeByName("graphite"))
	tt := myui.NewTester(func(c *myui.Context) { app.paneArea(c) }, 1000, 600)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		tt.Frame()
		app.mu.Lock()
		failed := app.failedTerms["broken|tbroken"]
		app.mu.Unlock()
		if failed && len(toastSnapshot(app)) != 0 {
			for range 3 {
				tt.Frame()
			}
			if len(s.Panes("broken")) != 1 {
				t.Fatal("attachment failure closed the pane")
			}
			app.mu.Lock()
			defer app.mu.Unlock()
			if app.terms["broken|tbroken"] != nil || app.starting["broken|tbroken"] || len(app.notes) != 1 {
				t.Fatal("failed attachment retained its view or retried on repaint")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("attachment failure was not reported")
}
