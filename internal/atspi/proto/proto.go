// Package proto holds the AT-SPI names both sides of the protocol
// agree on: the bridge that serves an application and the assistive
// technology side the tests play (atspitest).
package proto

import dbus "github.com/stubbedev/lys-lib/third_party/godbus"

// The desktop's accessibility switch and the accessibility bus's
// address live on the session bus under at-spi-bus-launcher's name.
const (
	BusName     = "org.a11y.Bus"
	BusPath     = dbus.ObjectPath("/org/a11y/bus")
	IfaceStatus = "org.a11y.Status"
)

// RegistryName is at-spi2-registryd's name on the accessibility bus;
// its RootPath object is the desktop every application embeds under.
const RegistryName = "org.a11y.atspi.Registry"

// RootPath is the application root's object path, fixed by the AT-SPI
// spec ("An application must have a single root object, called
// /org/a11y/atspi/accessible/root").
const RootPath dbus.ObjectPath = "/org/a11y/atspi/accessible/root"

// Interfaces every accessible object and every application root serve.
const (
	IfaceAccessible  = "org.a11y.atspi.Accessible"
	IfaceApplication = "org.a11y.atspi.Application"
)
