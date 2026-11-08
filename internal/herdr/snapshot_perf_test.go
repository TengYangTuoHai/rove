package herdr

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Guard the fast comparison when fields are added to the terminal model.
func TestSameTerminalChecksEveryField(t *testing.T) {
	typ := reflect.TypeOf(Terminal{})
	for i := 0; i < typ.NumField(); i++ {
		t.Run(typ.Field(i).Name, func(t *testing.T) {
			var a, b Terminal
			field := reflect.ValueOf(&b).Elem().Field(i)
			switch field.Kind() {
			case reflect.String:
				field.SetString("changed")
			case reflect.Int:
				field.SetInt(1)
			case reflect.Bool:
				field.SetBool(true)
			case reflect.Pointer:
				field.Set(reflect.New(field.Type().Elem()))
			case reflect.Slice:
				field.Set(reflect.MakeSlice(field.Type(), 1, 1))
			case reflect.Map:
				field.Set(reflect.MakeMap(field.Type()))
				field.SetMapIndex(reflect.ValueOf("key"), reflect.ValueOf("value"))
			default:
				t.Fatalf("add a test value for %v", field.Type())
			}
			if sameTerminal(&a, &b) {
				t.Fatal("field difference was ignored")
			}
		})
	}
	p, q := 7, 7
	a := &Terminal{Status: &Status{State: "working", Progress: &p}, Foreground: &Foreground{Argv: []string{"shell"}}}
	b := &Terminal{Status: &Status{State: "working", Progress: &q}, Foreground: &Foreground{Argv: []string{"shell"}}}
	if !sameTerminal(a, b) {
		t.Fatal("equal nested values should compare equal")
	}
	q++
	if sameTerminal(a, b) {
		t.Fatal("status progress difference was ignored")
	}
}

func TestSnapshotChangeDetection(t *testing.T) {
	s := &Session{}
	first := json.RawMessage(`{"terminals":[{"address":"t1","title":"first","labels":{"x":"y"}}]}`)
	if !s.applySnapshot(first) {
		t.Fatal("first snapshot should change the model")
	}
	if s.applySnapshot(first) {
		t.Fatal("identical snapshot should not change the model")
	}
	if !s.applyEvent("title", "t1", json.RawMessage(`{"title":"updated"}`)) {
		t.Fatal("title event was not applied")
	}
	if !s.applySnapshot(first) {
		t.Fatal("snapshot must restore a model changed by an event")
	}
	if !s.applySnapshot(json.RawMessage(`{"terminals":[]}`)) {
		t.Fatal("removal should change the model")
	}
	if s.applySnapshot(json.RawMessage(`{"terminals":[]}`)) {
		t.Fatal("empty snapshot should be stable")
	}
	if s.applySnapshot(json.RawMessage(`{`)) {
		t.Fatal("invalid snapshot should not change the model")
	}
}
