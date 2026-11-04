// Primary selection behavior: middle-click paste at the caret and
// X11-style copy-on-select, layered on the clipboard's primary
// selection (zwp_primary_selection_unstable_v1 where the compositor
// offers it).
package app

import (
	"github.com/stubbedev/lys-ui/widget"
)

// primarySelection carries the primary-selection behavior for every
// window of one application: middle-click paste at the caret, and
// X11-style copy-on-select. A nil source (no clipboard configured, or
// no primary protocol) disables both; every method tolerates a nil
// receiver.
type primarySelection struct {
	src primarySelectionSource
	// copyOnSelect mirrors a finished selection into the primary
	// selection, X11 style. Off by default.
	copyOnSelect bool
}

// pasteAt starts reading the primary selection into the text widget
// under the pointer — else the keyboard-focused one — inserting at its
// caret when the transfer lands, off the loop like every paste
// (keyClipboard.paste). Reports whether a read started; report hears its
// failure.
//
// Every error rejects the paste, the same contract as the keyboard
// paste path: a hostile peer — a payload past xfer.MaxPayload, or one
// that stalls past the transfer deadline — pastes nothing and is
// reported (Application.OnTransferError).
//
// A read-only or disabled text widget under the pointer is still the
// target: it refuses the insert, as it refuses typing, and a read-only
// one rings the error bell - the paste is not redirected to whatever
// widget holds the keyboard, which would surprise more than help.
func (p *primarySelection) pasteAt(router *widget.Router, report func(error)) bool {
	if p == nil || p.src == nil {
		return false
	}
	target := inserterAt(router)
	if target == nil {
		return false
	}
	p.src.ReadPrimary(func(text string, err error) {
		if err != nil {
			report(err)
			return
		}
		// One Insert replaces any active selection and fires OnChanged
		// exactly once, whatever the pasted length.
		target.Insert(text)
	})
	return true
}

// copyAfterRelease claims the primary selection with the focused
// widget's selection once a selection-making press ends, X11 style;
// serial is the release's serial, the triggering event the protocol
// asks for. No-op unless copy-on-select is enabled.
func (p *primarySelection) copyAfterRelease(router *widget.Router, serial uint32) {
	if p == nil || !p.copyOnSelect || p.src == nil {
		return
	}
	copySelectionPrimary(router, p.src, serial)
}

// inserterAt picks the widget that receives a primary paste: the one
// under the pointer, else the keyboard-focused one.
func inserterAt(router *widget.Router) widget.TextInserter {
	for _, w := range []widget.Widget{router.Hovered(), router.Focused()} {
		if in, ok := w.(widget.TextInserter); ok {
			return in
		}
	}
	return nil
}
