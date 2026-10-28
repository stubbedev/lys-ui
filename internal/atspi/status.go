package atspi

import (
	"errors"
	"fmt"
	"os"

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
// every change, from the watch's goroutine. address is the session
// bus; empty reads DBUS_SESSION_BUS_ADDRESS. No session bus, or none
// with org.a11y.Bus, is an error. stop ends the watch.
func WatchStatus(address string, fn func(enabled bool)) (stop func(), err error) {
	if address == "" {
		address = os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	}
	if address == "" {
		return nil, errors.New("atspi: no session bus")
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		return nil, fmt.Errorf("atspi: session bus: %w", err)
	}
	obj := conn.Object(busName, busPath)
	props := map[string]bool{}
	for _, prop := range []string{"IsEnabled", "ScreenReaderEnabled"} {
		v, err := obj.GetProperty(ifaceStatus + "." + prop)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("atspi: %s.%s: %w", ifaceStatus, prop, err)
		}
		props[prop], _ = v.Value().(bool)
	}
	enabled := props["IsEnabled"] || props["ScreenReaderEnabled"]
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(busPath),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("atspi: watch status: %w", err)
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	fn(enabled)
	go func() {
		// The signal carries the new values: reading them there, not
		// re-reading the properties, keeps every transition in order
		// (a launcher that cannot persist the setting flips it back at
		// once).
		for sig := range signals {
			if len(sig.Body) < 2 || sig.Body[0] != ifaceStatus {
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
	}()
	return func() { _ = conn.Close() }, nil
}
