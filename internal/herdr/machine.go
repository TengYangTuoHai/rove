package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rove/internal/core"
)

// Machine is one core this window reaches over ssh (hc machine ls).
type Machine struct {
	Name      string  `json:"name"`
	Target    string  `json:"target"`
	ID        string  `json:"id"`
	Host      string  `json:"host"`
	Version   string  `json:"version"`
	State     string  `json:"state"` // connecting, connected, needs_login, unreachable
	Session   *string `json:"session"`
	Temporary bool    `json:"temporary"`
	Via       bool    `json:"via"`
	SinceMs   *uint64 `json:"since_ms"`
	Why       *string `json:"why"`
}

// Connected reports whether the machine answers now.
func (m *Machine) Connected() bool { return m.State == "connected" }

// RefreshMachines reads machine.list again, on the caller's goroutine.
func (s *Session) RefreshMachines() { s.refreshMachines() }

// Machines returns the machines this core knows, this computer not among
// them, in the order the person dragged their groups into.
func (s *Session) Machines() []*Machine {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Machine, 0, len(s.machines))
	for _, m := range s.machines {
		cp := *m
		out = append(out, &cp)
	}
	rank := make(map[string]int, len(s.machineOrder))
	for i, name := range s.machineOrder {
		rank[name] = i
	}
	// Machines nobody dragged rank the same, so they keep machine.list's
	// order after the ones that were dragged.
	sort.SliceStable(out, func(i, j int) bool {
		return machineRank(rank, out[i].Name) < machineRank(rank, out[j].Name)
	})
	return out
}

// machineRank is where a machine sits among the dragged ones; machines
// never dragged rank after all of them.
func machineRank(rank map[string]int, name string) int {
	if at, ok := rank[name]; ok {
		return at
	}
	return len(rank) + 1
}

// MoveMachine puts one machine's group at a place (1 = first remote
// machine), as dragging its header does.
func (s *Session) MoveMachine(name string, index int) {
	machines := s.Machines()
	names := make([]string, 0, len(machines))
	for _, m := range machines {
		names = append(names, m.Name)
	}
	from := -1
	for i, n := range names {
		if n == name {
			from = i
		}
	}
	if from < 0 {
		return
	}
	names = append(names[:from], names[from+1:]...)
	at := clamp(index-1, 0, len(names))
	names = append(names[:at], append([]string{name}, names[at:]...)...)
	s.mu.Lock()
	s.machineOrder = names
	s.mu.Unlock()
	s.SaveView()
	s.changed()
}

// MachineNamed returns one machine, nil when unknown.
func (s *Session) MachineNamed(name string) *Machine {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.machineByName[name]; m != nil {
		cp := *m
		return &cp
	}
	return nil
}

// refreshMachines reads machine.list and catches documents up with it.
func (s *Session) refreshMachines() {
	if s.Core == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := s.Core.Call(ctx, "machine.list", map[string]any{})
	if err != nil {
		return
	}
	s.adoptMachines(raw, false)
}

// adoptMachines takes a machine.list reply: the machines stay, their
// documents are read once they answer.
func (s *Session) adoptMachines(raw json.RawMessage, initial bool) {
	var res struct {
		Machines []*Machine `json:"machines"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return
	}
	s.mu.Lock()
	before := s.machineByName
	s.machines = nil
	s.machineByName = map[string]*Machine{}
	var want []string
	var sig strings.Builder
	for _, m := range res.Machines {
		if m == nil || m.Temporary {
			continue
		}
		s.machines = append(s.machines, m)
		s.machineByName[m.Name] = m
		fmt.Fprintf(&sig, "%s|%s|%s|", m.Name, m.State, m.Version)
		if m.State == "connected" {
			was := before[m.Name]
			if was == nil || was.State != "connected" {
				// It may have changed while away: read its document again.
				delete(s.docs, m.Name)
			}
			want = append(want, m.Name)
		}
	}
	changed := sig.String() != s.machineSig
	s.machineSig = sig.String()
	sort.SliceStable(s.machines, func(i, j int) bool { return s.machines[i].Name < s.machines[j].Name })
	gone := map[string]bool{}
	for name, slot := range s.docs {
		if name != "" && s.machineByName[name] == nil && slot.loaded {
			gone[name] = true
		}
	}
	s.mu.Unlock()
	for name := range gone {
		s.dropMachine(name)
	}
	for _, name := range want {
		s.mu.Lock()
		slot := s.docs[name]
		loaded := slot != nil && slot.loaded
		home := s.homes[name]
		s.mu.Unlock()
		if !loaded {
			s.readDoc(name)
		}
		if home == "" {
			go s.readHome(name)
		}
	}
	if !initial && changed {
		s.changed()
	}
}

// AddMachine adds a machine reached the way ssh reaches it (a Host from
// ~/.ssh/config, or user@host). A machine that needs herdr-core installed or
// a password first cannot be added here: the caller starts its own terminal.
func (s *Session) AddMachine(target, name string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("a machine needs how ssh reaches it")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	params := map[string]any{"target": target}
	if name != "" {
		params["name"] = name
	}
	if _, err := s.Core.Call(ctx, "machine.add", params); err != nil {
		return err
	}
	s.refreshMachines()
	return nil
}

// RemoveMachine forgets a machine and its terminals.
func (s *Session) RemoveMachine(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.Core.Call(ctx, "machine.rm", map[string]any{"machine": name}); err != nil {
		return err
	}
	s.refreshMachines()
	return nil
}

// ConnectMachine asks a machine that is down to come back.
func (s *Session) ConnectMachine(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.Core.Call(ctx, "machine.connect", map[string]any{"machine": name}); err != nil {
		return err
	}
	s.refreshMachines()
	return nil
}

// RenameMachine renames a machine here; addresses with the old name stop.
func (s *Session) RenameMachine(name, newName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.Core.Call(ctx, "machine.rename", map[string]any{"machine": name, "new_name": newName}); err != nil {
		return err
	}
	s.dropMachine(name)
	s.refreshMachines()
	return nil
}

// readHome learns a machine's home folder, so a space there can say "~".
func (s *Session) readHome(name string) {
	res, err := s.Exec(name, "", []string{"sh", "-c", "printf %s \"$HOME\""}, 15*time.Second)
	if err != nil || res.Stdout == "" {
		return
	}
	s.mu.Lock()
	s.homes[name] = strings.TrimSpace(res.Stdout)
	s.mu.Unlock()
	s.changed()
}

// dropMachine forgets a machine's spaces and what the views kept of them.
func (s *Session) dropMachine(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []*Space
	for _, sp := range s.spaces {
		if sp.Machine != nil && *sp.Machine == name {
			for _, t := range sp.Tabs {
				delete(s.tabs, t.ID)
				delete(s.activeTab, sp.ID)
			}
			if s.active == sp.ID {
				s.active = ""
			}
			if s.wantActive == sp.ID {
				s.wantActive = ""
			}
			continue
		}
		keep = append(keep, sp)
	}
	s.spaces = keep
	delete(s.docs, name)
	delete(s.versions, name)
}

// RunOnMachine runs a program on a machine and returns its terminal, for
// the add-a-machine terminal (hc machine add inside a shell).
func (s *Session) RunOnMachine(argv []string, on string) (string, error) {
	labels := map[string]string{"herdr.run": "machine"}
	return s.runTerminal(argv, "", labels, on)
}

// AddMachineInTerminal starts `hc machine add <target>` in a terminal so the
// person can answer what only they can (a password, a host key, an install).
func (s *Session) AddMachineInTerminal(target string) (string, error) {
	name := "add-" + strings.ToLower(strings.NewReplacer("@", "-", ":", "-", ".", "-").Replace(target))
	if len(name) > 32 {
		name = name[:32]
	}
	argv := []string{s.Core.Binary(), "machine", "add", target}
	return s.runTerminal(argv, "", map[string]string{"herdr.run": "machine", "herdr.name": name}, "")
}

// NeedsTerminal reports whether an add error is something only a person can
// answer: a missing core there, a password, a host key, a login.
func NeedsTerminal(err error) bool {
	if err == nil {
		return false
	}
	var rpcErr *core.Error
	if errors.As(err, &rpcErr) {
		switch rpcErr.Kind() {
		case "hc_missing", "incompatible", "login_needed", "auth_failed", "host_key", "unreachable":
			if rpcErr.Kind() != "unreachable" {
				return true
			}
		}
	}
	msg := strings.ToLower(err.Error())
	for _, word := range []string{"password", "passphrase", "host key", "permission denied", "login", "no hc", "not installed", "timed out"} {
		if strings.Contains(msg, word) {
			return true
		}
	}
	return false
}

// MachineReady reports whether a machine's document has been read (this
// computer's always has).
func (s *Session) MachineReady(name string) bool {
	if name == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.docs[name]
	return slot != nil && slot.loaded
}
