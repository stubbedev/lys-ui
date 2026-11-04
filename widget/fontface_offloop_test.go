package widget

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/uifont"
)

func writeFont(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.ttf")
	if err := os.WriteFile(path, uifont.Sans, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fontFaceCSS(family, path string) string {
	return fmt.Sprintf(`@font-face { font-family: %q; src: url(%q) format("truetype"); }`, family, path)
}

func mappingsOf(t *testing.T, path string) int {
	t.Helper()
	maps, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(maps), path)
}

func TestFontFaceMapsOnceAcrossRelayers(t *testing.T) {
	path := writeFont(t)
	addCSS(t, fontFaceCSS("Once Face", path), 900)
	for range 16 {
		relayer()
	}
	if n := mappingsOf(t, path); n != 1 {
		t.Errorf("mapped %d times across relayers, want once", n)
	}
	if fontFaceOf("Once Face", 400) == nil {
		t.Error("the family does not resolve")
	}
}

func TestFontFaceNeverOpensOnTheLoop(t *testing.T) {
	f := newFakeInvoker(t)
	path := filepath.Join(t.TempDir(), "held.ttf")
	fifoPath(t, path)
	var warns []string
	style.SetParseWarn(func(msg string) { warns = append(warns, msg) })
	t.Cleanup(func() { style.SetParseWarn(nil) })
	onTime(t, "installing a stylesheet", func() {
		s := AddStylesheet(fontFaceCSS("Held Face", path), 900)
		t.Cleanup(s.Remove)
	})
	if fontFaceOf("Held Face", 400) != nil {
		t.Fatal("the family resolved before its file was read")
	}
	if len(warns) != 0 {
		t.Fatalf("reported before the answer: %v", warns)
	}
	settleAssets(t, f)
	if len(warns) != 1 || !strings.Contains(warns[0], path) {
		t.Errorf("warnings = %v, want one naming the file", warns)
	}
}

func TestFontFaceArrivesWithARelayer(t *testing.T) {
	f := newFakeInvoker(t)
	path := writeFont(t)
	before := styleGen
	s := AddStylesheet(fontFaceCSS("Late Face", path), 900)
	t.Cleanup(s.Remove)
	if fontFaceOf("Late Face", 400) != nil {
		t.Fatal("resolved before its answer")
	}
	stamped := styleGen
	if stamped == before {
		t.Fatal("installing the sheet stamped nothing")
	}
	settleAssets(t, f)
	if fontFaceOf("Late Face", 400) == nil {
		t.Fatal("the answer did not bring the family in")
	}
	if styleGen == stamped {
		t.Error("widgets were not told to restyle in the new face")
	}
}
