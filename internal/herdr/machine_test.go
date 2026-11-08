package herdr

import (
	"strings"
	"testing"
)

func machineNames(s *Session) string {
	var out []string
	for _, m := range s.Machines() {
		out = append(out, m.Name)
	}
	return strings.Join(out, ",")
}

func TestMachinesFollowDraggedOrder(t *testing.T) {
	s := &Session{
		machines:     []*Machine{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}},
		machineOrder: []string{"gamma", "alpha"},
	}
	// gamma was dragged first; beta was never dragged and stays last.
	if got := machineNames(s); got != "gamma,alpha,beta" {
		t.Fatalf("dragged order: %q", got)
	}
}

func TestMoveMachine(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	s := &Session{machines: []*Machine{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}}
	s.MoveMachine("gamma", 1)
	if got := machineNames(s); got != "gamma,alpha,beta" {
		t.Fatalf("moved to the top: %q", got)
	}
	s.MoveMachine("gamma", 3)
	if got := machineNames(s); got != "alpha,beta,gamma" {
		t.Fatalf("moved to the end: %q", got)
	}
	// The order is kept for the next run, in view.json.
	s.MoveMachine("beta", 1)
	next := &Session{machines: []*Machine{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}}
	next.LoadView()
	if got := machineNames(next); got != "beta,alpha,gamma" {
		t.Fatalf("kept order: %q", got)
	}
}
