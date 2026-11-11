package herdr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// runTerminal starts a program in a core terminal and returns its address.
func (s *Session) runTerminal(argv []string, cwd string, labels map[string]string, on string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if argv == nil {
		argv = []string{}
	}
	params := map[string]any{
		"argv":   argv,
		"cwd":    cwd,
		"env":    splitEnv(),
		"labels": labels,
		"keep":   true,
	}
	if on != "" {
		params["on"] = on
	}
	raw, err := s.Core.Call(ctx, "terminal.run", params)
	if err != nil {
		return "", err
	}
	var res struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Address == "" {
		return "", fmt.Errorf("the core started no terminal")
	}
	return res.Address, nil
}

// labelsFor names a new terminal into its space and tab, as Herdr does.
func labelsFor(spaceID, tabID string) map[string]string {
	l := map[string]string{"herdr.space": spaceID}
	if tabID != "" {
		l["herdr.tab"] = tabID
	}
	return l
}

// NewSpace makes a space with one tab and one terminal. machine is where its
// terminals run ("" is this computer); ids carry that machine's prefix.
func (s *Session) NewSpace(name, folder string, machine string, argv []string) (*Space, *Tab, error) {
	if folder == "" && machine == "" {
		if p := s.FocusedPane(); p.Cwd != "" {
			folder = p.Cwd
		}
	}
	var tag *string
	if machine != "" {
		m := machine
		tag = &m
	}
	spaceID := qualify(freshID("sp"), tag)
	tabID := qualify(freshID("tb"), tag)
	paneID := qualify(freshID("p"), tag)
	if folder != "" {
		if st, err := os.Stat(folder); err != nil || !st.IsDir() {
			folder = ""
		}
	}
	addr, err := s.runTerminal(argv, folder, labelsFor(spaceID, tabID), machine)
	if err != nil {
		return nil, nil, err
	}
	pane := &Node{ID: paneID, Type: "block", Content: &NodeContent{Terminal: addr}}
	root := &Node{ID: tabID, Type: "row", Children: []*Node{pane}}
	tab := &Tab{ID: tabID, Name: "", Root: root}
	sp := &Space{ID: spaceID, Name: name, Folder: folder, Tabs: []*Tab{tab}, Machine: tag}
	s.mu.Lock()
	s.spaces = append(s.spaces, sp)
	s.active = spaceID
	s.activeTab[spaceID] = tabID
	if v := s.tabs[tabID]; v == nil {
		s.tabs[tabID] = &tabView{focused: paneID}
	} else {
		v.focused = paneID
	}
	s.mu.Unlock()
	s.mu.Lock()
	orderSpaces(s.spaces)
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
	return sp, tab, nil
}

// CloseSpace ends a space and its terminals.
func (s *Session) CloseSpace(id string) {
	s.mu.Lock()
	var addrs []string
	for i, sp := range s.spaces {
		if sp.ID != id {
			continue
		}
		for _, t := range sp.Tabs {
			for _, b := range t.Root.blocks() {
				if a := b.terminal(); a != "" {
					addrs = append(addrs, a)
				}
			}
			delete(s.tabs, t.ID)
		}
		s.spaces = append(s.spaces[:i], s.spaces[i+1:]...)
		delete(s.activeTab, id)
		if s.active == id {
			s.active = ""
			if len(s.spaces) > 0 {
				s.active = s.spaces[max(0, i-1)].ID
			}
		}
		break
	}
	s.mu.Unlock()
	for _, a := range addrs {
		s.KillTerminal(a)
	}
	s.saveDoc()
	s.changed()
}

// RenameSpace names a space; "" goes back to its place's name.
func (s *Session) RenameSpace(id, name string) {
	s.mu.Lock()
	for _, sp := range s.spaces {
		if sp.ID == id {
			sp.Name = name
		}
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
}

// MoveSpace moves a space to a position (1 = first).
func (s *Session) MoveSpace(id string, index int) {
	s.mu.Lock()
	from := -1
	for i, sp := range s.spaces {
		if sp.ID == id {
			from = i
		}
	}
	if from >= 0 {
		sp := s.spaces[from]
		s.spaces = append(s.spaces[:from], s.spaces[from+1:]...)
		// A space stays among its own machine's spaces.
		first, last := -1, -1
		for i, other := range s.spaces {
			if sameMachine(other.Machine, sp.Machine) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		at := clamp(index-1, 0, len(s.spaces))
		if first >= 0 {
			at = clamp(at, first, last+1)
		}
		s.spaces = append(s.spaces[:at], append([]*Space{sp}, s.spaces[at:]...)...)
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
}

// SelectSpace shows a space by index (1-based) or id.
func (s *Session) SelectSpace(index int, id string) {
	s.mu.Lock()
	if id == "" && index > 0 && index <= len(s.spaces) {
		s.active = s.spaces[index-1].ID
	} else {
		for _, sp := range s.spaces {
			if sp.ID == id {
				s.active = id
			}
		}
	}
	s.mu.Unlock()
	s.changed()
}

// NextSpace shows the space after or before this one.
func (s *Session) NextSpace(delta int) {
	s.mu.Lock()
	at := -1
	for i, sp := range s.spaces {
		if sp.ID == s.active {
			at = i
		}
	}
	if len(s.spaces) > 0 {
		n := len(s.spaces)
		s.active = s.spaces[((at+delta)%n+n)%n].ID
	}
	s.mu.Unlock()
	s.changed()
}

// NewTab adds a tab with one terminal to the shown space.
func (s *Session) NewTab(name, folder string, argv []string) (*Tab, error) {
	sp, _ := s.Active()
	if sp == nil {
		return nil, fmt.Errorf("no space")
	}
	if folder == "" {
		folder = sp.firstCwd(s.termSnapshot())
	}
	tabID := qualify(freshID("tb"), sp.Machine)
	paneID := qualify(freshID("p"), sp.Machine)
	on := ""
	if sp.Machine != nil {
		on = *sp.Machine
	}
	addr, err := s.runTerminal(argv, folder, labelsFor(sp.ID, tabID), on)
	if err != nil {
		return nil, err
	}
	pane := &Node{ID: paneID, Type: "block", Content: &NodeContent{Terminal: addr}}
	tab := &Tab{ID: tabID, Name: name, Root: &Node{ID: tabID, Type: "row", Children: []*Node{pane}}}
	s.mu.Lock()
	for _, other := range s.spaces {
		if other.ID == sp.ID {
			other.Tabs = append(other.Tabs, tab)
			s.activeTab[sp.ID] = tabID
			s.tabs[tabID] = &tabView{focused: paneID}
		}
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
	return tab, nil
}

// CloseTab ends a tab, its panes and their terminals.
func (s *Session) CloseTab(id string) {
	s.mu.Lock()
	var addrs []string
	var spaceID string
	for _, sp := range s.spaces {
		for i, t := range sp.Tabs {
			if t.ID != id {
				continue
			}
			spaceID = sp.ID
			for _, b := range t.Root.blocks() {
				if a := b.terminal(); a != "" {
					addrs = append(addrs, a)
				}
			}
			sp.Tabs = append(sp.Tabs[:i], sp.Tabs[i+1:]...)
			delete(s.tabs, id)
			if s.activeTab[sp.ID] == id {
				s.activeTab[sp.ID] = ""
				if len(sp.Tabs) > 0 {
					s.activeTab[sp.ID] = sp.Tabs[min(i, len(sp.Tabs)-1)].ID
				}
			}
			break
		}
	}
	empty := false
	if spaceID != "" {
		for i, sp := range s.spaces {
			if sp.ID == spaceID && len(sp.Tabs) == 0 {
				s.spaces = append(s.spaces[:i], s.spaces[i+1:]...)
				empty = true
				if s.active == spaceID {
					s.active = ""
					if len(s.spaces) > 0 {
						s.active = s.spaces[max(0, i-1)].ID
					}
				}
			}
		}
	}
	s.mu.Unlock()
	for _, a := range addrs {
		s.KillTerminal(a)
	}
	_ = empty
	s.saveDoc()
	s.changed()
}

// RenameTab names a tab; "" gives it its number again.
func (s *Session) RenameTab(id, name string) {
	s.mu.Lock()
	for _, sp := range s.spaces {
		for _, t := range sp.Tabs {
			if t.ID == id {
				t.Name = name
			}
		}
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
}

// MoveTab moves a tab of the shown space (1 = first).
func (s *Session) MoveTab(id string, index int) {
	s.mu.Lock()
	for _, sp := range s.spaces {
		from := -1
		for i, t := range sp.Tabs {
			if t.ID == id {
				from = i
			}
		}
		if from < 0 {
			continue
		}
		t := sp.Tabs[from]
		sp.Tabs = append(sp.Tabs[:from], sp.Tabs[from+1:]...)
		at := clamp(index-1, 0, len(sp.Tabs))
		sp.Tabs = append(sp.Tabs[:at], append([]*Tab{t}, sp.Tabs[at:]...)...)
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
}

// NextTab shows the next or previous tab of the shown space.
func (s *Session) NextTab(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sp, t := s.activeLocked()
	if sp == nil || t == nil || len(sp.Tabs) == 0 {
		return
	}
	at := 0
	for i, other := range sp.Tabs {
		if other.ID == t.ID {
			at = i
		}
	}
	n := len(sp.Tabs)
	s.activeTab[sp.ID] = sp.Tabs[((at+delta)%n+n)%n].ID
	go func() { s.changed() }()
}

// Split adds a pane to the focused one: "right" or "down".
func (s *Session) Split(direction string, argv []string) (*PaneInfo, error) {
	sp, tab := s.Active()
	if sp == nil || tab == nil {
		return nil, fmt.Errorf("no tab")
	}
	focused := ""
	s.mu.Lock()
	if v := s.tabs[tab.ID]; v != nil {
		focused = v.focused
	}
	s.mu.Unlock()
	if focused == "" {
		return nil, fmt.Errorf("no pane")
	}
	folder := sp.firstCwd(s.termSnapshot())
	s.mu.Lock()
	if v := s.tabs[tab.ID]; v != nil {
		if b := tab.Root.find(v.focused); b != nil {
			if term := s.terminals[b.terminal()]; term != nil {
				if term.Foreground != nil && term.Foreground.Cwd != "" {
					folder = term.Foreground.Cwd
				} else if term.Cwd != "" {
					folder = term.Cwd
				}
			}
		}
	}
	s.mu.Unlock()
	on := ""
	if sp.Machine != nil {
		on = *sp.Machine
	}
	addr, err := s.runTerminal(argv, folder, labelsFor(sp.ID, tab.ID), on)
	if err != nil {
		return nil, err
	}
	paneID := qualify(freshID("p"), sp.Machine)
	s.mu.Lock()
	target := tab.Root.find(focused)
	if target == nil {
		s.mu.Unlock()
		s.KillTerminal(addr)
		return nil, fmt.Errorf("the pane is gone")
	}
	pane := &Node{ID: paneID, Type: "block", Content: &NodeContent{Terminal: addr}}
	axis := "row"
	if direction == "down" {
		axis = "column"
	}
	weight := 0.5
	parent := tab.Root.parent(target.ID)
	if target == tab.Root {
		root := &Node{ID: tab.ID, Type: axis, Children: []*Node{target, pane}}
		target.Size = &Size{Weight: &weight}
		pane.Size = &Size{Weight: &weight}
		tab.Root = root
	} else if parent != nil && parent.Type == axis {
		target.Size = &Size{Weight: &weight}
		pane.Size = &Size{Weight: &weight}
		for i, c := range parent.Children {
			if c == target {
				parent.Children = append(parent.Children[:i+1], append([]*Node{pane}, parent.Children[i+1:]...)...)
				break
			}
		}
	} else if parent != nil {
		w := 1.0
		box := &Node{ID: freshID("p"), Type: axis, Size: target.Size, Children: []*Node{target, pane}}
		target.Size = &Size{Weight: &w}
		pane.Size = &Size{Weight: &w}
		for i, c := range parent.Children {
			if c == target {
				parent.Children[i] = box
				break
			}
		}
	}
	if v := s.tabs[tab.ID]; v != nil {
		v.focused = paneID
		if v.before != nil {
			v.before = append(v.before, focused)
		}
		v.zoomed = ""
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
	return &PaneInfo{Pane: paneID, Terminal: addr, Tab: tab.ID, Focused: true}, nil
}

// ClosePane removes a pane and ends its terminal, or the tab if it was the last.
func (s *Session) ClosePane(pane string) {
	s.closePane(pane, "", false)
}

// ClosePaneIfTerminal removes a pane only if it still owns expectedTerminal and
// is not temporarily showing another terminal. It returns whether it closed.
// This removes the pane without killing the terminal: a finished terminal may
// have restarted since the view was closed.
func (s *Session) ClosePaneIfTerminal(pane, expectedTerminal string) bool {
	if pane == "" || expectedTerminal == "" {
		return false
	}
	return s.closePane(pane, expectedTerminal, true)
}

func (s *Session) closePane(pane, expected string, check bool) bool {
	s.mu.Lock()
	var addr string
	closed := false
search:
	for si, sp := range s.spaces {
		for ti, t := range sp.Tabs {
			node := t.Root.find(pane)
			if node == nil {
				continue
			}
			if check && (node.Type != "block" || node.terminal() != expected || s.query[pane] != "") {
				s.mu.Unlock()
				return false
			}
			closed = true
			addr = node.terminal()
			last := len(t.Root.blocks()) <= 1
			if last {
				sp.Tabs = append(sp.Tabs[:ti], sp.Tabs[ti+1:]...)
				delete(s.tabs, t.ID)
				if s.activeTab[sp.ID] == t.ID {
					s.activeTab[sp.ID] = ""
					if len(sp.Tabs) > 0 {
						s.activeTab[sp.ID] = sp.Tabs[min(ti, len(sp.Tabs)-1)].ID
					}
				}
				if len(sp.Tabs) == 0 {
					s.spaces = append(s.spaces[:si], s.spaces[si+1:]...)
					delete(s.activeTab, sp.ID)
					if s.active == sp.ID {
						s.active = ""
						if len(s.spaces) > 0 {
							s.active = s.spaces[max(0, si-1)].ID
						}
					}
				}
			} else {
				parent := t.Root.parent(pane)
				if parent != nil {
					for i, c := range parent.Children {
						if c.ID == pane {
							// The neighbor grows toward the removed pane; the other
							// siblings keep their shares of this split.
							if len(parent.Children) > 2 {
								at := i - 1
								if at < 0 {
									at = i + 1
								}
								absorbPaneSize(parent.Children[at], c)
							}
							parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
							break
						}
					}
					if len(parent.Children) == 1 && parent != t.Root {
						gp := t.Root.parent(parent.ID)
						if gp != nil {
							for i, c := range gp.Children {
								if c.ID == parent.ID {
									// The child's size was along the inner split's
									// axis. Inherit its parent's outer allocation.
									parent.Children[0].Size = parent.Size
									gp.Children[i] = parent.Children[0]
									break
								}
							}
						}
					}
				}
				if v := s.tabs[t.ID]; v != nil {
					if v.focused == pane {
						v.focused = ""
						for i := len(v.before) - 1; i >= 0; i-- {
							if t.Root.find(v.before[i]) != nil {
								v.focused = v.before[i]
								v.before = v.before[:i]
								break
							}
						}
						if v.focused == "" {
							for _, b := range t.Root.blocks() {
								v.focused = b.ID
								break
							}
						}
					}
					if v.zoomed == pane {
						v.zoomed = ""
					}
				}
			}
			delete(s.query, pane)
			delete(s.borrowed, pane)
			break search
		}
	}
	s.mu.Unlock()
	if !closed {
		return false
	}
	// A checked close comes from a finished terminal view. Its old address
	// may have restarted; removing the pane must not kill that process.
	if !check && addr != "" && s.Core != nil {
		s.KillTerminal(addr)
	}
	s.saveDoc()
	s.changed()
	return true
}

// absorbPaneSize gives a neighbor the removed pane's share in a flat split.
// A missing size grows with weight one, as the pane renderer does.
func absorbPaneSize(neighbor, removed *Node) {
	weight := func(n *Node) float64 {
		if n.Size != nil {
			if n.Size.Weight != nil && *n.Size.Weight > 0 {
				return *n.Size.Weight
			}
			if n.Size.PX != nil && *n.Size.PX > 0 {
				return 0
			}
		}
		return 1
	}
	a, b := weight(neighbor), weight(removed)
	if a > 0 && b > 0 {
		sum := a + b
		neighbor.Size = &Size{Weight: &sum}
	} else if a == 0 && b == 0 {
		sum := *neighbor.Size.PX + *removed.Size.PX
		neighbor.Size = &Size{PX: &sum}
	}
	// Mixed px/weight pairs keep their existing allocation: converting
	// between them requires the rendered extent, which the document lacks.
}

// ZoomPane shows one pane of the tab full size, or all again.
func (s *Session) ZoomPane(pane string) {
	s.mu.Lock()
	for _, sp := range s.spaces {
		for _, t := range sp.Tabs {
			if t.Root.find(pane) == nil {
				continue
			}
			v := s.tabs[t.ID]
			if v == nil {
				continue
			}
			if v.zoomed == pane {
				v.zoomed = ""
			} else {
				v.zoomed = pane
				v.focused = pane
			}
		}
	}
	s.mu.Unlock()
	s.changed()
}

// Go moves the focus to the pane along an edge.
func (s *Session) Go(direction string) {
	sp, tab := s.Active()
	if sp == nil || tab == nil {
		return
	}
	s.mu.Lock()
	v := s.tabs[tab.ID]
	if v == nil || len(tab.Root.blocks()) < 2 {
		s.mu.Unlock()
		return
	}
	panes := tab.Root.blocks()
	order := map[string]int{}
	for i, b := range panes {
		order[b.ID] = i
	}
	at, ok := order[v.focused]
	if !ok {
		at = 0
	}
	delta := 1
	if direction == "left" || direction == "up" {
		delta = -1
	}
	v.focused = panes[((at+delta)%len(panes)+len(panes))%len(panes)].ID
	s.mu.Unlock()
	s.changed()
}

// Swap exchanges the focused pane with its neighbour along an edge.
func (s *Session) Swap(direction string) {
	sp, tab := s.Active()
	if sp == nil || tab == nil {
		return
	}
	s.mu.Lock()
	v := s.tabs[tab.ID]
	if v == nil {
		s.mu.Unlock()
		return
	}
	node := tab.Root.find(v.focused)
	if node == nil {
		s.mu.Unlock()
		return
	}
	parent := tab.Root.parent(node.ID)
	if parent == nil {
		s.mu.Unlock()
		return
	}
	at := -1
	for i, c := range parent.Children {
		if c == node {
			at = i
		}
	}
	delta := 1
	if direction == "left" || direction == "up" {
		delta = -1
	}
	to := at + delta
	if at >= 0 && to >= 0 && to < len(parent.Children) {
		parent.Children[at], parent.Children[to] = parent.Children[to], parent.Children[at]
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
}

// SetWeights gives nodes of a tab new weights, as dragging the divider
// between two panes does. It changes the model only: the document keeps the
// sizes a drag leaves when the window writes it, on letting go.
func (s *Session) SetWeights(tabID string, weights map[string]float64) bool {
	found := false
	s.mu.Lock()
	for _, sp := range s.spaces {
		for _, t := range sp.Tabs {
			if t.ID != tabID {
				continue
			}
			found = true
			for id, w := range weights {
				if n := t.Root.find(id); n != nil {
					weight := w
					n.Size = &Size{Weight: &weight}
				}
			}
		}
	}
	s.mu.Unlock()
	if found {
		s.changed()
	}
	return found
}

// OpenTerminal starts a program in the shown space: a new tab, or a split.
func (s *Session) OpenTerminal(argv []string, folder, place string) {
	sp, _ := s.Active()
	if sp == nil {
		_, _, _ = s.NewSpace("", folder, "", argv)
		return
	}
	switch place {
	case "right", "down":
		_, _ = s.Split(place, argv)
	default:
		_, _ = s.NewTab("", folder, argv)
	}
}

// PutResult is where a file sent beside a terminal landed.
type PutResult struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// Put writes a file beside a terminal, on the machine it runs on, for its
// program to read: a pasted image's bytes, which a program there cannot read
// from this computer's clipboard. The core keeps the name (a suffix when it
// is taken) in a folder of its own that goes with the terminal, and types
// nothing: the caller pastes the path it returns.
func (s *Session) Put(terminal, name string, data []byte) (PutResult, error) {
	var out PutResult
	if s.Core == nil {
		return out, errNoCore
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := s.Core.Call(ctx, "terminal.put", map[string]any{
		"terminal": terminal,
		"name":     name,
		"data":     base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return PutResult{}, err
	}
	return out, nil
}

// KillTerminal ends a terminal's program.
func (s *Session) KillTerminal(addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s.Core.Call(ctx, "terminal.kill", map[string]any{"terminal": addr})
}

// ParkTerminal removes a terminal's pane and keeps it running.
func (s *Session) ParkTerminal(addr string) {
	s.mu.Lock()
	for _, sp := range s.spaces {
		for _, t := range sp.Tabs {
			for _, b := range t.Root.blocks() {
				if b.terminal() == addr {
					s.mu.Unlock()
					s.ClosePane(b.ID)
					return
				}
			}
		}
	}
	s.mu.Unlock()
}

// KeepTerminal gives a running terminal a pane in the shown tab, and returns
// the pane's id. It also tells the core to keep the terminal, so a program it
// loses comes back as a shell in that pane, as Herdr keeps one it was given.
func (s *Session) KeepTerminal(addr string) (string, error) {
	sp, tab := s.Active()
	if sp == nil || tab == nil {
		return "", fmt.Errorf("no tab")
	}
	paneID := freshID("p")
	if err := s.label(addr, labelsFor(sp.ID, tab.ID)); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.Core.Call(ctx, "terminal.keep", map[string]any{"terminal": addr, "keep": true}); err != nil {
		return "", err
	}
	s.mu.Lock()
	pane := &Node{ID: paneID, Type: "block", Content: &NodeContent{Terminal: addr}}
	if len(tab.Root.blocks()) == 1 {
		tab.Root.Children = append(tab.Root.Children, pane)
		tab.Root.Type = "row"
	} else if parent := tab.Root.parent(tab.Root.blocks()[0].ID); parent != nil {
		parent.Children = append(parent.Children, pane)
	}
	if v := s.tabs[tab.ID]; v != nil {
		v.focused = paneID
		v.zoomed = ""
	}
	s.mu.Unlock()
	s.saveDoc()
	s.changed()
	return paneID, nil
}

// label sets a terminal's labels.
func (s *Session) label(addr string, labels map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.Core.Call(ctx, "terminal.label", map[string]any{"terminal": addr, "labels": labels})
	return err
}

// ShowTerminal shows a terminal in a pane in place of the one it runs.
func (s *Session) ShowTerminal(addr string, pane string) {
	s.mu.Lock()
	if pane == "" {
		for _, p := range s.focusedPanesLocked() {
			pane = p
		}
	}
	if pane != "" {
		if b := s.findPaneLocked(pane); b != nil {
			s.query[pane] = addr
			s.markSeenLocked(addr)
		}
	}
	s.mu.Unlock()
	s.changed()
}

// HideTerminal puts a pane back to the terminal it runs.
func (s *Session) HideTerminal(pane string) {
	s.mu.Lock()
	delete(s.query, pane)
	s.mu.Unlock()
	s.changed()
}

func (s *Session) focusedPanesLocked() []string {
	_, t := s.activeLocked()
	if t == nil {
		return nil
	}
	if v := s.tabs[t.ID]; v != nil && v.focused != "" {
		return []string{v.focused}
	}
	return nil
}

func (s *Session) findPaneLocked(pane string) *Node {
	for _, sp := range s.spaces {
		for _, t := range sp.Tabs {
			if b := t.Root.find(pane); b != nil {
				return b
			}
		}
	}
	return nil
}

// termSnapshot copies the terminal records for use outside the lock.
func (s *Session) termSnapshot() map[string]*Terminal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*Terminal, len(s.terminals))
	for k, v := range s.terminals {
		out[k] = v
	}
	return out
}

// QueryOf returns the terminal a pane shows instead of its own, if any.
func (s *Session) QueryOf(pane string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query[pane]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = sort.Strings
