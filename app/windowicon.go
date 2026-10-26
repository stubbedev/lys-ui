// Window icons through the optional xdg-toplevel-icon-v1 protocol
// (#70): SetWindowIcon posts a per-window icon, SetIcon an
// application-wide default, both rasterized into square shm buffers at
// the sizes the compositor asked for (its icon_size events; a fixed
// ladder when it expressed none). The icon goes to the compositor or
// taskbar through the protocol - there is no WM_HINTS analog - and a
// compositor without the global gets a Debug note and no behavior
// change, never a failure.
package app

import (
	"image"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/logutil"
)

// defaultIconSizes is the rasterization ladder when the compositor
// expressed no preference: the sizes taskbars and switchers commonly
// ask for.
var defaultIconSizes = []int{16, 32, 48, 64, 128}

// postedIcon is one window's live toplevel icon: the protocol object
// and the shm buffers it references, which must outlive both.
type postedIcon struct {
	icon    *wlr.ToplevelIconV1
	buffers []*buffer.Buffer
}

// release destroys the icon object and retires its never-committed
// buffers back to the session arena.
func (p *postedIcon) release() {
	if p == nil {
		return
	}
	if p.icon != nil {
		_ = p.icon.Destroy()
		p.icon = nil
	}
	for _, b := range p.buffers {
		buffer.Cancel(b)
	}
	p.buffers = nil
}

// SetIcon posts a default window icon for every window of this
// application: windows without their own SetWindowIcon pick it up at
// their next apply, new windows at creation. A nil image clears the
// default. No-op (Debug) without the protocol.
func (a *Application) SetIcon(img image.Image) {
	if a.windowIcons == nil {
		a.windowIcons = make(map[*Window]*postedIcon)
	}
	if a.appliedIcons == nil {
		a.appliedIcons = make(map[*hostWindow]*postedIcon)
	}
	a.defaultIconSrc = img
	a.defaultIcon = a.buildIcon(img)
	for _, hw := range a.windows {
		if _, has := a.windowIcons[hw.win]; has {
			continue
		}
		a.applyWindowIcon(hw, a.defaultIcon)
	}
}

// SetWindowIcon posts icon as w's toplevel icon, overriding the
// application default: rasterized at the compositor's preferred sizes
// into square shm buffers, applied on the window's next commit (the
// repaint this schedules). A nil img clears the window's icon back to
// the default. No-op (Debug) without the protocol; never a failure.
func (a *Application) SetWindowIcon(w *Window, img image.Image) {
	if w == nil {
		return
	}
	if a.windowIcons == nil {
		a.windowIcons = make(map[*Window]*postedIcon)
	}
	if a.windowIconSrc == nil {
		a.windowIconSrc = make(map[*Window]image.Image)
	}
	a.windowIconSrc[w] = img
	icon := a.buildIcon(img)
	a.windowIcons[w] = icon
	if hw := a.hostWindowOf(w); hw != nil {
		if a.appliedIcons == nil {
			a.appliedIcons = make(map[*hostWindow]*postedIcon)
		}
		a.applyWindowIcon(hw, icon)
	}
}

// buildIcon rasterizes img into a protocol icon object with one buffer
// per size, or nil when there is nothing to post (nil img, no
// protocol, unreadable session).
func (a *Application) buildIcon(img image.Image) *postedIcon {
	if a.sess == nil {
		return nil
	}
	mgr := a.sess.ToplevelIconManager()
	if mgr == nil {
		debug.Log("shell", "window icon ignored: compositor has no xdg-toplevel-icon-v1")
		return nil
	}
	if img == nil {
		return nil
	}
	icon, err := mgr.CreateIcon()
	if err != nil {
		logutil.L().Warn("app: window icon: create icon", "err", err)
		return nil
	}
	p := &postedIcon{icon: icon}
	sizes := a.sess.PreferredIconSizes()
	if len(sizes) == 0 {
		sizes = defaultIconSizes
	}
	for _, size := range sizes {
		if size <= 0 || size > 512 {
			continue
		}
		b, err := buffer.NewFile(a.sess.Shm(), size, size, 1)
		if err != nil {
			logutil.L().Warn("app: window icon: buffer", "size", size, "err", err)
			continue
		}
		cv := render.New(b.Data, b.Stride, size, size)
		cv.Clear(cv.Rect(), 0)
		// A square, aspect-keeping crop: the ImageCover policy resampled
		// to the full square, the same kernel widget.Image uses.
		src, dst := render.PlaceImage(img.Bounds().Dx(), img.Bounds().Dy(), size, size, render.ImageCover, render.ImageCentered)
		cv.DrawImageDevice(render.Resample(img, src, dst.W, dst.H), dst.X, dst.Y)
		if err := icon.AddBuffer(b.WL, 1); err != nil {
			logutil.L().Warn("app: window icon: add buffer", "size", size, "err", err)
			buffer.Cancel(b)
			continue
		}
		p.buffers = append(p.buffers, b)
	}
	if len(p.buffers) == 0 {
		// An icon with no buffers would reset the toplevel's icon to its
		// default - the same as posting nothing, so post nothing.
		p.release()
		return nil
	}
	return p
}

// applyWindowIcon assigns (or clears) the icon on one window's
// toplevel and schedules the commit that applies the double-buffered
// state.
func (a *Application) applyWindowIcon(w *hostWindow, icon *postedIcon) {
	if a.sess == nil {
		return
	}
	mgr := a.sess.ToplevelIconManager()
	if mgr == nil || w == nil || w.win == nil || w.win.win == nil || w.win.win.Toplevel == nil {
		return
	}
	toplevel := w.win.win.Toplevel
	prev := a.appliedIcons[w]
	if prev != nil {
		// The toplevel's icon state is replaced wholesale; the old
		// object and buffers can go.
		prev.release()
		delete(a.appliedIcons, w)
	}
	if icon == nil {
		// Reset to the compositor default (a NULL icon).
		if err := mgr.SetIcon(toplevel, nil); err != nil {
			logutil.L().Warn("app: window icon: clear", "err", err)
		}
		w.dirty = true
		return
	}
	if err := mgr.SetIcon(toplevel, icon.icon); err != nil {
		logutil.L().Warn("app: window icon: set", "err", err)
		icon.release()
		return
	}
	a.appliedIcons[w] = icon
	w.dirty = true
}

// reapWindowIcons releases the icons of windows that left the loop:
// the icon objects must not outlive their toplevels' buffers.
func (a *Application) reapWindowIcons() {
	if len(a.appliedIcons) == 0 && len(a.windowIcons) == 0 && len(a.windowIconSrc) == 0 {
		return
	}
	live := make(map[*hostWindow]bool, len(a.windows))
	handles := make(map[*Window]bool, len(a.windows))
	for _, hw := range a.windows {
		live[hw] = true
		handles[hw.win] = true
	}
	for hw, icon := range a.appliedIcons {
		if !live[hw] {
			icon.release()
			delete(a.appliedIcons, hw)
		}
	}
	for w := range a.windowIconSrc {
		if !handles[w] {
			delete(a.windowIconSrc, w)
		}
	}
	for w := range a.windowIcons {
		if !handles[w] {
			delete(a.windowIcons, w)
		}
	}
}

// repostIcons posts the icons again on a new session (reconnect.go):
// the posted objects died with the old connection, the source images
// did not.
func (a *Application) repostIcons() {
	a.windowIcons, a.appliedIcons, a.defaultIcon = nil, make(map[*hostWindow]*postedIcon), nil
	if a.defaultIconSrc != nil {
		a.SetIcon(a.defaultIconSrc)
	}
	for w, img := range a.windowIconSrc {
		a.SetWindowIcon(w, img)
	}
}
