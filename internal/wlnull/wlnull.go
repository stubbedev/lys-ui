// Package wlnull is the null object argument the generated Wayland
// bindings cannot send: a typed-nil proxy (a nil *xdg.Surface, a nil
// *wl.Output) encodes as object 0 but then panics in the wire encoder's
// new-id scan, which calls Id on every proxy argument. Null is a nil
// pointer the wire writes as id 0 with every proxy method safe on it.
package wlnull

import "github.com/stubbedev/lys-lib/third_party/wayland/wl"

// Object is the nil-safe null proxy type.
type Object struct{}

// Null is the null object argument.
var Null = (*Object)(nil)

// Context is no connection.
func (*Object) Context() *wl.Context { return nil }

// SetContext ignores the connection.
func (*Object) SetContext(*wl.Context) {}

// Id is 0, the null object.
func (*Object) Id() wl.ProxyId { return 0 }

// SetId ignores the id: the null object never takes one.
func (*Object) SetId(wl.ProxyId) {}

// Unregister has nothing to unregister.
func (*Object) Unregister() {}
