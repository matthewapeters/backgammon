package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreCreatesMissingFile(t *testing.T) {
	s := &PersonaStore{Path: filepath.Join(t.TempDir(), "sub", "personas.json")}
	ps, err := s.Load()
	if err != nil || len(ps) != 0 {
		t.Fatalf("Load = %v, %v; want empty", ps, err)
	}
	if _, err := os.Stat(s.Path); err != nil {
		t.Fatalf("store file not created: %v", err)
	}
}

func TestStoreAddRoundTrips(t *testing.T) {
	s := &PersonaStore{Path: filepath.Join(t.TempDir(), "personas.json")}
	a := Persona{Name: "Grumpy Cat", Voice: "You are a grumpy cat.", Fallback: []string{"No."}, Prompt: "grumpy cat"}
	b := Persona{Name: "Robot", Voice: "You are a robot."}
	if err := s.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(b); err != nil {
		t.Fatal(err)
	}
	ps, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].Name != a.Name || ps[0].Prompt != a.Prompt || ps[0].Fallback[0] != "No." || ps[1].Name != b.Name {
		t.Fatalf("got %+v", ps)
	}
}

func TestStoreAddKeepsUnreadableFile(t *testing.T) {
	s := &PersonaStore{Path: filepath.Join(t.TempDir(), "personas.json")}
	if err := os.WriteFile(s.Path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Persona{Name: "X", Voice: "Y"}); err == nil {
		t.Fatal("Add succeeded on a corrupt store")
	}
	if data, _ := os.ReadFile(s.Path); string(data) != "not json" {
		t.Fatalf("corrupt store was overwritten: %q", data)
	}
}
