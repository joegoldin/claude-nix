// Package compaction maintains a per-session counter of /compact resets,
// detected as drops greater than dropThresholdPct in context_window.used_percentage
// between consecutive observations.
package compaction

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const dropThresholdPct = 10.0

type Store struct {
	Dir string
}

type sessionState struct {
	LastPct float64 `json:"last_pct"`
	Count   int     `json:"count"`
}

// Track records the current context percentage for sessionID and returns the
// running compaction count.
func (s *Store) Track(sessionID string, currentPct float64) (int, error) {
	path := s.pathFor(sessionID)
	state, existed, err := s.load(path)
	if err != nil {
		return 0, err
	}
	if existed && state.LastPct-currentPct > dropThresholdPct {
		state.Count++
	}
	state.LastPct = currentPct
	if err := s.save(path, state); err != nil {
		return state.Count, err
	}
	return state.Count, nil
}

func (s *Store) pathFor(sessionID string) string {
	return filepath.Join(s.Dir, sessionID+".json")
}

func (s *Store) load(path string) (sessionState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sessionState{}, false, nil
		}
		return sessionState{}, false, err
	}
	var st sessionState
	if err := json.Unmarshal(data, &st); err != nil {
		return sessionState{}, false, nil
	}
	return st, true, nil
}

func (s *Store) save(path string, state sessionState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
