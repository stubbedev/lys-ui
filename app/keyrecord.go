package app

import (
	"github.com/stubbedev/lys-lib/debug"
)

type inhibitor interface{ Destroy() }

func (a *Application) sessionInhibitShortcuts(w *hostWindow) (inhibitor, error) {
	if !a.sess.ShortcutsInhibitAvailable() {
		return nil, nil
	}
	inh, err := a.sess.InhibitShortcuts(w.host.HostSurface(), a.sess.Seat())
	if err != nil {
		return nil, err
	}
	return inh, nil
}

func (a *Application) syncShortcutsInhibit() {
	for _, w := range a.windows {
		_, recording := focusedRecorder(w.router)
		switch {
		case recording && w.shortcutsInhibitor == nil:
			inh, err := a.inhibitShortcuts(w)
			if err != nil {
				debug.Log("input", "inhibit the compositor's shortcuts while recording one: %v", err)
				continue
			}
			if inh != nil {
				w.shortcutsInhibitor = inh
			}
		case !recording && w.shortcutsInhibitor != nil:
			w.shortcutsInhibitor.Destroy()
			w.shortcutsInhibitor = nil
		}
	}
}
