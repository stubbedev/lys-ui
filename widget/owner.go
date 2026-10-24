package widget

import "reflect"

// widgetPkg is this package's import path: the types whose values own
// their nodes.
var widgetPkg = reflect.TypeFor[Base]().PkgPath()

// ownerPaths caches, per wrapper type, the embedded-field path to the
// lys-ui widget that owns its node; nil for a type that is its own
// owner. Styling runs on the event-loop goroutine, like the rest of
// the cascade, so the cache needs no lock.
var ownerPaths = map[reflect.Type][]int{}

// styleOwner is the widget a node styles as: w itself, or, for a
// struct that embeds a lys-ui widget to add behavior (type toggle
// struct{ *widget.Switch }), that widget. A container holds the
// wrapper and the widget styles itself; both must resolve the same
// cascade, or a rule naming the widget's element matches one and not
// the other. A custom widget built on Base stays its own owner: its
// methods are its behavior.
func styleOwner(w Widget) Widget {
	t := reflect.TypeOf(w)
	path, ok := ownerPaths[t]
	if !ok {
		path = ownerPath(t)
		ownerPaths[t] = path
	}
	if path == nil {
		return w
	}
	v := reflect.ValueOf(w)
	for _, i := range path {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return w
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return w
	}
	if o, ok := reflect.TypeAssert[Widget](v); ok {
		return o
	}
	return w
}

// ownerPath finds the embedded-field path from t to the lys-ui widget
// that owns t's node, nil when t owns it itself.
func ownerPath(t reflect.Type) []int {
	st := t
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct || st.PkgPath() == widgetPkg {
		return nil
	}
	widget := reflect.TypeFor[Widget]()
	for i := range st.NumField() {
		f := st.Field(i)
		if !f.Anonymous || !f.IsExported() || !f.Type.Implements(widget) {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.PkgPath() == widgetPkg {
			if ft == reflect.TypeFor[Base]() {
				return nil
			}
			return []int{i}
		}
		if sub := ownerPath(f.Type); sub != nil {
			return append([]int{i}, sub...)
		}
	}
	return nil
}
