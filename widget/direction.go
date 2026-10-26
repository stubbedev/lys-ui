package widget

import "github.com/stubbedev/lys-lib/text"

// Direction names the base paragraph direction a text widget resolves
// and lays out with. It is text.Direction — the one resolution every
// text path shares; the alias lets call sites read against the widget
// package.
type Direction = text.Direction

const (
	// DirectionAuto resolves the base from the paragraph's first strong
	// character (UAX #9 rules P2-P3). It is the zero value: existing
	// LTR content renders unchanged, and a Hebrew- or Arabic-first
	// paragraph lays out right to left on its own.
	DirectionAuto = text.DirectionAuto
	// DirectionLTR forces a left-to-right base; start hugs the left.
	DirectionLTR = text.DirectionLTR
	// DirectionRTL forces a right-to-left base; start hugs the right,
	// and a Box row flows from the right edge.
	DirectionRTL = text.DirectionRTL
)
