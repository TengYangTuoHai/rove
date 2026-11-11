package herdr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func checkSpaceOrder(t *testing.T, s *Session, want ...string) {
	t.Helper()
	var got []string
	for i, sp := range s.Places() {
		got = append(got, sp.ID)
		if sp.Index != i+1 {
			t.Fatalf("space %s has index %d, want %d", sp.ID, sp.Index, i+1)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("space order = %v, want %v", got, want)
	}
}

func applySpaceOrderDocument(t *testing.T, s *Session, machine string, version int, ids ...string) {
	t.Helper()
	var d docFile
	for _, id := range ids {
		d.Spaces = append(d.Spaces, docSpace{ID: id})
	}
	raw, err := json.Marshal(map[string]any{"version": version, "value": d})
	if err != nil {
		t.Fatal(err)
	}
	s.applyDoc(machine, raw)
}

func readSpaceOrderView(t *testing.T) viewFile {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(AppHome(), "view.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v viewFile
	if err := json.Unmarshal(text, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMoveSpaceSavesViewOrderImmediately(t *testing.T) {
	// A new ROVE_HOME must be created, and changed's one-second throttle must
	// not postpone the explicit drag's write.
	t.Setenv("ROVE_HOME", filepath.Join(t.TempDir(), "new", "rove"))
	s := &Session{spaces: []*Space{{ID: "a"}, {ID: "b"}, {ID: "c"}}, active: "b",
		activeTab: map[string]string{"b": "selected-tab"}, machineOrder: []string{"remote"}, savedAt: time.Now()}
	s.MoveSpace("c", 1)
	v := readSpaceOrderView(t)
	if !slices.Equal(v.SpaceOrder, []string{"c", "a", "b"}) {
		t.Fatalf("saved space_order = %v", v.SpaceOrder)
	}
	if v.Spaces["b"] != "selected-tab" || v.Active != "b" || !slices.Equal(v.Machines, []string{"remote"}) {
		t.Fatalf("space order displaced existing view state: %+v", v)
	}
	// Load a core document with the old order. The screen keeps its explicit
	// drag and restores the existing selection state independently.
	next := &Session{activeTab: map[string]string{}}
	applySpaceOrderDocument(t, next, "", 1, "a", "b", "c")
	next.LoadView()
	checkSpaceOrder(t, next, "c", "a", "b")
	if next.active != "b" || next.activeTab["b"] != "selected-tab" {
		t.Fatal("loading the dragged order lost the screen's selection")
	}
}

func TestSpaceOrderSurvivesDocumentRefresh(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	s := &Session{}
	applySpaceOrderDocument(t, s, "", 1, "a", "b", "c")
	s.MoveSpace("c", 1)
	// Another client can write a newer document in a different order. Keep
	// this window's drag, accept new spaces, and do not resurrect deleted IDs.
	applySpaceOrderDocument(t, s, "", 2, "b", "a", "new-1", "c", "new-2")
	checkSpaceOrder(t, s, "c", "a", "b", "new-1", "new-2")
	applySpaceOrderDocument(t, s, "", 3, "b", "new-2", "a", "new-1")
	checkSpaceOrder(t, s, "a", "b", "new-2", "new-1")
	// Until the user drags it, a new space follows the document's order.
	// A known space that comes back still has its saved position.
	applySpaceOrderDocument(t, s, "", 4, "new-1", "b", "c", "a", "new-2")
	checkSpaceOrder(t, s, "c", "a", "b", "new-1", "new-2")
}

func TestSpaceOrderRestoresDelayedRemoteDocuments(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	remoteOrder := []string{"remote/c", "remote/b", "remote/a", "offline/y", "offline/x"}
	previous := &Session{spaceOrder: remoteOrder}
	previous.SaveView()
	s := &Session{}
	applySpaceOrderDocument(t, s, "", 1, "local-a", "local-b")
	s.LoadView()
	// A local drag while remote machines are absent must retain their IDs.
	s.MoveSpace("local-b", 1)
	wantSaved := append(append([]string(nil), remoteOrder...), "local-b", "local-a")
	if got := readSpaceOrderView(t).SpaceOrder; !slices.Equal(got, wantSaved) {
		t.Fatalf("local drag discarded remote order: %v, want %v", got, wantSaved)
	}
	applySpaceOrderDocument(t, s, "remote", 1, "a", "b", "c")
	applySpaceOrderDocument(t, s, "offline", 1, "x", "y")
	checkSpaceOrder(t, s, "local-b", "local-a", "offline/y", "offline/x", "remote/c", "remote/b", "remote/a")
	// A partially loaded group keeps the missing ID in its saved slot when
	// dragged, as well as the untouched order of every other machine.
	applySpaceOrderDocument(t, s, "remote", 2, "a", "c", "new")
	s.MoveSpace("remote/a", 5)
	checkSpaceOrder(t, s, "local-b", "local-a", "offline/y", "offline/x", "remote/a", "remote/c", "remote/new")
	wantSaved = []string{"remote/a", "remote/b", "remote/c", "offline/y", "offline/x", "local-b", "local-a", "remote/new"}
	if got := readSpaceOrderView(t).SpaceOrder; !slices.Equal(got, wantSaved) {
		t.Fatalf("remote drag changed absent/other-machine IDs: %v, want %v", got, wantSaved)
	}
	applySpaceOrderDocument(t, s, "remote", 3, "new", "c", "b", "a")
	checkSpaceOrder(t, s, "local-b", "local-a", "offline/y", "offline/x", "remote/a", "remote/b", "remote/c", "remote/new")
}

func TestSpaceOrderWithoutSavedDragUsesDocumentOrder(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	s := &Session{}
	s.SaveView()
	applySpaceOrderDocument(t, s, "", 1, "c", "a", "b")
	s.LoadView()
	checkSpaceOrder(t, s, "c", "a", "b")
	applySpaceOrderDocument(t, s, "", 2, "b", "c", "a")
	checkSpaceOrder(t, s, "b", "c", "a")
}
