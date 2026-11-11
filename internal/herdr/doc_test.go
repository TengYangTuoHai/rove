package herdr

import (
	"encoding/json"
	"testing"
)

func TestDocumentEchoKeepsPendingPaneWeights(t *testing.T) {
	// Version zero is a valid initial document, not evidence it was loaded.
	raw := json.RawMessage(`{"version":0,"value":{"spaces":[{"id":"space","tabs":[{"id":"tab","root":{"id":"pane","type":"block","size":{"weight":0.5}}}]}]}}`)
	s := &Session{}
	s.applyDoc("", raw)
	check := func(want float64) {
		t.Helper()
		n := s.Tree("tab")
		if n == nil || n.Size == nil || n.Size.Weight == nil || *n.Size.Weight != want {
			t.Fatalf("pane weights differ from %v: %+v", want, n)
		}
	}
	check(0.5)
	if !s.SetWeights("tab", map[string]float64{"pane": 0.7}) {
		t.Fatal("no pane to resize")
	}
	s.docs[""].err = "temporary read failure"
	s.applyDoc("", raw)
	check(0.7)
	if s.docs[""].err != "" {
		t.Fatal("a successful equal-version read kept the previous error")
	}

	// Reconnecting drops the load state and must reload a known version.
	delete(s.docs, "")
	s.applyDoc("", raw)
	check(0.5)

	// A newer document still replaces the model. If it cannot be parsed,
	// another read of that version must get a chance to recover.
	s.applyDoc("", json.RawMessage(`{"version":1,"value":42}`))
	if s.docs[""].available {
		t.Fatal("an invalid document was marked available")
	}
	var newer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &newer); err != nil {
		t.Fatal(err)
	}
	newer["version"] = json.RawMessage(`1`)
	retry, err := json.Marshal(newer)
	if err != nil {
		t.Fatal(err)
	}
	if !s.SetWeights("tab", map[string]float64{"pane": 0.8}) {
		t.Fatal("no pane to resize before retry")
	}
	s.applyDoc("", retry)
	check(0.5)
	if !s.docs[""].available {
		t.Fatal("retry did not recover the document")
	}
}
