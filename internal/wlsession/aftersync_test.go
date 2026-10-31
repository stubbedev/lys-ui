package wlsession

import (
	"errors"
	"testing"
	"time"
)

// AfterSync runs its func from the dispatch of the compositor's answer,
// not before, even when that answer is the next event a Step reads.
func TestAfterSyncRunsOnTheAnswer(t *testing.T) {
	s, _, answered := newSyncSession(t)
	ran := false
	if err := s.AfterSync(func() { ran = true }); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("AfterSync ran its func before any answer was dispatched")
	}
	stepped := make(chan error, 1)
	go func() { stepped <- s.Step() }()
	select {
	case err := <-stepped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no answer dispatched (%d syncs answered)", answered.Load())
	}
	if !ran {
		t.Error("the answer was dispatched and the func did not run")
	}
}

// On a closed connection AfterSync fails and never runs the func.
func TestAfterSyncOnAClosedSessionFails(t *testing.T) {
	s, _, _ := newSyncSession(t)
	s.Close()
	if err := s.AfterSync(func() { t.Error("ran on a closed session") }); !errors.Is(err, ErrDisconnected) {
		t.Errorf("AfterSync on a closed session = %v, want ErrDisconnected", err)
	}
}
