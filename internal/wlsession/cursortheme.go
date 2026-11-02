package wlsession

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	sys "github.com/stubbedev/lys-lib/third_party/wayland/os"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xcursor"
)

// The xcursor names of the shapes the toolkit asks for by name.
const (
	CursorLeftPtr           = "left_ptr"
	CursorTopSide           = "top_side"
	CursorBottomSide        = "bottom_side"
	CursorLeftSide          = "left_side"
	CursorRightSide         = "right_side"
	CursorTopLeftCorner     = "top_left_corner"
	CursorTopRightCorner    = "top_right_corner"
	CursorBottomLeftCorner  = "bottom_left_corner"
	CursorBottomRightCorner = "bottom_right_corner"
)

// fallbackCursorTheme is the theme every lookup ends in, as in libXcursor.
const fallbackCursorTheme = "default"

// cursorImage is one cursor frame in the theme's shm pool. delay is how
// long the frame of an animated cursor shows, in milliseconds.
type cursorImage struct {
	buffer             *wl.Buffer
	delay              uint32
	hotspotX, hotspotY int32
	width, height      int32
}

// themedCursor is one shape's frames, in order.
type themedCursor struct {
	images []*cursorImage
}

// cursorTheme holds the session's cursor buffers in one shm pool that
// grows as cursors load, and the themes that supply them. The pool
// serves a file cursor too, so it opens with no theme installed.
type cursorTheme struct {
	name     string
	size     uint32
	pool     *wl.ShmPool
	file     *os.File
	poolSize int32
	cursors  map[string]*themedCursor
	lookup   []*xcursor.Theme
	openErr  error
}

// loadCursorThemeFromEnv opens the theme XCURSOR_THEME names at the size
// XCURSOR_SIZE names, or the fallback theme at defaultSize.
func loadCursorThemeFromEnv(defaultSize uint32, shm *wl.Shm) (*cursorTheme, error) {
	const initialPoolSize = 16 * 16 * 4
	file, err := sys.CreateAnonymousFile(initialPoolSize)
	if err != nil {
		return nil, fmt.Errorf("cursor pool file: %w", err)
	}
	pool, err := shm.CreatePool(file.Fd(), initialPoolSize)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("cursor pool: %w", err), file.Close())
	}
	name := os.Getenv("XCURSOR_THEME")
	if name == "" {
		name = fallbackCursorTheme
	}
	size := defaultSize
	if v, err := strconv.ParseUint(os.Getenv("XCURSOR_SIZE"), 10, 32); err == nil {
		size = uint32(v)
	}
	lookup, openErr := openCursorThemes(name, cursorSearchPath(os.Getenv))
	return &cursorTheme{
		name:     name,
		size:     size,
		pool:     pool,
		file:     file,
		poolSize: initialPoolSize,
		cursors:  make(map[string]*themedCursor),
		lookup:   lookup,
		openErr:  openErr,
	}, nil
}

// cursorSearchPath is xcursor's search path and, unless XCURSOR_PATH
// replaces it, the two legacy directories libXcursor also looks in.
func cursorSearchPath(getenv func(string) string) []string {
	dirs := xcursor.SearchPath(getenv)
	if getenv("XCURSOR_PATH") != "" {
		return dirs
	}
	if home := getenv("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, ".cursors"))
	}
	return append(dirs, "/usr/share/cursors/xorg-x11")
}

// openCursorThemes opens the named theme and the fallback every lookup
// ends in. A theme that will not open leaves the other to serve; the
// error says why either did not.
func openCursorThemes(name string, searchPath []string) ([]*xcursor.Theme, error) {
	names := []string{name}
	if name != fallbackCursorTheme {
		names = append(names, fallbackCursorTheme)
	}
	themes := make([]*xcursor.Theme, 0, len(names))
	var errs []error
	for _, n := range names {
		t, err := xcursor.Open(n, searchPath)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		themes = append(themes, t)
	}
	return themes, errors.Join(errs...)
}

// cursorFrames reads a shape's frames from the first theme that has it.
func cursorFrames(themes []*xcursor.Theme, size int, name string) ([]xcursor.Image, error) {
	var errs []error
	for _, t := range themes {
		frames, err := t.Frames(size, name)
		if err == nil {
			return frames, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			debug.Log("input", "cursor %q: %v", name, err)
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("cursor %q: no theme to look it up in", name)
	}
	return nil, errors.Join(errs...)
}

// cursor returns the named shape, loading its frames into the pool the
// first time. A theme that did not open answers with why.
func (t *cursorTheme) cursor(name string) (*themedCursor, error) {
	if c, ok := t.cursors[name]; ok {
		return c, nil
	}
	frames, err := cursorFrames(t.lookup, intconv.Clamp[int](t.size), name)
	if err != nil {
		return nil, errors.Join(err, t.openErr)
	}
	c := &themedCursor{images: make([]*cursorImage, len(frames))}
	for i, frame := range frames {
		img, err := t.addImage(frame)
		if err != nil {
			return nil, fmt.Errorf("cursor %q: %w", name, err)
		}
		c.images[i] = img
	}
	t.cursors[name] = c
	return c, nil
}

// addImage appends one frame's pixels to the pool as a buffer.
func (t *cursorTheme) addImage(img xcursor.Image) (*cursorImage, error) {
	offset, err := t.file.Seek(0, 2)
	if err != nil {
		return nil, fmt.Errorf("seek the cursor pool: %w", err)
	}
	if err := t.grow(intconv.Clamp[int32](offset + int64(len(img.Pixels)))); err != nil {
		return nil, err
	}
	if _, err := t.file.Write(img.Pixels); err != nil {
		return nil, fmt.Errorf("write the cursor pool: %w", err)
	}
	w, h := intconv.Clamp[int32](img.Size.X), intconv.Clamp[int32](img.Size.Y)
	buffer, err := t.pool.CreateBuffer(intconv.Clamp[int32](offset), w, h, w*4, wl.ShmFormatArgb8888)
	if err != nil {
		return nil, fmt.Errorf("cursor buffer: %w", err)
	}
	return &cursorImage{
		buffer:   buffer,
		delay:    img.Delay,
		hotspotX: intconv.Clamp[int32](img.Hotspot.X),
		hotspotY: intconv.Clamp[int32](img.Hotspot.Y),
		width:    w,
		height:   h,
	}, nil
}

func (t *cursorTheme) grow(size int32) error {
	if size <= t.poolSize {
		return nil
	}
	if err := t.file.Truncate(int64(size)); err != nil {
		return fmt.Errorf("grow the cursor pool file to %d: %w", size, err)
	}
	if err := t.pool.Resize(size); err != nil {
		return fmt.Errorf("resize the cursor pool to %d: %w", size, err)
	}
	t.poolSize = size
	return nil
}
