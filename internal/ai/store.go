package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// storeVersion is the current custom persona file format.
const storeVersion = 1

type storeFile struct {
	Version  int       `json:"version"`
	Personas []Persona `json:"personas"`
}

// PersonaStore keeps player-created personas in a JSON file. The built-in
// Personas are never written to it.
type PersonaStore struct {
	Path string
}

// DefaultPersonaPath is personas.json in the user's config directory.
func DefaultPersonaPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backgammon", "personas.json"), nil
}

// Load returns the stored personas, creating an empty store if the file is missing.
func (s *PersonaStore) Load() ([]Persona, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, s.save(nil)
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.Path, err)
	}
	if f.Version > storeVersion {
		return nil, fmt.Errorf("%s is format version %d; this build reads up to %d", s.Path, f.Version, storeVersion)
	}
	return f.Personas, nil
}

// Add appends p to the store. It rereads the file first so it never
// clobbers a store it could not parse.
func (s *PersonaStore) Add(p Persona) error {
	ps, err := s.Load()
	if err != nil {
		return err
	}
	return s.save(append(ps, p))
}

func (s *PersonaStore) save(ps []Persona) error {
	if ps == nil {
		ps = []Persona{}
	}
	data, err := json.MarshalIndent(storeFile{Version: storeVersion, Personas: ps}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	// Write and rename so a crash never leaves a half-written store.
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
