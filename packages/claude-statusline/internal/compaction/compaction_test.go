package compaction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackFirstObservation(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	n, err := s.Track("session-a", 47.0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("first observation should be 0, got %d", n)
	}
}

func TestTrackIncrementsOnLargeDrop(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	_, _ = s.Track("s", 80.0)
	n, _ := s.Track("s", 10.0)
	if n != 1 {
		t.Errorf("count after drop = %d, want 1", n)
	}
	_, _ = s.Track("s", 30.0)
	if n, _ := s.Track("s", 5.0); n != 2 {
		t.Errorf("count after second drop = %d, want 2", n)
	}
}

func TestTrackIgnoresSmallDrops(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	_, _ = s.Track("s", 50.0)
	n, _ := s.Track("s", 45.0)
	if n != 0 {
		t.Errorf("small drop counted: %d", n)
	}
}

func TestStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	s1 := &Store{Dir: dir}
	_, _ = s1.Track("s", 90.0)
	_, _ = s1.Track("s", 10.0)

	s2 := &Store{Dir: dir}
	n, _ := s2.Track("s", 5.0)
	if n != 1 {
		t.Errorf("after reload, count = %d, want 1", n)
	}

	stored := filepath.Join(dir, "s.json")
	if _, err := os.ReadFile(stored); err != nil {
		t.Errorf("expected store file to exist: %v", err)
	}
}
