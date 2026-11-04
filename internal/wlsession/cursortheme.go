package wlsession

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	sys "github.com/stubbedev/lys-lib/third_party/wayland/os"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xcursor"
	"golang.org/x/sys/unix"
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

// maxPwriteIovs is IOV_MAX, the most one pwritev takes.
const maxPwriteIovs = 1024

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
// grows as cursors arrive from the background goroutine, and what is
// known of each shape. The pool serves a file cursor too, so it opens
// with no theme installed. It touches no file but its own memfd.
type cursorTheme struct {
	pool     *wl.ShmPool
	file     *os.File
	poolSize int32
	cursors  map[string]*themedCursor
	missing  map[string]error
	files    map[string]*cursorImage
	badFiles map[string]error
	used     int64
}

// newCursorTheme opens an empty pool.
func newCursorTheme(shm *wl.Shm) (*cursorTheme, error) {
	const initialPoolSize = 16 * 16 * 4
	file, err := sys.CreateAnonymousFile(initialPoolSize)
	if err != nil {
		return nil, fmt.Errorf("cursor pool file: %w", err)
	}
	pool, err := shm.CreatePool(file.Fd(), initialPoolSize)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("cursor pool: %w", err), file.Close())
	}
	return &cursorTheme{
		pool:     pool,
		file:     file,
		poolSize: initialPoolSize,
		cursors:  make(map[string]*themedCursor),
		missing:  make(map[string]error),
		files:    make(map[string]*cursorImage),
		badFiles: make(map[string]error),
	}, nil
}

// cursorReader reads shapes from the themes: disk work, for the background
// goroutine alone.
type cursorReader struct {
	size    uint32
	lookup  []*xcursor.Theme
	openErr error
}

// cursorThemeFromEnv is the theme XCURSOR_THEME names at the size
// XCURSOR_SIZE names, or the fallback theme at defaultSize.
func cursorThemeFromEnv(defaultSize uint32) (string, uint32) {
	name := os.Getenv("XCURSOR_THEME")
	if name == "" {
		name = fallbackCursorTheme
	}
	size := defaultSize
	if v, err := strconv.ParseUint(os.Getenv("XCURSOR_SIZE"), 10, 32); err == nil {
		size = uint32(v)
	}
	return name, size
}

// newCursorReaderFromEnv opens the theme from the environment.
func newCursorReaderFromEnv(defaultSize uint32) *cursorReader {
	name, size := cursorThemeFromEnv(defaultSize)
	lookup, openErr := openCursorThemes(name, cursorSearchPath(os.Getenv))
	return &cursorReader{size: size, lookup: lookup, openErr: openErr}
}

// frames reads a shape, its error naming a theme that did not open.
func (r *cursorReader) frames(name string) ([]xcursor.Image, error) {
	frames, err := cursorFrames(r.lookup, intconv.Clamp[int](r.size), name)
	if err != nil {
		return nil, errors.Join(err, r.openErr)
	}
	return frames, nil
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

// lookup reports what is known of a shape: its frames, whether it has
// been read at all, and why it has none.
func (t *cursorTheme) lookup(name string) (cur *themedCursor, known bool, err error) {
	if c, ok := t.cursors[name]; ok {
		return c, true, nil
	}
	if err, ok := t.missing[name]; ok {
		return nil, true, err
	}
	return nil, false, nil
}

// resolve picks the shape desired shows, the first of its candidates that
// exists, degrading to the arrow when none does. settled is false while
// a shape that could change the answer has not been read.
func (t *cursorTheme) resolve(desired string) (name string, settled bool) {
	for _, candidate := range cursorCandidates(desired) {
		cur, known, _ := t.lookup(candidate)
		if !known {
			return "", false
		}
		if cur != nil {
			return candidate, true
		}
	}
	_, known, _ := t.lookup(defaultCursor)
	return defaultCursor, known
}

// unread lists what must be read to resolve desired, in the order resolve
// asks: its candidates, then the arrow.
func (t *cursorTheme) unread(desired string) []string {
	candidates := cursorCandidates(desired)
	names := make([]string, 0, len(candidates)+1)
	for _, name := range append(slices.Clone(candidates), defaultCursor) {
		if _, known, _ := t.lookup(name); !known && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// addCursor records one read of a shape, into the pool when it has
// frames. A shape that failed is not read again: resolve walks a shape's
// candidates on every apply.
func (t *cursorTheme) addCursor(name string, frames []xcursor.Image, err error) {
	if _, known, _ := t.lookup(name); known {
		return
	}
	if err == nil {
		var images []*cursorImage
		if images, err = t.addImages(frames); err == nil {
			t.cursors[name] = &themedCursor{images: images}
			return
		}
		err = fmt.Errorf("cursor %q: %w", name, err)
	}
	t.missing[name] = err
}

// fileCursor reports what is known of the xcursor file at path: its image,
// whether it has been read at all, and why it has none. It is added once
// per file: the pool only grows.
func (t *cursorTheme) fileCursor(path string) (img *cursorImage, known bool, err error) {
	if img, ok := t.files[path]; ok {
		return img, true, nil
	}
	if err, ok := t.badFiles[path]; ok {
		return nil, true, err
	}
	return nil, false, nil
}

// addFile records one read of a file cursor, into the pool when it has an
// image.
func (t *cursorTheme) addFile(path string, img xcursor.Image, err error) {
	if _, known, _ := t.fileCursor(path); known {
		return
	}
	if err == nil {
		var images []*cursorImage
		if images, err = t.addImages([]xcursor.Image{img}); err == nil {
			t.files[path] = images[0]
			return
		}
	}
	t.badFiles[path] = err
}

// addImages puts frames in the pool, growing it and writing them at once,
// and makes a buffer of each.
func (t *cursorTheme) addImages(frames []xcursor.Image) ([]*cursorImage, error) {
	iovs := make([][]byte, len(frames))
	var total int64
	for i := range frames {
		iovs[i] = frames[i].Pixels
		total += int64(len(frames[i].Pixels))
	}
	if err := t.grow(intconv.Clamp[int32](t.used + total)); err != nil {
		return nil, err
	}
	if err := pwriteAll(t.file, iovs, t.used); err != nil {
		return nil, fmt.Errorf("write the cursor pool: %w", err)
	}
	images := make([]*cursorImage, len(frames))
	backing := make([]cursorImage, len(frames))
	for i := range frames {
		w, h := intconv.Clamp[int32](frames[i].Size.X), intconv.Clamp[int32](frames[i].Size.Y)
		buffer, err := t.pool.CreateBuffer(intconv.Clamp[int32](t.used), w, h, w*4, wl.ShmFormatArgb8888)
		if err != nil {
			return nil, fmt.Errorf("cursor buffer: %w", err)
		}
		t.used += int64(len(frames[i].Pixels))
		backing[i] = cursorImage{
			buffer:   buffer,
			delay:    frames[i].Delay,
			hotspotX: intconv.Clamp[int32](frames[i].Hotspot.X),
			hotspotY: intconv.Clamp[int32](frames[i].Hotspot.Y),
			width:    w,
			height:   h,
		}
		images[i] = &backing[i]
	}
	return images, nil
}

// pwriteAll writes iovs at off in as few syscalls as the kernel lets it.
func pwriteAll(f *os.File, iovs [][]byte, off int64) error {
	for len(iovs) > 0 {
		n, err := unix.Pwritev(int(f.Fd()), iovs[:min(len(iovs), maxPwriteIovs)], off)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		off += int64(n)
		for n > 0 {
			if n < len(iovs[0]) {
				iovs[0] = iovs[0][n:]
				break
			}
			n -= len(iovs[0])
			iovs = iovs[1:]
		}
	}
	return nil
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
