package atspi

import (
	"context"
	"fmt"
	"sync"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

// The desktop's accessibility switch: org.a11y.Status on the session
// bus (at-spi-bus-launcher's object) says whether assistive
// technologies are wanted - IsEnabled (the toolkit-accessibility
// setting) or ScreenReaderEnabled (set while a screen reader runs).
// Qt's bridge follows the same rule: it serves only while either is
// true, so an application on a desktop without assistive technologies
// never walks its tree.

const (
	busName     = "org.a11y.Bus"
	busPath     = dbus.ObjectPath("/org/a11y/bus")
	ifaceStatus = "org.a11y.Status"
)

// WatchStatus reports the switch to fn: once before returning, then on
// every change, from the watch's goroutine, over the session bus conn,
// which stays the caller's. A bus without org.a11y.Bus is an error.
// stop ends the watch and takes back its match. It waits on the bus, so
// call it off the loop goroutine.
func WatchStatus(conn *dbus.Conn, fn func(enabled bool)) (stop func(), err error) {
	ctx, cancel := context.WithTimeout(context.Background(), busTimeout)
	defer cancel()
	obj := conn.Object(busName, busPath)
	props := map[string]bool{}
	for _, prop := range []string{"IsEnabled", "ScreenReaderEnabled"} {
		var v dbus.Variant
		if err := obj.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, ifaceStatus, prop).Store(&v); err != nil {
			return nil, fmt.Errorf("atspi: %s.%s: %w", ifaceStatus, prop, err)
		}
		props[prop], _ = v.Value().(bool)
	}
	enabled := props["IsEnabled"] || props["ScreenReaderEnabled"]
	match := statusMatch()
	if err := conn.AddMatchSignalContext(ctx, match...); err != nil {
		return nil, fmt.Errorf("atspi: watch status: %w", err)
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	fn(enabled)
	done := make(chan struct{})
	var follow sync.WaitGroup
	follow.Go(func() {
		for {
			select {
			case <-done:
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				if sig.Path != busPath || len(sig.Body) < 2 || sig.Body[0] != ifaceStatus {
					continue
				}
				changed, _ := sig.Body[1].(map[string]dbus.Variant)
				for prop := range props {
					if v, ok := changed[prop]; ok {
						props[prop], _ = v.Value().(bool)
					}
				}
				if now := props["IsEnabled"] || props["ScreenReaderEnabled"]; now != enabled {
					enabled = now
					fn(enabled)
				}
			}
		}
	})
	return sync.OnceFunc(func() {
		conn.RemoveSignal(signals)
		_ = conn.RemoveMatchSignal(match...)
		close(done)
		follow.Wait()
	}), nil
}

func statusMatch() []dbus.MatchOption {
	return []dbus.MatchOption{
		dbus.WithMatchObjectPath(busPath),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	}
}
