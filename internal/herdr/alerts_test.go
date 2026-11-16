package herdr

import (
	"encoding/json"
	"testing"
	"time"
)

func alertSession() *Session {
	s := sessionWithOnePane(`{"address":"t10","state":"live","status":{"app":"pi","state":"working"}}`)
	s.activeTab = map[string]string{"sp1": "tb1"}
	s.git, s.gitBusy = map[string]gitInfo{}, map[string]bool{}
	s.savedAt = time.Now()
	s.reindex()
	return s
}

func reportAgentState(s *Session, state string) {
	line, _ := json.Marshal(map[string]any{
		"method": "event",
		"params": map[string]any{"type": "status", "terminal": "t10", "data": map[string]any{
			"status": map[string]string{"app": "pi", "state": state, "title": "Agent session"},
		}},
	})
	s.handleLine(line)
}

func TestAgentAlertsFollowBackgroundTransitions(t *testing.T) {
	s := alertSession()
	s.spaces = append(s.spaces, &Space{ID: "elsewhere"})
	s.active = "elsewhere"
	s.reindex()
	reportAgentState(s, "blocked")
	reportAgentState(s, "blocked")
	reportAgentState(s, "working")
	reportAgentState(s, "done")
	reportAgentState(s, "done")
	alerts := s.TakeAgentAlerts()
	if len(alerts) != 2 || alerts[0].Status != "blocked" || alerts[1].Status != "done" {
		t.Fatalf("background transitions = %+v", alerts)
	}
	if alerts[1].Title != "Agent session" || alerts[1].Program.Key != "pi" {
		t.Fatalf("alert lost its agent context: %+v", alerts[1])
	}
	if again := s.TakeAgentAlerts(); len(again) != 0 {
		t.Fatalf("drained alerts repeated: %+v", again)
	}
	// An acknowledged agent still alerts on a later completed turn, even
	// though the sidebar presents seen completions as idle.
	s.seen = map[string]int{"t10": 0}
	reportAgentState(s, "working")
	reportAgentState(s, "done")
	if got := s.TakeAgentAlerts(); len(got) != 1 || got[0].Status != "done" {
		t.Fatalf("seen agent's next completion was lost: %+v", got)
	}
	// The UI may not have rendered between working and done.
	reportAgentState(s, "working")
	reportAgentState(s, "done")
	if got := s.TakeAgentAlerts(); len(got) != 1 {
		t.Fatalf("next completion was lost between frames: %+v", got)
	}
}

func TestAgentAlertsIgnoreInitialAndVisibleStates(t *testing.T) {
	s := alertSession()
	reportAgentState(s, "blocked")
	reportAgentState(s, "done")
	if got := s.TakeAgentAlerts(); len(got) != 0 {
		t.Fatalf("visible agent alerted: %+v", got)
	}
	s.spaces = append(s.spaces, &Space{ID: "elsewhere"})
	s.active = "elsewhere"
	s.reindex()
	if got := s.TakeAgentAlerts(); len(got) != 0 {
		t.Fatalf("navigation replayed an old completion: %+v", got)
	}
	// A launch with an already completed agent establishes a baseline.
	s.agents = nil
	s.alertsReady = false
	s.reindex()
	if got := s.TakeAgentAlerts(); len(got) != 0 {
		t.Fatalf("initial snapshot alerted: %+v", got)
	}
}
