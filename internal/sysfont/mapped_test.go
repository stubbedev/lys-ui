package sysfont

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/internal/uifont"
)

func isMapped(t *testing.T, path string) bool {
	t.Helper()
	maps, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(maps), path)
}

func TestAFontFileStaysMappedAsItsFaceReadsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "geist.ttf")
	if err := os.WriteFile(path, uifont.Sans, 0o600); err != nil {
		t.Fatal(err)
	}
	tf, err := FaceFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if tf == nil {
		t.Fatal("no typeface")
	}
	if !isMapped(t, path) {
		t.Error("the font file is not mapped: its tables were copied")
	}
}

func TestAFileThatIsNoFontIsAnErrorAndStaysUnmapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-font.ttf")
	if err := os.WriteFile(path, []byte("definitely not a font file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FaceFile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want one naming %s", err, path)
	}
	if isMapped(t, path) {
		t.Error("a file that failed to parse stayed mapped")
	}
}

func TestAMissingFontFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.ttf")
	if _, err := FaceFile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want one naming %s", err, path)
	}
}
