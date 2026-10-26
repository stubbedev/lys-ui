package app

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

func TestGlobalShortcutEventsReachTheCallback(t *testing.T) {
	type ev struct {
		pressed bool
		sec     uint64
	}
	var got []ev
	s := &GlobalShortcut{on: func(pressed bool, sec uint64) { got = append(got, ev{pressed, sec}) }}
	s.HandleGlobalShortcutV1Pressed(wlr.GlobalShortcutV1PressedEvent{TvSecHi: 1, TvSecLo: 5})
	s.HandleGlobalShortcutV1Released(wlr.GlobalShortcutV1ReleasedEvent{TvSecHi: 0, TvSecLo: 42})
	if want := []ev{{true, 1<<32 + 5}, {false, 42}}; !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if shortcutSeconds(^uint32(0), ^uint32(0)) != ^uint64(0) {
		t.Error("the halves do not combine")
	}
	// Destroy without a protocol object is harmless and idempotent.
	s.Destroy()
	s.Destroy()
}
