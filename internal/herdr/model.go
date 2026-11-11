// Package herdr is the app's model of the world herdr-core runs: the spaces,
// tabs and panes Herdr keeps in the core's "herdr-app" document, and the
// terminals behind them.
package herdr

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// Node is one pane of a tab, or the row/column that holds panes: the part of
// a layout node a tab's document stores (herdr-app's layouts).
type Node struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "row", "column" or "block"
	Size     *Size           `json:"size,omitempty"`
	Children []*Node         `json:"children,omitempty"`
	Content  *NodeContent    `json:"content,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// Size is a node's share: px, or weight of what fixed siblings leave.
type Size struct {
	PX     *float64 `json:"px,omitempty"`
	Weight *float64 `json:"weight,omitempty"`
}

// NodeContent says what a block shows: a terminal, or a mod pane.
type NodeContent struct {
	Terminal string          `json:"terminal,omitempty"`
	Mod      string          `json:"mod,omitempty"`
	Props    json.RawMessage `json:"props,omitempty"`
}

// UnmarshalJSON keeps fields this app does not use (pane looks, hidden) so a
// document it writes back keeps them.
func (n *Node) UnmarshalJSON(b []byte) error {
	type plain Node
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*n = Node(p)
	n.Raw = append(n.Raw[:0], b...)
	return nil
}

// MarshalJSON writes back what was read, with the fields this app changed.
func (n *Node) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if len(n.Raw) > 0 {
		_ = json.Unmarshal(n.Raw, &out)
	}
	out["id"] = n.ID
	out["type"] = n.Type
	if n.Size != nil {
		out["size"] = n.Size
	} else {
		delete(out, "size")
	}
	if len(n.Children) > 0 {
		out["children"] = n.Children
	} else {
		delete(out, "children")
	}
	if n.Content != nil {
		out["content"] = n.Content
	}
	return json.Marshal(out)
}

// terminal is the address of the terminal the pane shows, if any.
func (n *Node) terminal() string {
	if n.Content != nil {
		return n.Content.Terminal
	}
	return ""
}

// find returns the node with the id, depth first.
func (n *Node) find(id string) *Node {
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if f := c.find(id); f != nil {
			return f
		}
	}
	return nil
}

// parent returns the node whose children hold id.
func (n *Node) parent(id string) *Node {
	for _, c := range n.Children {
		if c.ID == id {
			return n
		}
		if p := c.parent(id); p != nil {
			return p
		}
	}
	return nil
}

// blocks returns the terminal blocks of a tab, in document order.
func (n *Node) blocks() []*Node {
	if n.Type == "block" {
		return []*Node{n}
	}
	var out []*Node
	for _, c := range n.Children {
		out = append(out, c.blocks()...)
	}
	return out
}

// Tab is a named piece of work in a space: its panes and how they are split.
type Tab struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Root   *Node  `json:"root"`
	Active bool   `json:"-"`
	Index  int    `json:"-"`
}

// Space is a place to work: a machine and a folder, with tabs.
type Space struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Folder  string  `json:"folder"`
	Tabs    []*Tab  `json:"tabs"`
	Machine *string `json:"machine,omitempty"`
	Active  bool    `json:"-"`
	Index   int     `json:"-"`
}

// doc is the document every Herdr screen of a machine shares.
type doc struct {
	Spaces []*Space `json:"spaces"`
}

// Terminal is one core terminal (hc ls / terminal.get).
type Terminal struct {
	Address    string            `json:"address"`
	Argv       []string          `json:"argv"`
	Cols       int               `json:"cols"`
	Rows       int               `json:"rows"`
	Cwd        string            `json:"cwd"`
	Name       *string           `json:"name"`
	Title      string            `json:"title"`
	State      string            `json:"state"` // live, finished
	Status     *Status           `json:"status"`
	StatusMs   *int64            `json:"status_ms"`
	Turn       int               `json:"turn"`
	Keep       bool              `json:"keep"`
	Labels     map[string]string `json:"labels"`
	Parent     *string           `json:"parent"`
	Reason     *string           `json:"reason"`
	ExitCode   *int              `json:"exit_code"`
	Pid        *int              `json:"pid"`
	Foreground *Foreground       `json:"foreground"`
	Reported   bool              `json:"reported"`
	Template   *string           `json:"template"`
	Where      string            `json:"where"`
}

// Status is what a program reported it is doing through OSC 7501: the core's
// `status` object, one report. Words are the program's own; nothing is
// translated.
type Status struct {
	State    string `json:"state"`          // idle, working, done, blocked or error
	App      string `json:"app,omitempty"`  // the app it says it is
	Kind     string `json:"kind,omitempty"` // permission, question or auth (blocked only)
	Msg      string `json:"msg,omitempty"`
	Title    string `json:"title,omitempty"`
	Progress *int   `json:"progress,omitempty"` // 0..100 (working or blocked)
}

// Word is the state word the program reported, "" when it said nothing.
func (st *Status) Word() string {
	if st == nil {
		return ""
	}
	return st.State
}

// app is the app the program said it is, "" when it did not say.
func (st *Status) app() string {
	if st == nil {
		return ""
	}
	return st.App
}

// Foreground is the process in front of the terminal's shell.
type Foreground struct {
	Pid  int      `json:"pid"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
}

// activeStatus ignores a retained report once the original shell owns the
// foreground again. The terminal itself remains live after its agent exits.
func (t *Terminal) activeStatus() *Status {
	if fg := t.Foreground; fg != nil && t.Pid != nil && fg.Pid > 0 && fg.Pid == *t.Pid {
		name := fg.Name
		if len(fg.Argv) > 0 {
			name = fg.Argv[0]
		}
		if inList(shellNames, base(name)) && !shellCommand(fg.Argv) {
			return nil
		}
	}
	return t.Status
}

// Word returns the agent's reported word, "" when it reports none or exited.
func (t *Terminal) Word() string { return t.activeStatus().Word() }

// AgentTitle is the agent's reported session title, falling back to the
// terminal title when the program does not report one.
func (t *Terminal) AgentTitle() string {
	if st := t.activeStatus(); st != nil && st.Title != "" {
		return st.Title
	}
	return t.Title
}

// Live reports whether the terminal's program runs.
func (t *Terminal) Live() bool { return t.State == "live" }

// The app's names for unlabeled things.
func (t *Terminal) label(k string) string { return t.Labels[k] }

// paneOf returns the pane and tab the terminal is labeled into, if any.
func (t *Terminal) paneOf() (string, string) { return t.label("herdr.pane"), t.label("herdr.tab") }

// freshID makes an id as Herdr does: a prefix and 32 hex digits.
func freshID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// SpaceName is how a space shows: its name, else the repo (or folder) its
// first terminal is in now, "~" at home.
func SpaceName(s *Space, terminals map[string]*Terminal) string {
	home, _ := os.UserHomeDir()
	return spaceNameWith(s, terminals, home)
}

// spaceNameWith names a space, calling its folder "~" when it is the home of
// the machine it lives on.
func spaceNameWith(s *Space, terminals map[string]*Terminal, home string) string {
	if s.Name != "" {
		return s.Name
	}
	where := s.Folder
	for _, t := range s.Tabs {
		for _, b := range t.Root.blocks() {
			if term := terminals[b.terminal()]; term != nil {
				if term.Foreground != nil && term.Foreground.Cwd != "" {
					where = term.Foreground.Cwd
				} else if term.Cwd != "" {
					where = term.Cwd
				}
				goto found
			}
		}
	}
found:
	if where == "" {
		return "~"
	}
	// Remote paths use their machine's separators, regardless of our OS.
	where = strings.ReplaceAll(where, "\\", "/")
	home = strings.ReplaceAll(home, "\\", "/")
	where = strings.TrimRight(where, "/")
	if where == "" {
		return "/"
	}
	if home != "" && where == strings.TrimRight(home, "/") {
		return "~"
	}
	return path.Base(where)
}

// TabName is how a tab shows: its name, else its number.
func TabName(t *Tab, index int) string {
	if t.Name != "" {
		return t.Name
	}
	return fmt.Sprint(index)
}

// firstCwd is where a space's new terminals start: its first terminal's live
// folder, else the folder it was made in.
func (s *Space) firstCwd(terminals map[string]*Terminal) string {
	for _, t := range s.Tabs {
		for _, b := range t.Root.blocks() {
			if term := terminals[b.terminal()]; term != nil {
				if term.Foreground != nil && term.Foreground.Cwd != "" {
					return term.Foreground.Cwd
				}
				if term.Cwd != "" {
					return term.Cwd
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil && s.Folder == "" {
		return home
	}
	return s.Folder
}

// StatusRank is Herdr's attention order for a status word: blocked > done >
// working > idle, 0 for a word the app does not know.
func StatusRank(s string) int {
	switch s {
	case "blocked":
		return 4
	case "done":
		return 3
	case "working":
		return 2
	case "idle":
		return 1
	}
	return 0
}

// worse returns the more urgent of two status words.
func worse(a, b string) string {
	if StatusRank(b) > StatusRank(a) {
		return b
	}
	return a
}

// sortedMachines names the machines of the spaces that are not this computer.
func sortedMachines(spaces []*Space) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range spaces {
		if s.Machine != nil && !seen[*s.Machine] {
			seen[*s.Machine] = true
			out = append(out, *s.Machine)
		}
	}
	sort.Strings(out)
	return out
}

// splitEnv is what the app tells a program about its terminal view.
func splitEnv() map[string]string {
	env := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "TERM_PROGRAM": "herdr"}
	// Rove's MyGo views support Kitty graphics. The core answers DA1 before
	// forwarding a viewer's graphics reply, so dsh-tui's DA1 probe barrier
	// can otherwise finish before it learns that images are supported.
	// Advertise the known protocol while keeping an explicit user override.
	protocol, set := os.LookupEnv("DSH_TUI_IMAGE_PROTOCOL")
	if !set || protocol == "" {
		protocol = "kitty"
	}
	env["DSH_TUI_IMAGE_PROTOCOL"] = protocol
	return env
}

// trimQ removes one pair of surrounding quotes, for terminal titles.
func trimQ(s string) string { return strings.TrimSpace(s) }

// clone copies a node tree, for views to read outside the session's lock.
func (n *Node) clone() *Node {
	if n == nil {
		return nil
	}
	cp := &Node{ID: n.ID, Type: n.Type, Content: n.Content}
	if n.Size != nil {
		size := *n.Size
		cp.Size = &size
	}
	for _, c := range n.Children {
		cp.Children = append(cp.Children, c.clone())
	}
	cp.Raw = append([]byte(nil), n.Raw...)
	return cp
}

// qualify marks an id with the machine it belongs to, as the app's model
// does: a space on rohan has ids "rohan/sp…", its terminal "rohan/t3".
func qualify(id string, machine *string) string {
	if machine == nil || *machine == "" || id == "" {
		return id
	}
	return *machine + "/" + id
}

// localID is what a machine's own document calls an id.
func localID(id string, machine *string) string {
	if machine == nil || *machine == "" {
		return id
	}
	return strings.TrimPrefix(id, *machine+"/")
}

// sameMachine reports whether a space (or a nil machine) is this machine.
func sameMachine(have, want *string) bool {
	if have == nil || *have == "" {
		return want == nil || *want == ""
	}
	return want != nil && *have == *want
}

// mapIDs copies a node tree, renaming its ids and the terminals it shows.
func (n *Node) mapIDs(f func(string) string) *Node {
	if n == nil {
		return nil
	}
	cp := &Node{ID: f(n.ID), Type: n.Type, Raw: append([]byte(nil), n.Raw...)}
	if n.Size != nil {
		size := *n.Size
		cp.Size = &size
	}
	if n.Content != nil {
		content := *n.Content
		content.Terminal = f(content.Terminal)
		cp.Content = &content
	}
	for _, c := range n.Children {
		cp.Children = append(cp.Children, c.mapIDs(f))
	}
	return cp
}

// docTab and docSpace are a machine's document, whose ids are that machine's
// own (no "rohan/" prefix).
type docTab struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root *Node  `json:"root"`
}

type docSpace struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Folder string   `json:"folder"`
	Tabs   []docTab `json:"tabs"`
}

type docFile struct {
	Spaces []docSpace `json:"spaces"`
}

// captured is a machine's share of the model, as its document stores it.
func captured(spaces []*Space, machine *string) docFile {
	local := func(id string) string { return localID(id, machine) }
	var out docFile
	for _, sp := range spaces {
		if !sameMachine(sp.Machine, machine) {
			continue
		}
		ds := docSpace{ID: local(sp.ID), Name: sp.Name, Folder: sp.Folder}
		for _, t := range sp.Tabs {
			ds.Tabs = append(ds.Tabs, docTab{ID: local(t.ID), Name: t.Name, Root: t.Root.mapIDs(local)})
		}
		out.Spaces = append(out.Spaces, ds)
	}
	return out
}

// fromDoc reads a machine's document into spaces of this model.
func fromDoc(d docFile, machine *string) []*Space {
	var out []*Space
	for _, ds := range d.Spaces {
		sp := &Space{ID: qualify(ds.ID, machine), Name: ds.Name, Folder: ds.Folder, Machine: machine}
		for i, dt := range ds.Tabs {
			root := dt.Root
			if root == nil {
				root = &Node{ID: dt.ID, Type: "row"}
			}
			root = root.mapIDs(func(id string) string { return qualify(id, machine) })
			tab := &Tab{ID: qualify(dt.ID, machine), Name: dt.Name, Root: root, Index: i + 1}
			sp.Tabs = append(sp.Tabs, tab)
		}
		out = append(out, sp)
	}
	return out
}

// orderSpaces puts this computer's spaces first, then each machine's, as
// every Herdr screen shows them.
func orderSpaces(spaces []*Space) {
	sort.SliceStable(spaces, func(i, j int) bool {
		a, b := spaces[i].Machine, spaces[j].Machine
		if a == nil || *a == "" {
			return b != nil && *b != ""
		}
		if b == nil || *b == "" {
			return false
		}
		return *a < *b
	})
}

// SpaceMachine is a space's machine name, "" for this computer.
func SpaceMachine(sp *Space) string {
	if sp == nil || sp.Machine == nil {
		return ""
	}
	return *sp.Machine
}

// TerminalMachine is the machine a terminal address runs on, "" for this
// computer: addresses here are t7, there machine/t7.
func TerminalMachine(addr string) string {
	if i := strings.IndexByte(addr, '/'); i > 0 {
		return addr[:i]
	}
	return ""
}
