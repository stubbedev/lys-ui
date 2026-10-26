// Command lys-ui-lock is the M5 lock screen: one ext-session-lock-v1
// surface per output with a centered auth entry, built through
// app.LockSession. Typing lys-ui and pressing Enter unlocks; a wrong
// attempt clears the field and refocuses it on every output. Escape
// is the compositor's to refuse: a confirmed lock only ends through
// Unlock.
package main

import (
	"errors"
	"log"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/widget"
)

const password = "lys-ui"

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	defer func() { _ = sess.Roundtrip() }()
	application := app.NewApplication(sess)
	if !application.SessionLockAvailable() {
		return app.ErrSessionLockUnavailable
	}
	face, err := app.UIFont(15)
	if err != nil {
		return err
	}
	theme := widget.Current()

	var lock *app.SessionLock
	submit := func(entry *widget.Entry) func(string) {
		return func(text string) {
			if text == password {
				if err := lock.Unlock(); err != nil {
					log.Printf("lys-ui-lock: %v", err)
					return
				}
				application.Quit()
				return
			}
			entry.SetText("")
			if err := lock.SetFocus(entry); err != nil {
				log.Printf("lys-ui-lock: %v", err)
			}
		}
	}
	lock, err = application.LockSession(app.SessionLockConfig{
		Surface: func(out *app.Output) app.LockSurface {
			entry := widget.NewEntry(face, 15, theme.Text)
			entry.SetPlaceholder("password")
			entry.OnActivate = submit(entry)
			card := widget.NewBox(widget.Column, 10, 24)
			card.Append(widget.NewThemeIcon("system-lock-screen", 32), false)
			card.Append(widget.NewLabel(face, 17, "lys-ui", theme.Text), false)
			card.Append(entry, false)
			card.Append(widget.NewLabel(face, 12, "type "+password+" and press Enter", theme.TextMuted), false)
			centered := widget.NewCenterBox(nil, card, nil)
			return app.LockSurface{
				Root:       centered,
				Background: render.RGB(0x11, 0x11, 0x1B),
				Focus:      entry,
			}
		},
		OnFinished: func() { log.Print("lys-ui-lock: the compositor ended the lock") },
	})
	if err != nil {
		return err
	}
	return application.Run()
}
