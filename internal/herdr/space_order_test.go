package herdr

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestMoveSpaceDocumentRoundTrip(t *testing.T) {
	for _, machine := range []string{"", "remote"} {
		for _, tc := range []struct {
			name string
			id   string
			to   int
			want []string
		}{
			{"first to last", "a", 3, []string{"b", "c", "a"}},
			{"last to first", "c", 1, []string{"c", "a", "b"}},
			{"first to middle", "a", 2, []string{"b", "a", "c"}},
			{"last to middle", "c", 2, []string{"a", "c", "b"}},
			{"middle to first", "b", 1, []string{"b", "a", "c"}},
			{"middle to last", "b", 3, []string{"a", "c", "b"}},
		} {
			t.Run(machine+"/"+tc.name, func(t *testing.T) {
				t.Setenv("ROVE_HOME", t.TempDir())
				tag := machineTag(machine)
				s := &Session{}
				if machine != "" {
					s.spaces = append(s.spaces, &Space{ID: "local"})
				}
				for _, id := range []string{"a", "b", "c"} {
					s.spaces = append(s.spaces, &Space{ID: qualify(id, tag), Machine: tag})
				}
				s.spaces = append(s.spaces, &Space{ID: "z/other", Machine: machineTag("z")})
				offset := 0
				if machine != "" {
					offset = 1
				}
				s.MoveSpace(qualify(tc.id, tag), offset+tc.to)
				// Grouping after a document refresh must retain the dragged order.
				orderSpaces(s.spaces)
				value, err := json.Marshal(captured(s.spaces, tag))
				if err != nil {
					t.Fatal(err)
				}
				var saved docFile
				if err := json.Unmarshal(value, &saved); err != nil {
					t.Fatal(err)
				}
				var ids []string
				for _, sp := range saved.Spaces {
					ids = append(ids, sp.ID)
				}
				if !slices.Equal(ids, tc.want) {
					t.Fatalf("saved order = %v, want %v", ids, tc.want)
				}
				next := &Session{spaces: []*Space{{ID: "z/other", Machine: machineTag("z")}}}
				raw, err := json.Marshal(map[string]any{"version": 1, "value": saved})
				if err != nil {
					t.Fatal(err)
				}
				next.applyDoc(machine, raw)
				ids = nil
				for _, sp := range next.Places() {
					if sameMachine(sp.Machine, tag) {
						ids = append(ids, localID(sp.ID, tag))
					}
				}
				if !slices.Equal(ids, tc.want) {
					t.Fatalf("reloaded order = %v, want %v", ids, tc.want)
				}
			})
		}
	}
}
