package ui

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"rove/internal/herdr"
)

// controlHomeForTest is a short folder with a socket path inside the length
// a Unix socket allows.
func controlHomeForTest(t *testing.T) string {
	t.Helper()
	base := ""
	if runtime.GOOS == "darwin" {
		base = "/tmp" // macOS's default temporary path can exceed the socket limit.
	}
	home, err := os.MkdirTemp(base, "herdr-control")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("HERDR_APP_HOME", home)
	return home
}

// TestControlSocketAnswers runs the window's control socket without a window:
// one request line in, one reply line out, and the commands this window does
// not take say so instead of changing something.
func TestControlSocketAnswers(t *testing.T) {
	controlHomeForTest(t)
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	app.S = &herdr.Session{}
	app.ServeControl()

	call := func(t *testing.T, line string) map[string]any {
		t.Helper()
		conn, err := net.DialTimeout("unix", ControlSocket(), time.Second)
		if err != nil {
			t.Fatalf("no socket at %s: %v", ControlSocket(), err)
		}
		defer conn.Close()
		if _, err := conn.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
		reply, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			t.Fatalf("no reply line: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal(reply, &out); err != nil {
			t.Fatalf("reply is not one JSON line: %q: %v", reply, err)
		}
		return out
	}
	failed := func(t *testing.T, reply map[string]any, want string) {
		t.Helper()
		got, _ := reply["error"].(string)
		if got == "" {
			t.Fatalf("reply %v has no error", reply)
		}
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want it to name %q", got, want)
		}
	}

	t.Run("bad json", func(t *testing.T) {
		failed(t, call(t, "not json"), "bad request")
	})
	t.Run("a command it does not take", func(t *testing.T) {
		failed(t, call(t, `{"do":"pane.split","args":{"direction":"right"}}`), "tab.new")
	})
	t.Run("an argument it does not have", func(t *testing.T) {
		failed(t, call(t, `{"do":"tab.new","args":{"bogus":1}}`), "no argument")
	})
	t.Run("an argument of the wrong kind", func(t *testing.T) {
		failed(t, call(t, `{"do":"tab.new","args":{"name":7}}`), "name takes a string")
	})
	t.Run("a keep with no terminal", func(t *testing.T) {
		failed(t, call(t, `{"do":"terminal.keep","args":{}}`), "needs terminal")
	})
	t.Run("a keep of a terminal the core does not have", func(t *testing.T) {
		failed(t, call(t, `{"do":"terminal.keep","args":{"terminal":"t99"}}`), "no running terminal")
	})
}

// TestControlSocketStaysWithOneWindow leaves a live socket where another
// window would look: the second window must not take the commands from it.
func TestControlSocketStaysWithOneWindow(t *testing.T) {
	controlHomeForTest(t)
	path := ControlSocket()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	app.S = &herdr.Session{}
	app.ServeControl()

	// The socket still belongs to the listener that was there first.
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("the first window's socket went away: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := bufio.NewReader(conn).ReadBytes('\n'); err == nil {
		t.Fatal("the second window answered on the first window's socket")
	}
}
