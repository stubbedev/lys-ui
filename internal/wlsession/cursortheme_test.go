package wlsession

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
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

// loadedTheme opens the environment's theme on a shm whose compositor
// only has to hold the socket open: the pool's file is what is checked.
func loadedTheme(t *testing.T, base, theme string) *cursorTheme {
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
	loaded, err := loadCursorThemeFromEnv(cursorThemeSize, shm)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loaded.file.Close() })
	return loaded
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
	info, err := theme.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	const frameBytes = 24 * 24 * 4
	pool := make([]byte, 2*frameBytes)
	if _, err := theme.file.ReadAt(pool, info.Size()-2*frameBytes); err != nil {
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
	if _, err := theme.addImage(fileImage(t)); err != nil {
		t.Errorf("a file cursor on a missing theme: %v", err)
	}
}

func fileImage(t *testing.T) xcursor.Image {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c")
	if err := os.WriteFile(path, xcursorFile(frame{nominal: 8, side: 8}), 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := fileCursorImage(path)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestFileCursorPicksTheWidestImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c")
	data := xcursorFile(frame{nominal: 24, side: 24}, frame{nominal: 48, side: 48}, frame{nominal: 32, side: 32})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := fileCursorImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if img.Size.X != 48 {
		t.Errorf("picked width %d, want 48", img.Size.X)
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
	if _, err := fileCursorImage(path); err == nil {
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
