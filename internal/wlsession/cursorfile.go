package wlsession

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/neurlang/wayland/wlcursor"
	"github.com/neurlang/wayland/wlcursor/xcursor"
	"github.com/stubbedev/lys-lib/debug"
)

// File cursors (CSS cursor: url()): the stylesheet names an xcursor
// file and a hotspot; the session loads its images straight off disk,
// no theme involved. Only one frame shows — the largest image the
// file carries — so an animated file cursor pins its opening frame;
// the theme path still advances the shapes it knows.

// fileCursorPrefix marks a SetCursor name that asks for a file
// cursor: `file:<path>#<x>,<y>[,<fallback>]`, the hotspot the
// stylesheet named replacing the file's own, and the CSS fallback
// keyword shown when the file cannot load.
const fileCursorPrefix = "file:"

// fileCursor is one parsed file cursor request.
type fileCursor struct {
	path     string
	hx, hy   int32
	fallback string
}

// parseFileCursor splits a file cursor request into its path, hotspot
// and fallback; ok reports the name is one.
func parseFileCursor(name string) (fileCursor, bool) {
	rest, found := strings.CutPrefix(name, fileCursorPrefix)
	if !found {
		return fileCursor{}, false
	}
	path, hot, found := strings.Cut(rest, "#")
	if !found || path == "" {
		return fileCursor{}, false
	}
	parts := strings.Split(hot, ",")
	if len(parts) != 2 && len(parts) != 3 {
		return fileCursor{}, false
	}
	hx, okx := hotspotCoord(parts[0])
	hy, oky := hotspotCoord(parts[1])
	if !okx || !oky {
		return fileCursor{}, false
	}
	fc := fileCursor{path: path, hx: hx, hy: hy}
	if len(parts) == 3 {
		fc.fallback = parts[2]
	}
	return fc, true
}

// hotspotCoord parses one hotspot coordinate: a non-negative integer
// in the int32 range wl_pointer.set_cursor carries.
func hotspotCoord(s string) (int32, bool) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0, false
	}
	return int32(v), true
}

// pushFileCursorLocked shows the widest image of the xcursor file
// with the CSS hotspot. The theme's shm pool backs the buffer, so a
// theme must load; a missing or malformed file degrades to the CSS
// fallback keyword (the arrow without one). Called with
// cursorState.mu held.
func (s *Session) pushFileCursorLocked(fc fileCursor) error {
	img, err := fileCursorImage(fc.path)
	if err != nil {
		debug.Log("input", "file cursor %q: %v", fc.path, err)
		return s.applyNamedCursorLocked(fc.fallback)
	}
	theme, err := loadCursorTheme(s.shm)
	if err != nil {
		debug.Log("input", "file cursor theme: %v", err)
		return s.applyNamedCursorLocked(fc.fallback)
	}
	ib, err := wlcursor.NewImageBuffer(theme, img)
	if err != nil {
		debug.Log("input", "file cursor buffer: %v", err)
		return s.applyNamedCursorLocked(fc.fallback)
	}
	if s.crs.push != nil {
		return s.crs.push(ib)
	}
	if err := s.crs.surface.Attach(ib.GetBuffer(), 0, 0); err != nil {
		return fmt.Errorf("cursor attach: %w", err)
	}
	if err := s.crs.surface.DamageBuffer(0, 0, int32(ib.GetWidth()), int32(ib.GetHeight())); err != nil {
		return fmt.Errorf("cursor damage: %w", err)
	}
	if err := s.crs.surface.Commit(); err != nil {
		return fmt.Errorf("cursor commit: %w", err)
	}
	if err := s.pointer.SetCursor(s.pointerEnterSerial, s.crs.surface, fc.hx, fc.hy); err != nil {
		return fmt.Errorf("set cursor: %w", err)
	}
	return nil
}

// fileCursorImage reads the xcursor file and picks its widest image.
func fileCursorImage(path string) (*xcursor.Image, error) {
	buf, err := os.ReadFile(path) //nolint:gosec // the path is the stylesheet's own url() cursor
	if err != nil {
		return nil, err
	}
	imgs, err := xcursor.ParseXcursor(buf)
	if err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return nil, errors.New("no images")
	}
	img := imgs[0]
	for _, cand := range imgs[1:] {
		if cand.Width > img.Width {
			img = cand
		}
	}
	return img, nil
}
