// Package compose turns keysym sequences into text: pressing ' then e
// on a us-intl/deadkeys layout commits "é". The XKB keymap only yields
// the dead_acute keysym; composing needs an XKB compose table
// (~/.XCompose plus the system file), which this package resolves and
// parses once per process and shares across input contexts.
//
// The parser, trie, and state machine come from lys-lib's pure-Go
// compose implementation (no cgo, the Compose file is just data); this
// package adds lys-ui's semantics on top: Feed answers pending,
// done-with-text, or none; backspace unwinds one level; a nil table —
// no compose file anywhere — is the disabled state and every call is a
// no-op, so callers never branch on whether compose exists.
//
// Compose commits on completion only, X11 style: while a sequence is
// in flight no text is emitted, and there is no preedit display (that
// is the IME's territory). Composing works without the text-input
// protocol.
package compose

import (
	"os"

	"github.com/stubbedev/lys-lib/xkb"
)

// Table is an immutable set of compose sequences: keysym chains mapped
// to their committed text. Safe for concurrent use; a process needs
// one table (Load) and each input context its own State (Start).
type Table struct {
	xkb *xkb.ComposeTable
}

// Load resolves the active compose file the way X11 clients do and
// parses it: $XCOMPOSEFILE, then ~/.XCompose, then the system file for
// the locale (LC_ALL/LC_CTYPE/LANG, with en_US.UTF-8 as the fallback).
// A nil Table means compose is disabled — no compose file exists, and
// dead-key layouts simply keep producing bare dead keysyms, which
// callers then treat like any other non-text key.
func Load() *Table {
	t, err := xkb.NewContext(os.Getenv).NewComposeTableFromLocale("")
	if err != nil || t == nil {
		return nil
	}
	return &Table{xkb: t}
}

// NewTableFile parses one explicit Compose file — the seam tests and
// unusual setups use instead of the Load search order.
func NewTableFile(path string) (*Table, error) {
	t, err := xkb.NewContext(os.Getenv).NewComposeTableFromFile(path, "")
	if err != nil {
		return nil, err
	}
	return &Table{xkb: t}, nil
}

// Start opens a state machine over the table. Start on a nil Table
// (compose disabled) returns a nil State whose methods are all no-ops.
func (t *Table) Start() *State {
	if t == nil {
		return nil
	}
	return &State{table: t, cs: t.xkb.NewState()}
}
