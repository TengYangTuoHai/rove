package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rove/internal/core"
)

// RecoverPane starts a new shell in a pane whose original terminal is finished.
// expectedTerminal must be the terminal the user chose to replace. It returns
// the new address on success. If a document write has an uncertain outcome,
// the address is returned with an error: it may already be in the document, so
// callers must not kill it. Further attempts for the same pane and terminal
// are blocked in this session until its reference changes.
func (s *Session) RecoverPane(paneID, expectedTerminal string) (string, error) {
	if s.Core == nil {
		return "", errNoCore
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ops := recoverOps{
		call: s.Core.Call,
		run:  s.runTerminal,
		kill: func(addr string) { s.KillTerminal(addr) },
	}
	return s.recoverPane(ctx, paneID, expectedTerminal, ops)
}

type recoveryUnknown struct {
	expected string
	address  string
	machine  string
	write    bool
}

type recoverOps struct {
	call func(context.Context, string, any) (json.RawMessage, error)
	run  func([]string, string, map[string]string, string) (string, error)
	kill func(string)
}

// recoverPane serializes against this process's ordinary document writes.
// The core's document version protects against writers in other processes.
func (s *Session) recoverPane(ctx context.Context, paneID, expected string, ops recoverOps) (string, error) {
	if paneID == "" || expected == "" {
		return "", errors.New("pane and expected terminal are required")
	}
	writeMu.Lock()
	defer writeMu.Unlock()

	s.mu.Lock()
	sp, tab, node := s.recoveryPaneLocked(paneID)
	if node == nil || node.terminal() != expected || s.query[paneID] != "" {
		s.mu.Unlock()
		return "", errors.New("pane is gone, changed, or showing another terminal")
	}
	if pending, ok := s.recoverUnknown[paneID]; ok {
		if pending.expected == expected {
			s.mu.Unlock()
			return pending.address, errors.New("previous recovery outcome is unknown; refresh the pane before trying again")
		}
		delete(s.recoverUnknown, paneID)
	}
	spaceID, tabID, on, folder := sp.ID, tab.ID, machineOf(sp), sp.Folder
	if TerminalMachine(expected) != on {
		s.mu.Unlock()
		return "", errors.New("pane and terminal are on different machines")
	}
	s.mu.Unlock()

	params := map[string]any{"name": "herdr-app"}
	if on != "" {
		params["on"] = on
	}
	raw, err := ops.call(ctx, "doc.get", params)
	if err != nil {
		return "", fmt.Errorf("read pane document: %w", err)
	}
	var doc struct {
		Value   json.RawMessage `json:"value"`
		Version uint64          `json:"version"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("read pane document: %w", err)
	}
	// Validate the document before creating anything. Preserve opaque fields.
	value, err := recoveryDoc(doc.Value, on, spaceID, tabID, paneID, expected, "")
	if err != nil {
		return "", err
	}
	_ = value
	term, err := recoveryTerminal(ctx, ops, expected)
	if err != nil {
		return "", err
	}
	if term.Foreground != nil && term.Foreground.Cwd != "" {
		folder = term.Foreground.Cwd
	} else if term.Cwd != "" {
		folder = term.Cwd
	}
	markUnknown := func(address string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.recoverUnknown == nil {
			s.recoverUnknown = map[string]recoveryUnknown{}
		}
		s.recoverUnknown[paneID] = recoveryUnknown{expected: expected, address: address}
	}
	addr, err := ops.run(nil, folder, labelsFor(spaceID, tabID), on)
	if err != nil {
		var rpcErr *core.Error
		if errors.As(err, &rpcErr) && (rpcErr.Kind() == "invalid_params" || rpcErr.Kind() == "spawn_failed" || rpcErr.Kind() == "invalid_template" || rpcErr.Kind() == "name_taken" || rpcErr.Kind() == "config") {
			return "", fmt.Errorf("start replacement shell: %w", err)
		}
		markUnknown("")
		return "", fmt.Errorf("start replacement shell outcome unknown; refresh before retrying: %w", err)
	}
	if addr == "" || addr == expected || TerminalMachine(addr) != on {
		// Never kill an address we cannot confidently identify as the new shell.
		markUnknown(addr)
		return addr, errors.New("core returned an invalid replacement address; recovery outcome is unknown")
	}
	cleanup := func(err error) (string, error) {
		ops.kill(addr)
		return "", err
	}
	value, err = recoveryDoc(doc.Value, on, spaceID, tabID, paneID, expected, addr)
	if err != nil {
		return cleanup(err)
	}
	// A terminal can have been restarted since the first lookup.
	if _, err := recoveryTerminal(ctx, ops, expected); err != nil {
		return cleanup(err)
	}

	// Keep local pane mutations from racing the check and the commit. Other
	// machines' document writers are guarded by doc.set's version check.
	s.mu.Lock()
	sp2, tab2, node2 := s.recoveryPaneLocked(paneID)
	if node2 == nil || sp2.ID != spaceID || tab2.ID != tabID || machineOf(sp2) != on || node2.terminal() != expected || s.query[paneID] != "" {
		s.mu.Unlock()
		return cleanup(errors.New("pane changed while starting the shell"))
	}
	set := map[string]any{"name": "herdr-app", "value": json.RawMessage(value), "version": doc.Version}
	if on != "" {
		set["on"] = on
	}
	// Bound the time a slow core can hold the model lock. A timeout is an
	// uncertain write, not evidence that the core did not commit it.
	writeCtx, writeCancel := context.WithTimeout(ctx, 3*time.Second)
	result, err := ops.call(writeCtx, "doc.set", set)
	writeCancel()
	var saved struct {
		Version uint64 `json:"version"`
	}
	if err != nil {
		var rpcErr *core.Error
		if errors.As(err, &rpcErr) && (rpcErr.Kind() == "conflict" || rpcErr.Kind() == "invalid_params") {
			s.mu.Unlock()
			return cleanup(fmt.Errorf("save recovered pane: %w", err))
		}
	} else if json.Unmarshal(result, &saved) != nil || saved.Version <= doc.Version {
		err = errors.New("invalid document write response")
	}
	if err != nil {
		// Block stale document writes before releasing the model lock.
		if s.recoverUnknown == nil {
			s.recoverUnknown = map[string]recoveryUnknown{}
		}
		s.recoverUnknown[paneID] = recoveryUnknown{expected: expected, address: addr, machine: on, write: true}
		s.mu.Unlock()
		// A lost response may still have committed. An old document is
		// inconclusive: only our exact new address confirms success.
		checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		checkRaw, checkErr := ops.call(checkCtx, "doc.get", params)
		cancel()
		var check struct {
			Value   json.RawMessage `json:"value"`
			Version uint64          `json:"version"`
		}
		if checkErr == nil && json.Unmarshal(checkRaw, &check) == nil && check.Version > doc.Version {
			_, checkErr = recoveryDoc(check.Value, on, spaceID, tabID, paneID, addr, "")
			if checkErr == nil {
				s.mu.Lock()
				sp2, tab2, node2 = s.recoveryPaneLocked(paneID)
				if node2 != nil && sp2.ID == spaceID && tab2.ID == tabID && machineOf(sp2) == on && node2.terminal() == expected && s.query[paneID] == "" {
					content := *node2.Content
					content.Terminal = addr
					node2.Content = &content
				}
				if s.versions == nil {
					s.versions = map[string]uint64{}
				}
				if s.versions[on] < check.Version {
					s.versions[on] = check.Version
				}
				delete(s.recoverUnknown, paneID)
				s.mu.Unlock()
				s.changed()
				return addr, nil
			}
		}
		return addr, fmt.Errorf("save recovered pane outcome unknown; refresh before retrying: %w", err)
	}
	// Copy content so a previously returned tree cannot observe a mutation.
	content := *node2.Content
	content.Terminal = addr
	node2.Content = &content
	if s.versions == nil {
		s.versions = map[string]uint64{}
	}
	s.versions[on] = saved.Version
	s.mu.Unlock()
	s.changed()
	return addr, nil
}

func (s *Session) recoveryPaneLocked(id string) (*Space, *Tab, *Node) {
	for _, sp := range s.spaces {
		for _, tab := range sp.Tabs {
			if tab.Root == nil {
				continue
			}
			if n := tab.Root.find(id); n != nil && n.Type == "block" && n.Content != nil && n.Content.Terminal != "" {
				return sp, tab, n
			}
		}
	}
	return nil, nil, nil
}

func recoveryTerminal(ctx context.Context, ops recoverOps, address string) (*Terminal, error) {
	raw, err := ops.call(ctx, "terminal.get", map[string]any{"terminal": address})
	if err != nil {
		return nil, fmt.Errorf("check terminal state: %w", err)
	}
	var term Terminal
	if err := json.Unmarshal(raw, &term); err != nil || term.Address != address || term.State != "finished" {
		return nil, errors.New("terminal is not confirmed finished")
	}
	return &term, nil
}

// recoveryDoc changes only the matching terminal reference in the versioned,
// opaque document. IDs in a machine's document do not have its prefix.
func recoveryDoc(raw json.RawMessage, on, spaceID, tabID, paneID, expected, replacement string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return nil, errors.New("unreadable pane document")
	}
	tag := machineTag(on)
	spaceID, tabID, paneID, expected = localID(spaceID, tag), localID(tabID, tag), localID(paneID, tag), localID(expected, tag)
	spaces, ok := doc["spaces"].([]any)
	if !ok {
		return nil, errors.New("pane document has no spaces")
	}
	var matches int
	var visit func(any)
	visit = func(v any) {
		n, ok := v.(map[string]any)
		if !ok {
			return
		}
		if n["id"] == paneID && n["type"] == "block" {
			if content, ok := n["content"].(map[string]any); ok && content["terminal"] == expected {
				matches++
				if replacement != "" {
					content["terminal"] = localID(replacement, tag)
				}
			}
		}
		if children, ok := n["children"].([]any); ok {
			for _, child := range children {
				visit(child)
			}
		}
	}
	for _, item := range spaces {
		sp, ok := item.(map[string]any)
		if !ok || sp["id"] != spaceID {
			continue
		}
		tabs, _ := sp["tabs"].([]any)
		for _, item := range tabs {
			tab, ok := item.(map[string]any)
			if ok && tab["id"] == tabID {
				visit(tab["root"])
			}
		}
	}
	if matches != 1 {
		return nil, errors.New("pane document changed or pane is gone")
	}
	return json.Marshal(doc)
}
