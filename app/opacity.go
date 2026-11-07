package app

// surfaceOpacity is what the compositor multiplies a surface by: the
// app's SetOpacity times the fade a Revealer at the surface's root
// drives, each 1 until set.
type surfaceOpacity struct {
	app, fade       float64
	appSet, fadeSet bool
}

// value is the opacity to send, and whether either part was ever set.
func (o surfaceOpacity) value() (float64, bool) {
	v := 1.0
	if o.appSet {
		v = o.app
	}
	if o.fadeSet {
		v *= o.fade
	}
	return v, o.appSet || o.fadeSet
}

func (o *surfaceOpacity) setApp(alpha float64) { o.app, o.appSet = alpha, true }

func (o *surfaceOpacity) setFade(alpha float64) { o.fade, o.fadeSet = alpha, true }

// fadeOpacity sends o faded to alpha, and keeps the fade only once the
// compositor took it: a refused fade is drawn by the Revealer instead.
func fadeOpacity(o *surfaceOpacity, alpha float64, send func(surfaceOpacity) error) bool {
	faded := *o
	faded.setFade(alpha)
	if send(faded) != nil {
		return false
	}
	*o = faded
	return true
}
