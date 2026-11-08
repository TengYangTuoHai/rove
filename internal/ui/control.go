package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"rove/internal/herdr"
)

// ControlHome is the folder `herdr-app do` looks in for the window's socket:
// $HERDR_APP_HOME, else ~/.config/herdr-app, reading the same two as the
// herdr-app command does.
func ControlHome() string {
	if h := os.Getenv("HERDR_APP_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "herdr-app")
}

// ControlSocket is the file the window answers on. It is herdr-app's name in
// herdr-app's folder, because that is where the commands agents already run
// look: `herdr-app do` and `herdr-app ls` reach this window without a setting.
func ControlSocket() string {
	dir := ControlHome()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "herdr-app.sock")
}

// controlCommands are the commands this window answers from outside; the
// window's keys reach the rest, and `herdr-app do` says so when it asks for
// one of those instead.
var controlCommands = []string{"tab.new", "terminal.keep"}

// ServeControl answers `herdr-app do <command>` on the control socket, one
// JSON line each way, as Herdr's window does. It is what puts a worker the
// core runs on the screen: `tab.new` starts a tab, `terminal.keep` gives a
// terminal that is already running a pane.
func (a *App) ServeControl() {
	path := ControlSocket()
	if path == "" {
		return
	}
	// A live socket means another window owns this home: it keeps the
	// commands, as Herdr's own window does, and this one says so once.
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		_ = conn.Close()
		log.Printf("control: another Herdr is serving %s; this window answers nothing there", path)
		return
	}
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Printf("control: cannot make room for %s: %v", path, err)
		return
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		log.Printf("control: cannot serve %s: %v", path, err)
		return
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go a.serveControl(conn)
		}
	}()
}

// serveControl reads one request line, answers it with one reply line and
// closes, as herdr-app does.
func (a *App) serveControl(conn net.Conn) {
	defer conn.Close()
	line, err := bufio.NewReader(io.LimitReader(conn, 1<<20)).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	reply := a.control(line)
	body, err := json.Marshal(reply)
	if err != nil {
		body = []byte(`{"error":"the window cannot answer"}`)
	}
	_, _ = conn.Write(append(body, '\n'))
}

// control runs one request and returns what to print for it. An error in the
// reply is how `herdr-app do` reports failure: it exits 1.
func (a *App) control(line []byte) map[string]any {
	var req struct {
		Do   string          `json:"do"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return controlError(fmt.Errorf("bad request: %v", err))
	}
	if req.Do == "" {
		return controlError(fmt.Errorf("this window answers { do }: %s", strings.Join(controlCommands, ", ")))
	}
	result, err := a.do(req.Do, req.Args)
	if err != nil {
		return controlError(err)
	}
	return result
}

func controlError(err error) map[string]any {
	return map[string]any{"error": err.Error()}
}

// do runs one of the window's commands with the arguments a request carries.
func (a *App) do(name string, raw json.RawMessage) (map[string]any, error) {
	switch name {
	case "tab.new":
		return a.controlTabNew(raw)
	case "terminal.keep":
		return a.controlTerminalKeep(raw)
	}
	return nil, fmt.Errorf("this window does not run %q from outside: %s", name, strings.Join(controlCommands, ", "))
}

// controlTabNew starts a tab, in the space an argument names or the shown
// one, and answers the ids it made. A window with no space makes one, as its
// own key does.
func (a *App) controlTabNew(raw json.RawMessage) (map[string]any, error) {
	args, err := controlArgs(raw, "name", "space", "folder", "argv")
	if err != nil {
		return nil, err
	}
	name, err := argText(args, "name")
	if err != nil {
		return nil, err
	}
	spaceID, err := argText(args, "space")
	if err != nil {
		return nil, err
	}
	folder, err := argText(args, "folder")
	if err != nil {
		return nil, err
	}
	argv, err := argList(args, "argv")
	if err != nil {
		return nil, err
	}
	sp, _ := a.S.Active()
	if spaceID != "" {
		sp = a.spaceByID(spaceID)
		if sp == nil {
			return nil, fmt.Errorf("no space %q", spaceID)
		}
		a.S.SelectSpace(0, sp.ID)
	}
	if sp == nil || len(a.S.Tabs()) == 0 {
		space, tab, err := a.S.NewSpace("", folder, "", argv)
		if err != nil {
			return nil, err
		}
		if name != "" {
			a.S.RenameTab(tab.ID, name)
		}
		return tabReply(a.S, space.ID, tab), nil
	}
	tab, err := a.S.NewTab(name, folder, argv)
	if err != nil {
		return nil, err
	}
	return tabReply(a.S, sp.ID, tab), nil
}

// controlTerminalKeep gives a terminal that is already running a pane in the
// shown tab: a worker the core started, or one the window parked.
func (a *App) controlTerminalKeep(raw json.RawMessage) (map[string]any, error) {
	args, err := controlArgs(raw, "terminal", "place")
	if err != nil {
		return nil, err
	}
	addr, err := argText(args, "terminal")
	if err != nil {
		return nil, err
	}
	if addr == "" {
		return nil, fmt.Errorf("terminal.keep needs terminal")
	}
	place, err := argText(args, "place")
	if err != nil {
		return nil, err
	}
	if place != "" && place != "tab" {
		return nil, fmt.Errorf("terminal.keep place %q is not here yet: this window keeps it in the shown tab", place)
	}
	// A terminal already on the screen comes to the front instead of a
	// second pane showing it.
	pane, front, err := a.keepTerminal(addr)
	if err != nil {
		return nil, err
	}
	if front {
		return map[string]any{"terminal": addr, "pane": pane}, nil
	}
	sp, tab := a.S.Active()
	if sp == nil || tab == nil {
		return nil, fmt.Errorf("no space to keep it in")
	}
	reply := map[string]any{"terminal": addr, "space": sp.ID, "tab": tab.ID}
	if pane != "" {
		reply["pane"] = pane
	}
	return reply, nil
}

// keepTerminal gives a terminal that is already running a pane in the shown
// tab: a worker the core started, or one the window parked, has none of its
// own. It answers the pane, and front when the terminal was already on the
// screen and came to the front instead.
func (a *App) keepTerminal(addr string) (pane string, front bool, err error) {
	if a.S.FocusTerminal(addr) {
		return a.S.FocusedPane().Pane, true, nil
	}
	if term := a.S.Terminal(addr); term == nil || !term.Live() {
		return "", false, fmt.Errorf("no running terminal %q", addr)
	}
	sp, tab := a.S.Active()
	if sp == nil || tab == nil {
		return "", false, fmt.Errorf("no space to keep it in")
	}
	if machineOf(addr) != herdr.SpaceMachine(sp) {
		return "", false, fmt.Errorf("a terminal's pane must stay on its own machine")
	}
	pane, err = a.S.KeepTerminal(addr)
	return pane, false, err
}

// tabReply is a new tab as a command answers it: its space, its tab and the
// terminal its pane runs.
func tabReply(s *herdr.Session, spaceID string, tab *herdr.Tab) map[string]any {
	reply := map[string]any{"space": spaceID, "tab": tab.ID}
	if panes := s.Panes(tab.ID); len(panes) > 0 {
		reply["pane"] = panes[0].Pane
		reply["terminal"] = panes[0].Terminal
	}
	return reply
}

// controlArgs checks a request's arguments against the ones a command takes,
// as herdr-app does: a name the command does not have is an error, never a
// guess at what was meant.
func controlArgs(raw json.RawMessage, takes ...string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("arguments are not a JSON object: %v", err)
	}
	for name := range out {
		if !slices.Contains(takes, name) {
			return nil, fmt.Errorf("no argument %q; it takes %s", name, strings.Join(takes, ", "))
		}
	}
	return out, nil
}

// argText is one string argument, "" when the request leaves it out.
func argText(args map[string]json.RawMessage, name string) (string, error) {
	raw, ok := args[name]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", fmt.Errorf("%s takes a string, not %s", name, raw)
	}
	return text, nil
}

// argList is one list-of-strings argument, nil when the request leaves it out.
func argList(args map[string]json.RawMessage, name string) ([]string, error) {
	raw, ok := args[name]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s takes a list of strings, not %s", name, raw)
	}
	return list, nil
}

// spaceByID finds a space of any machine by its id.
func (a *App) spaceByID(id string) *herdr.Space {
	for _, sp := range a.S.Places() {
		if sp.ID == id {
			return sp
		}
	}
	return nil
}

// machineOf is the machine a terminal address runs on, "" for this computer.
func machineOf(addr string) string {
	machine, _, _ := strings.Cut(addr, "/")
	if machine == addr {
		return ""
	}
	return machine
}
