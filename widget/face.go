package widget

import (
	"github.com/stubbedev/lys-lib/render"
)

// requireFace enforces the toolkit-wide nil-face contract for
// constructors: a constructor that takes a font either receives a
// usable face or panics right there with a message naming the face
// argument — never later, on the first Shape deep in shaping.
//
// The check covers typed nils too: (*render.Typeface)(nil) and
// (*render.Chain)(nil) inside the interface are not == nil, but they
// are just as unusable, and used to fail exactly the deep way this
// contract exists to prevent.
func requireFace(constructor string, face render.Font) render.Font {
	switch f := face.(type) {
	case nil:
		panic(constructor + ": nil face")
	case *render.Typeface:
		if f == nil {
			panic(constructor + ": nil face")
		}
	case *render.Chain:
		if f == nil {
			panic(constructor + ": nil face")
		}
	}
	return face
}
