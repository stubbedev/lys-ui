package widget

import "github.com/stubbedev/lys-ui/internal/icons"

// SetDeviceScale records that root's tree presents at num device pixels
// per denom logical ones, and has every dynamic Icon in it decode for
// that scale off the loop ahead of its paint; an Icon arranged into the
// tree later does the same. The application calls it as a window learns
// its scale, before the first frame, and on every change, while the
// rasters on show stay until their replacements are decoded.
func SetDeviceScale(root Widget, num, denom int) {
	n := nodeOf(root)
	frac := fracOf(num, denom)
	if n == nil || n.deviceScale == frac {
		return
	}
	n.deviceScale = frac
	if frac == 0 {
		return
	}
	walkOwned(root, func(w Widget) {
		if p, ok := w.(scalePreparer); ok {
			p.prepare(frac, n)
		}
	})
}

// DeviceScaleOf returns the device scale w's tree presents at, num
// device pixels per denom logical ones, as SetDeviceScale set it on its
// style root; 0/120 while unknown.
func DeviceScaleOf(w Widget) (num, denom int) {
	n := nodeOf(w)
	if n == nil {
		return 0, iconScaleDenom
	}
	return int(styleRootOf(n).deviceScale), iconScaleDenom
}

type scalePreparer interface{ prepare(frac uint32, root *node) }

var _ scalePreparer = (*Icon)(nil)

func (i *Icon) prepare(frac uint32, root *node) {
	if i.kind == iconStatic {
		return
	}
	size := i.size
	if i.parent != nil {
		size = i.effSize()
	}
	box := iconBox{name: i.name, size: size, frac: frac}
	var gen uint64
	if i.kind == iconTheme {
		gen = icons.Default().Generation()
	}
	if i.prepared == box && i.preparedGen == gen {
		return
	}
	i.prepared, i.preparedGen = box, gen
	if _, ok := i.peek(box); ok {
		return
	}
	i.request(box, root)
}
