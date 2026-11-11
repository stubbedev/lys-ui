package atspi

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/stubbedev/lys-lib/dbusx"
	"github.com/stubbedev/lys-lib/debug"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/atspi/proto"
)

// The desktop's accessibility switch: org.a11y.Status on the session
// bus (at-spi-bus-launcher's object) says whether assistive
// technologies are wanted - IsEnabled (the toolkit-accessibility
// setting) or ScreenReaderEnabled (set while a screen reader runs).
// Qt's bridge follows the same rule: it serves only while either is
// true, so an application on a desktop without assistive technologies
// never walks its tree.

// WatchStatus reports the switch to fn: once before returning, then on
// every change, from the watch's goroutine, over the session bus conn,
// which stays the caller's. Only the owner of org.a11y.Bus is heard,
// and a bus without one is an error. stop ends the watch and takes back
// its match. It waits on the bus, so call it off the loop goroutine.
func WatchStatus(conn *dbus.Conn, fn func(enabled bool)) (stop func(), err error) {
	var (
		status a11yStatus
		asOf   dbus.Sequence
	)
	ready := make(chan struct{})
	watch, err := dbusx.WatchSignals(context.Background(), conn, dbusx.FromOwner(proto.BusName),
		func(err error) { debug.Log("a11y", "a status change was not read: %v", err) },
		dbusx.On(dbusx.PropertiesChanged, proto.BusPath, func(c dbusx.ChangedProperties, announced dbus.Sequence) error {
			<-ready
			closed := dbus.CloseFDs([]any{c.Changed})
			if announced <= asOf {
				return closed
			}
			was := status.on()
			if err := status.apply(c.Changed); err != nil {
				return errors.Join(err, closed)
			}
			if now := status.on(); now != was {
				fn(now)
			}
			return closed
		}).WithArg0(proto.IfaceStatus))
	if err != nil {
		return nil, fmt.Errorf("atspi: watch status: %w", err)
	}
	props, answered, err := readStatus(conn)
	if err == nil {
		err = status.apply(props)
	}
	if err != nil {
		close(ready)
		return nil, errors.Join(err, watch.Stop())
	}
	asOf = answered
	fn(status.on())
	close(ready)
	return sync.OnceFunc(func() {
		if err := watch.Stop(); err != nil {
			debug.Log("a11y", "stop watching the status: %v", err)
		}
	}), nil
}

// readStatus reads org.a11y.Status's properties, with the sequence of
// the answer: a change announced before it is already in what it holds.
func readStatus(conn *dbus.Conn) (map[string]dbus.Variant, dbus.Sequence, error) {
	ctx, cancel := context.WithTimeout(context.Background(), busTimeout)
	defer cancel()
	call := conn.Object(proto.BusName, proto.BusPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, proto.IfaceStatus)
	var props map[string]dbus.Variant
	if err := call.Store(&props); err != nil {
		return nil, call.ResponseSequence, fmt.Errorf("atspi: read %s: %w", proto.IfaceStatus, err)
	}
	return props, call.ResponseSequence, nil
}

// a11yStatus is the two properties of org.a11y.Status the switch is
// made of.
type a11yStatus struct {
	enabled, screenReader bool
}

func (s a11yStatus) on() bool { return s.enabled || s.screenReader }

// apply takes the properties props holds a new value of; one that is
// not a boolean is an error, and leaves s as it was.
func (s *a11yStatus) apply(props map[string]dbus.Variant) error {
	next := *s
	for _, prop := range []struct {
		name  string
		field *bool
	}{{"IsEnabled", &next.enabled}, {"ScreenReaderEnabled", &next.screenReader}} {
		v, ok := props[prop.name]
		if !ok {
			continue
		}
		b, ok := v.Value().(bool)
		if !ok {
			return fmt.Errorf("atspi: %s.%s is %s, want a boolean", proto.IfaceStatus, prop.name, v.Signature())
		}
		*prop.field = b
	}
	*s = next
	return nil
}
