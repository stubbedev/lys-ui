package wlsession

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/wltest"
	"github.com/stubbedev/lys-lib/xcursor"
)

// frame is one image of a test Xcursor file; every pixel holds bgra.
type frame struct {
	nominal, side, delay uint32
	bgra                 [4]byte
}

func xcursorFile(frames ...frame) []byte {
	const headerSize, tocEntry, imageHeader = 16, 12, 36
	le := binary.LittleEndian
	out := le.AppendUint32([]byte("Xcur"), headerSize)
	out = le.AppendUint32(out, 0x10000)
	out = le.AppendUint32(out, uint32(len(frames)))
	pos := uint32(headerSize + tocEntry*len(frames))
	var body []byte
	for _, f := range frames {
		out = le.AppendUint32(out, 0xfffd0002)
		out = le.AppendUint32(out, f.nominal)
		out = le.AppendUint32(out, pos+uint32(len(body)))
		for _, v := range []uint32{imageHeader, 0xfffd0002, f.nominal, 1, f.side, f.side, 1, 2, f.delay} {
			body = le.AppendUint32(body, v)
		}
		body = append(body, bytes.Repeat(f.bgra[:], int(f.side*f.side))...)
	}
	return append(out, body...)
}

func writeCursor(t *testing.T, base, theme, name string, data []byte) {
	t.Helper()
	dir := filepath.Join(base, theme, "cursors")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

type loadedCursors struct {
	*cursorTheme
	reader *cursorReader
}

func (l *loadedCursors) cursor(name string) (*themedCursor, error) {
	if _, known, _ := l.lookup(name); !known {
		frames, err := l.reader.frames(name)
		l.addCursor(name, frames, err)
	}
	cur, _, err := l.lookup(name)
	return cur, err
}

func (l *loadedCursors) readFile(path string) (*cursorImage, error) {
	if _, known, _ := l.fileCursor(path); !known {
		img, err := xcursor.ReadWidest(path)
		l.addFile(path, img, err)
	}
	img, _, err := l.fileCursor(path)
	return img, err
}

// loadedTheme opens the environment's theme on a shm whose compositor
// only has to hold the socket open: the pool's file is what is checked.
func loadedTheme(t *testing.T, base, theme string) *loadedCursors {
	t.Helper()
	t.Setenv("XCURSOR_PATH", base)
	t.Setenv("XCURSOR_THEME", theme)
	t.Setenv("XCURSOR_SIZE", "")
	ln, _ := wltest.LoopbackDisplay(t, "wayland-cursor-theme")
	t.Cleanup(func() { _ = ln.Close() })
	d, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Context().Close() })
	registry, err := d.GetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	shm := wl.NewShm(d.Context())
	if err := registry.Bind(1, "wl_shm", 1, shm); err != nil {
		t.Fatal(err)
	}
	pool, err := newCursorTheme(shm)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.file.Close() })
	return &loadedCursors{cursorTheme: pool, reader: newCursorReaderFromEnv(cursorThemeSize)}
}

// An animated shape reaches the pool frame by frame at the size asked
// for, carrying its delays, hotspot and size, and with its bytes as the
// file stores them: Xcursor's ARGB32 is wl_shm's argb8888.
func TestThemeLoadsAnAnimatedCursorIntoThePool(t *testing.T) {
	base := t.TempDir()
	first, second := [4]byte{0x10, 0x20, 0x30, 0xff}, [4]byte{0x40, 0x50, 0x60, 0xff}
	writeCursor(t, base, "T", "watch", xcursorFile(
		frame{nominal: 48, side: 48, delay: 99, bgra: [4]byte{9, 9, 9, 9}},
		frame{nominal: 24, side: 24, delay: 30, bgra: first},
		frame{nominal: 24, side: 24, delay: 40, bgra: second},
	))
	theme := loadedTheme(t, base, "T")
	cur, err := theme.cursor("watch")
	if err != nil {
		t.Fatal(err)
	}
	if len(cur.images) != 2 {
		t.Fatalf("%d frames, want the two of size 24", len(cur.images))
	}
	for i, delay := range []uint32{30, 40} {
		img := cur.images[i]
		if img.delay != delay || img.width != 24 || img.height != 24 || img.hotspotX != 1 || img.hotspotY != 2 {
			t.Errorf("frame %d: %+v", i, img)
		}
	}
	const frameBytes = 24 * 24 * 4
	pool := make([]byte, 2*frameBytes)
	if _, err := theme.file.ReadAt(pool, 0); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool[:4], first[:]) || !slices.Equal(pool[frameBytes:frameBytes+4], second[:]) {
		t.Errorf("pool holds % x and % x, want the file's bytes", pool[:4], pool[frameBytes:frameBytes+4])
	}
	again, err := theme.cursor("watch")
	if err != nil || again != cur {
		t.Errorf("a second lookup = %p, %v; want the cached %p", again, err, cur)
	}
}

// A lookup that fails is not repeated: resolving a shape retries its
// missing candidates on every apply.
func TestAMissingShapeIsLookedUpOnce(t *testing.T) {
	base := t.TempDir()
	writeCursor(t, base, "T", "watch", xcursorFile(frame{nominal: 24, side: 24}))
	theme := loadedTheme(t, base, "T")
	if _, err := theme.cursor("xterm"); err == nil {
		t.Fatal("a shape the theme lacks loaded")
	}
	writeCursor(t, base, "T", "xterm", xcursorFile(frame{nominal: 24, side: 24}))
	if _, err := theme.cursor("xterm"); err == nil {
		t.Error("the second lookup went back to the disk")
	}
}

// As in libXcursor, a shape the theme lacks comes from the default
// theme, whether or not the theme inherits it.
func TestThemeEndsInTheDefaultTheme(t *testing.T) {
	base := t.TempDir()
	writeCursor(t, base, "T", "watch", xcursorFile(frame{nominal: 24, side: 24, bgra: [4]byte{1, 1, 1, 1}}))
	writeCursor(t, base, "default", "xterm", xcursorFile(frame{nominal: 24, side: 24, bgra: [4]byte{2, 2, 2, 2}}))
	theme := loadedTheme(t, base, "T")
	if _, err := theme.cursor("xterm"); err != nil {
		t.Errorf("xterm from the default theme: %v", err)
	}
	if _, err := theme.cursor("hand1"); err == nil {
		t.Error("a shape no theme has loaded")
	}
}

// A theme that is not installed only fails its lookups: the pool stays
// up for file cursors, and the lookup's error names the theme.
func TestAMissingThemeStillOpensThePool(t *testing.T) {
	theme := loadedTheme(t, t.TempDir(), "Gone")
	if _, err := theme.cursor("left_ptr"); err == nil {
		t.Fatal("a missing theme served a cursor")
	} else if !strings.Contains(err.Error(), `"Gone"`) {
		t.Errorf("error %q does not name the theme", err)
	}
	path := filepath.Join(t.TempDir(), "c")
	if err := os.WriteFile(path, xcursorFile(frame{nominal: 8, side: 8}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := theme.readFile(path); err != nil {
		t.Errorf("a file cursor on a missing theme: %v", err)
	}
}

func TestFileCursorPicksTheWidestImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c")
	data := xcursorFile(frame{nominal: 24, side: 24}, frame{nominal: 48, side: 48}, frame{nominal: 32, side: 32})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	theme := loadedTheme(t, t.TempDir(), "T")
	img, err := theme.readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if img.width != 48 {
		t.Errorf("picked width %d, want 48", img.width)
	}
	used := theme.used
	again, err := theme.readFile(path)
	if err != nil || again != img || theme.used != used {
		t.Errorf("showing it again: %p, %v, pool at %d from %d; want the same image", again, err, theme.used, used)
	}
}

// A real theme, the dev shell's, end to end into the pool.
func TestARealThemeLoadsIntoThePool(t *testing.T) {
	path := os.Getenv("LYS_UI_TEST_XCURSOR_PATH")
	if path == "" {
		t.Skip("LYS_UI_TEST_XCURSOR_PATH is not set")
	}
	theme := loadedTheme(t, path, "Vanilla-DMZ")
	cur, err := theme.cursor("watch")
	if err != nil {
		t.Fatal(err)
	}
	if len(cur.images) < 2 {
		t.Fatalf("watch has %d frames", len(cur.images))
	}
	for i, img := range cur.images {
		if img.delay == 0 || img.width != 24 {
			t.Errorf("frame %d: %+v", i, img)
		}
	}
	if _, err := theme.cursor("left_ptr"); err != nil {
		t.Error(err)
	}
}

// A truncated file is an error, the caller's cue for the CSS fallback,
// and never a panic.
func TestFileCursorRefusesATruncatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c")
	data := xcursorFile(frame{nominal: 24, side: 24})
	if err := os.WriteFile(path, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadedTheme(t, t.TempDir(), "T").readFile(path); err == nil {
		t.Error("a truncated file loaded")
	}
}

func TestCursorSearchPathAddsTheLegacyDirectories(t *testing.T) {
	env := map[string]string{"HOME": "/h"}
	dirs := cursorSearchPath(func(k string) string { return env[k] })
	for _, want := range []string{"/h/.local/share/icons", "/h/.icons", "/usr/share/icons", "/usr/share/pixmaps", "/h/.cursors", "/usr/share/cursors/xorg-x11"} {
		if !slices.Contains(dirs, want) {
			t.Errorf("%v lacks %s", dirs, want)
		}
	}
	env["XCURSOR_PATH"] = "/a:/b"
	if dirs := cursorSearchPath(func(k string) string { return env[k] }); !slices.Equal(dirs, []string{"/a", "/b"}) {
		t.Errorf("XCURSOR_PATH gave %v", dirs)
	}
}

type cursorRig struct {
	s       *Session
	pushed  []*cursorImage
	reads   chan string
	release chan struct{}
}

func newCursorRig(t *testing.T) *cursorRig {
	t.Helper()
	loaded := loadedTheme(t, t.TempDir(), "T")
	r := &cursorRig{reads: make(chan string, 64), release: make(chan struct{})}
	s := &Session{pointerEnterSerial: 1, pointer: &fakePointer{}}
	t.Cleanup(s.stopBackground)
	s.crs.pool = loaded.cursorTheme
	s.crs.surface = &wl.Surface{}
	s.crs.push = func(img *cursorImage) error {
		r.pushed = append(r.pushed, img)
		return nil
	}
	s.crs.readFrames = func(name string) ([]xcursor.Image, error) {
		r.reads <- goroutineID()
		<-r.release
		if name != "xterm" && name != defaultCursor {
			return nil, os.ErrNotExist
		}
		return []xcursor.Image{{Size: image.Pt(8, 8), Pixels: make([]byte, 8*8*4)}}, nil
	}
	s.crs.readFile = func(string) (xcursor.Image, error) {
		r.reads <- goroutineID()
		<-r.release
		return xcursor.Image{Size: image.Pt(8, 8), Pixels: make([]byte, 8*8*4)}, nil
	}
	r.s = s
	return r
}

func (r *cursorRig) readsOnLoop(t *testing.T) {
	t.Helper()
	for {
		select {
		case id := <-r.reads:
			if id == goroutineID() {
				t.Error("a cursor file was read on the loop goroutine")
			}
		default:
			return
		}
	}
}

func TestCursorShapeReadsStayOffTheLoop(t *testing.T) {
	r := newCursorRig(t)
	if err := r.s.SetCursor("text"); err != nil {
		t.Fatal(err)
	}
	if len(r.pushed) != 0 {
		t.Errorf("%d pushes before the read ended, want the old shape kept", len(r.pushed))
	}
	close(r.release)
	settleBackground(t, r.s)
	r.readsOnLoop(t)
	if len(r.pushed) != 1 || r.s.crs.shown != "xterm" {
		t.Fatalf("after the read: %d pushes, shown %q; want xterm on screen", len(r.pushed), r.s.crs.shown)
	}

	if err := r.s.SetCursor(""); err != nil {
		t.Fatal(err)
	}
	settleBackground(t, r.s)
	if r.s.crs.shown != defaultCursor {
		t.Errorf("shown %q, want the arrow", r.s.crs.shown)
	}
	r.readsOnLoop(t)
	if err := r.s.SetCursor("text"); err != nil {
		t.Fatal(err)
	}
	if len(r.reads) != 0 {
		t.Errorf("hovering a known shape read %d files", len(r.reads))
	}
	if r.s.crs.shown != "xterm" {
		t.Errorf("shown %q on the second hover, want it at once", r.s.crs.shown)
	}
}

func TestCursorReadForAnOldShapeShowsTheNewOne(t *testing.T) {
	r := newCursorRig(t)
	r.s.crs.readFrames = func(name string) ([]xcursor.Image, error) {
		<-r.release
		return []xcursor.Image{{Size: image.Pt(8, 8), Pixels: make([]byte, 8*8*4)}}, nil
	}
	if err := r.s.SetCursor("text"); err != nil {
		t.Fatal(err)
	}
	if err := r.s.SetCursor("pointer"); err != nil {
		t.Fatal(err)
	}
	close(r.release)
	settleBackground(t, r.s)
	if r.s.crs.shown != "hand1" {
		t.Errorf("shown %q, want the shape last asked for", r.s.crs.shown)
	}
}

func TestFileCursorReadsStayOffTheLoop(t *testing.T) {
	r := newCursorRig(t)
	const cursor = fileCursorPrefix + "/x/c.cur#1,2,text"
	if err := r.s.SetCursor(cursor); err != nil {
		t.Fatal(err)
	}
	close(r.release)
	settleBackground(t, r.s)
	r.readsOnLoop(t)
	used := r.s.crs.pool.used
	if _, known, err := r.s.crs.pool.fileCursor("/x/c.cur"); !known || err != nil {
		t.Fatalf("the file never landed: known %v, %v", known, err)
	}
	if len(r.pushed) == 0 {
		t.Fatal("nothing shown")
	}
	if err := r.s.SetCursor(cursor); err != nil {
		t.Fatal(err)
	}
	if len(r.reads) != 0 || r.s.crs.pool.used != used {
		t.Errorf("showing it again read %d files and took the pool from %d to %d", len(r.reads), used, r.s.crs.pool.used)
	}
}

func TestCursorPreloadFillsThePool(t *testing.T) {
	r := newCursorRig(t)
	r.s.shm = &wl.Shm{}
	close(r.release)
	r.s.preloadCursors()
	settleBackground(t, r.s)
	if len(r.pushed) != 0 {
		t.Errorf("the preload pushed %d shapes with none asked for", len(r.pushed))
	}
	if cur, known, _ := r.s.crs.pool.lookup("xterm"); !known || cur == nil {
		t.Error("the text shape was not preloaded")
	}
	reads := len(r.reads)
	r.readsOnLoop(t)
	if reads == 0 {
		t.Error("the preload read nothing")
	}
	if err := r.s.SetCursor("text"); err != nil {
		t.Fatal(err)
	}
	if r.s.crs.shown != "xterm" {
		t.Errorf("first hover after the preload shows %q, want its shape at once", r.s.crs.shown)
	}

	drawn := newCursorRig(t)
	drawn.s.shm = &wl.Shm{}
	drawn.s.shapeDevice = &wlr.WpCursorShapeDeviceV1{}
	drawn.s.preloadCursors()
	if len(drawn.reads) != 0 || drawn.s.bg != nil {
		t.Error("a compositor with cursor-shape-v1 still got a preload")
	}
}
