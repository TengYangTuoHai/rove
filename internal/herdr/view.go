package herdr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// AppHome is the folder this window keeps its own files in: settings.json,
// keys.json and view.json. It is not herdr-app's folder: Herdr's window has
// settings of its own, and keeps them apart from the original app's.
func AppHome() string {
	if h := os.Getenv("ROVE_HOME"); h != "" {
		return h
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "rove")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "rove")
}

// viewFile is what this screen keeps: the shown space, each space's tab,
// each tab's focus and the order the spaces and machine groups were dragged into.
type viewFile struct {
	Active     string                    `json:"active"`
	Spaces     map[string]string         `json:"spaces"`
	Tabs       map[string]viewTab        `json:"tabs"`
	Machines   []string                  `json:"machines,omitempty"`
	SpaceOrder []string                  `json:"space_order,omitempty"`
	Recent     []map[string]string       `json:"recent,omitempty"`
	Seen       map[string]map[string]any `json:"attention,omitempty"`
}

type viewTab struct {
	Focused string   `json:"focused"`
	Before  []string `json:"before"`
	Zoomed  *string  `json:"zoomed"`
	Settled *uint64  `json:"settled"`
}

// LoadView restores this screen's selection, focus, zoom and dragged order.
func (s *Session) LoadView() {
	text, err := os.ReadFile(filepath.Join(AppHome(), "view.json"))
	if err != nil {
		return
	}
	var v viewFile
	if json.Unmarshal(text, &v) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.machineOrder = v.Machines
	s.spaceOrder = v.SpaceOrder
	s.restoreSpaceOrderLocked()
	for _, sp := range s.spaces {
		if id, ok := v.Spaces[sp.ID]; ok {
			s.activeTab[sp.ID] = id
		}
		for _, t := range sp.Tabs {
			if vt, ok := v.Tabs[t.ID]; ok {
				tv := &tabView{focused: vt.Focused, before: vt.Before}
				if vt.Zoomed != nil {
					tv.zoomed = *vt.Zoomed
				}
				s.tabs[t.ID] = tv
			}
		}
	}
	if v.Active != "" {
		s.wantActive = v.Active
		for _, sp := range s.spaces {
			if sp.ID == v.Active {
				s.active = v.Active
			}
		}
	}
}

// rememberSpaceOrderLocked updates the moved machine's visible slots. IDs not
// loaded now keep their slots, including spaces on disconnected machines.
func (s *Session) rememberSpaceOrderLocked(machine *string) {
	var current []string
	present := make(map[string]bool)
	for _, sp := range s.spaces {
		if sameMachine(sp.Machine, machine) {
			current = append(current, sp.ID)
			present[sp.ID] = true
		}
	}
	order := make([]string, 0, len(s.spaceOrder)+len(current))
	next := 0
	for _, id := range s.spaceOrder {
		if present[id] {
			if next < len(current) {
				order = append(order, current[next])
				next++
			}
		} else {
			order = append(order, id)
		}
	}
	s.spaceOrder = append(order, current[next:]...)
}

// restoreSpaceOrderLocked applies the window's saved order within each machine
// group. New spaces keep document order after saved IDs; absent IDs stay saved
// for documents that may arrive later. Call after orderSpaces groups the model.
func (s *Session) restoreSpaceOrderLocked() {
	if len(s.spaceOrder) == 0 {
		return
	}
	rank := make(map[string]int, len(s.spaceOrder))
	for i, id := range s.spaceOrder {
		rank[id] = i
	}
	for first := 0; first < len(s.spaces); {
		last := first + 1
		for last < len(s.spaces) && sameMachine(s.spaces[first].Machine, s.spaces[last].Machine) {
			last++
		}
		group := s.spaces[first:last]
		sort.SliceStable(group, func(i, j int) bool {
			a, haveA := rank[group[i].ID]
			b, haveB := rank[group[j].ID]
			if haveA != haveB {
				return haveA
			}
			return haveA && a < b
		})
		first = last
	}
}

// saveViewMu keeps document echoes and UI changes from writing an older
// snapshot over a newer one, or racing over the same temporary file.
var saveViewMu sync.Mutex

// SaveView writes this screen's selection, focus, zoom and dragged order.
func (s *Session) SaveView() {
	saveViewMu.Lock()
	defer saveViewMu.Unlock()
	s.mu.Lock()
	v := viewFile{Active: s.active, Spaces: map[string]string{}, Tabs: map[string]viewTab{},
		Machines: append([]string(nil), s.machineOrder...), SpaceOrder: append([]string(nil), s.spaceOrder...)}
	for id, tab := range s.activeTab {
		v.Spaces[id] = tab
	}
	for id, tv := range s.tabs {
		zoomed := tv.zoomed
		v.Tabs[id] = viewTab{Focused: tv.focused, Before: append([]string(nil), tv.before...), Zoomed: &zoomed}
	}
	s.mu.Unlock()
	text, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(AppHome(), 0o755); err != nil {
		return
	}
	path := filepath.Join(AppHome(), "view.json")
	tmp := path + ".tmp"
	if os.WriteFile(tmp, text, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}
