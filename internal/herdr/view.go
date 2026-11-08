package herdr

import (
	"encoding/json"
	"os"
	"path/filepath"
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
// each tab's focus and the order the machine groups were dragged into.
type viewFile struct {
	Active   string                    `json:"active"`
	Spaces   map[string]string         `json:"spaces"`
	Tabs     map[string]viewTab        `json:"tabs"`
	Machines []string                  `json:"machines,omitempty"`
	Recent   []map[string]string       `json:"recent,omitempty"`
	Seen     map[string]map[string]any `json:"attention,omitempty"`
}

type viewTab struct {
	Focused string   `json:"focused"`
	Before  []string `json:"before"`
	Zoomed  *string  `json:"zoomed"`
	Settled *uint64  `json:"settled"`
}

// LoadView restores this screen's selection, focus and zoom from view.json.
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

// SaveView writes this screen's selection, focus and zoom, as Herdr does.
func (s *Session) SaveView() {
	s.mu.Lock()
	v := viewFile{Active: s.active, Spaces: map[string]string{}, Tabs: map[string]viewTab{}, Machines: s.machineOrder}
	for id, tab := range s.activeTab {
		v.Spaces[id] = tab
	}
	for id, tv := range s.tabs {
		zoomed := tv.zoomed
		v.Tabs[id] = viewTab{Focused: tv.focused, Before: tv.before, Zoomed: &zoomed}
	}
	s.mu.Unlock()
	text, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(AppHome(), "view.json")
	tmp := path + ".tmp"
	if os.WriteFile(tmp, text, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}
