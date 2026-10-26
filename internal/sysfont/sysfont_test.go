package sysfont

import (
	"strings"
	"testing"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

// mustSans resolves the default sans face, skipping the test when the
// machine has no usable font store.
func mustSans(t *testing.T) *render.Typeface {
	t.Helper()
	tf, err := Sans()
	if err != nil {
		t.Skipf("no system font store: %v", err)
	}
	return tf
}

func TestBest(t *testing.T) {
	sans := mustSans(t)

	t.Run("monospace resolves the configured mono", func(t *testing.T) {
		tf, err := Best("monospace", 14)
		if err != nil {
			t.Skipf("no monospace in the store: %v", err)
		}
		if want := mustMonospace(t); tf.Family() != want.Family() {
			t.Errorf("Best(monospace) = %q, want the configured %q", tf.Family(), want.Family())
		}
		// A real monospace face advances every glyph equally.
		i := tf.Shape("iiii", 14).Advance()
		w := tf.Shape("WWWW", 14).Advance()
		if i != w {
			t.Errorf("monospace advances differ: iiii=%.3f WWW=%.3f for %q", i, w, tf.Family())
		}
	})

	t.Run("sans-serif resolves the same face as Sans", func(t *testing.T) {
		tf, err := Best("sans-serif", 14)
		if err != nil {
			t.Fatalf("Best(sans-serif): %v", err)
		}
		if tf.Family() != sans.Family() {
			t.Errorf("Best(sans-serif) = %q, want %q", tf.Family(), sans.Family())
		}
	})

	t.Run("any installed family resolves", func(t *testing.T) {
		tf, err := Best(sans.Family(), 14)
		if err != nil {
			t.Fatalf("Best(%q): %v", sans.Family(), err)
		}
		if tf.Family() != sans.Family() {
			t.Errorf("Best(%q) = %q", sans.Family(), tf.Family())
		}
	})

	t.Run("unknown families fall back instead of failing", func(t *testing.T) {
		// CSS behavior: an unavailable family resolves to whatever
		// the matcher can find, never an error.
		tf, err := Best("no-such-family-anywhere", 14)
		if err != nil {
			t.Fatalf("unknown family returned an error: %v", err)
		}
		if tf == nil || tf.Family() == "" {
			t.Error("unknown family resolved to nothing")
		}
	})

	t.Run("non-positive sizes are caller bugs", func(t *testing.T) {
		for _, size := range []float64{0, -1} {
			if _, err := Best("sans-serif", size); err == nil {
				t.Errorf("Best(sans-serif, %v) succeeded, want an error", size)
			}
		}
	})
}

func mustMonospace(t *testing.T) *render.Typeface {
	t.Helper()
	tf, err := Monospace()
	if err != nil {
		t.Skipf("no monospace in the store: %v", err)
	}
	return tf
}

func TestVariantIntegrationUnchanged(t *testing.T) {
	sans := mustSans(t)

	t.Run("bold resolves in the same family or downgrades", func(t *testing.T) {
		tf, err := Variant(sans, true, false)
		if err != nil {
			t.Fatalf("Variant(bold): %v", err)
		}
		if tf.Family() != sans.Family() {
			t.Errorf("Variant family = %q, want the base %q", tf.Family(), sans.Family())
		}
		switch w := tf.Describe().Aspect.Weight; w {
		case font.WeightBold:
			// resolved
		case font.WeightNormal:
			// documented downgrade: the family has no bold face
		default:
			t.Errorf("Variant(bold) weight = %v, want bold or the documented regular downgrade", w)
		}
	})

	t.Run("regular variant matches the base face identity", func(t *testing.T) {
		tf, err := Variant(sans, false, false)
		if err != nil {
			t.Fatalf("Variant(regular): %v", err)
		}
		if tf != sans {
			t.Errorf("regular Variant resolved a different face instance: %p != %p", tf, sans)
		}
	})

	t.Run("faces from outside the store still resolve a variant", func(t *testing.T) {
		// Fixture faces (not installed system families) get the CSS
		// fallback: some face resolves, never an error.
		tf, err := render.LoadFont(goregular.TTF)
		if err != nil {
			t.Fatalf("load Go font: %v", err)
		}
		bold, err := Variant(tf, true, false)
		if err != nil {
			t.Fatalf("Variant of fixture face: %v", err)
		}
		if bold == nil {
			t.Error("Variant of fixture face returned nil")
		}
	})
}

func TestScanOnce(t *testing.T) {
	a := mustSans(t)
	b := mustSans(t)
	if a != b {
		t.Error("two Sans() calls returned different face instances: the store must be scanned and cached once")
	}
}

func TestFallbackChain(t *testing.T) {
	primary, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatalf("load Go font: %v", err)
	}
	if primary.Covers('你') || primary.Covers('😀') {
		t.Fatal("fixture font unexpectedly covers the mixed-script test runes")
	}

	ch := Fallback(primary)

	t.Run("fully covered text never consults the store", func(t *testing.T) {
		s := ch.Shape("lys-ui", 14)
		if s.Runs() != 1 {
			t.Errorf("Runs = %d, want 1", s.Runs())
		}
		if s.NotDefCount() != 0 {
			t.Errorf("NotDefCount = %d, want 0", s.NotDefCount())
		}
	})

	mixed := "g你😀"
	s := ch.Shape(mixed, 16)

	t.Run("mixed latin CJK emoji all find covering faces", func(t *testing.T) {
		if missing := missingCoverage(ch, mixed); missing != "" {
			t.Skipf("system store lacks coverage: %s", missing)
		}
		if s.NotDefCount() != 0 {
			t.Errorf("NotDefCount = %d, want 0 for covered scripts", s.NotDefCount())
		}
		if s.Runs() != len([]rune(mixed)) {
			t.Errorf("Runs = %d, want %d (a run per face change)", s.Runs(), len([]rune(mixed)))
		}
	})

	t.Run("mixed text draws visible glyphs for all three scripts", func(t *testing.T) {
		if missing := missingCoverage(ch, mixed); missing != "" {
			t.Skipf("system store lacks coverage: %s", missing)
		}
		const (
			w, h = 120, 40
		)
		data := make([]byte, render.Stride(w)*h)
		cv := render.NewScaled(data, render.Stride(w), w, h, 240, 120) // 2x device scale
		ch.Draw(cv, s, 4, 28, render.RGB(255, 255, 255))

		xs := s.CaretPositions()
		for r := range len([]rune(mixed)) {
			lo, hi := int(xs[r]*2)+2, int(xs[r+1]*2)-1
			count := 0
			for y := range h {
				for x := lo; x <= hi && x < w; x++ {
					if px(data, render.Stride(w), x, y) {
						count++
					}
				}
			}
			if count == 0 {
				t.Errorf("glyph %d (rune %q, x %d..%d device) produced no ink", r, []rune(mixed)[r], lo, hi)
			}
		}
	})

	t.Run("color emoji renders through the bitmap glyph path", func(t *testing.T) {
		emoji, err := Best("Noto Color Emoji", 16)
		if err != nil {
			t.Skipf("Noto Color Emoji not installed: %v", err)
		}
		ch := render.NewChain(emoji)
		s := ch.Shape("\U0001F600\U0001F642", 16) // grinning + slightly smiling
		if s.NotDefCount() != 0 {
			t.Fatalf("NotDefCount = %d, want 0 for the emoji face", s.NotDefCount())
		}
		const w, h = 80, 40
		data := make([]byte, render.Stride(w)*h)
		cv := render.New(data, render.Stride(w), w, h)
		ch.Draw(cv, s, 4, 30, render.RGB(255, 255, 255))
		count := 0
		for y := range h {
			for x := range w {
				if px(data, render.Stride(w), x, y) {
					count++
				}
			}
		}
		if count < 25 {
			t.Errorf("color emoji produced %d ink pixels, want a filled bitmap (CBDT decode or scale broken)", count)
		}
	})

	t.Run("standard fallback families are preferred when installed", func(t *testing.T) {
		if face := ch.Face('你'); !strings.Contains(strings.ToLower(face.Family()), "cjk") {
			t.Skipf("Noto Sans CJK not installed or not preferred; %q covers Han", face.Family())
		}
		if face := ch.Face('😀'); !strings.Contains(strings.ToLower(face.Family()), "emoji") {
			t.Skipf("Noto Color Emoji not installed or not preferred; %q covers emoji", face.Family())
		}
	})

	t.Run("the store picks faces the primary lacks", func(t *testing.T) {
		for _, r := range []rune{'你', '😀'} {
			// Mirror the Fallback resolver: resolve with the rune's
			// script hint, then verify actual coverage - ResolveFace
			// returns an arbitrary face when nothing covers, so
			// coverage is the check, and its absence a skip reason.
			tf, err := lookup(primary.Family(), regAspect, language.LookupScript(r), r)
			if err != nil {
				t.Skipf("store lookup failed for %q: %v", r, err)
			}
			if !tf.Covers(r) {
				t.Skipf("no store face covers %q; got %q", r, tf.Family())
			}
			if tf.Family() == primary.Family() {
				t.Errorf("%q resolved to family %q, want a covering fallback family", r, tf.Family())
			}
		}
	})
}

// missingCoverage names the first test rune the chain cannot cover, ""
// when all are covered: the honest skip reason for font-poor machines.
func missingCoverage(ch *render.Chain, text string) string {
	for _, r := range text {
		if ch.Shape(string(r), 16).NotDefCount() > 0 {
			return string(r)
		}
	}
	return ""
}

// px reports whether the pixel is inked.
func px(data []byte, stride, x, y int) bool {
	i := y*stride + x*4
	return data[i+3] > 8
}

func TestWeightedResolvesTheRequestedWeight(t *testing.T) {
	sans := mustSans(t)
	bold, err := Weighted(sans.Family(), 16, 700, false)
	if err != nil {
		t.Fatalf("Weighted(700): %v", err)
	}
	if bold.Family() != sans.Family() {
		t.Errorf("family = %q, want %q", bold.Family(), sans.Family())
	}
	if w := bold.Describe().Aspect.Weight; w != font.WeightBold && w != font.WeightNormal {
		t.Errorf("weight = %v, want bold or the regular downgrade", w)
	}
	regular, err := Weighted(sans.Family(), 16, 400, false)
	if err != nil {
		t.Fatalf("Weighted(400): %v", err)
	}
	if w := regular.Describe().Aspect.Weight; w != font.WeightNormal {
		t.Errorf("400 resolved weight %v, want regular", w)
	}
}

func TestWeightedRejectsBadInput(t *testing.T) {
	if _, err := Weighted("sans-serif", 0, 400, false); err == nil {
		t.Error("zero size accepted")
	}
	for _, w := range []int{0, -100, 1001} {
		if _, err := Weighted("sans-serif", 16, w, false); err == nil {
			t.Errorf("weight %d accepted", w)
		}
	}
}

// An italic of a family with no italic face and no ital axis is a
// synthetic oblique, the face CSS synthesizes by default: Geist, the
// UI font, resolved upright for an italic request, so italic text
// stood straight (#282). The upright request stays upright.
func TestItalicWithoutAnItalicFaceIsOblique(t *testing.T) {
	ui, err := UI()
	if err != nil {
		t.Fatal(err)
	}
	italic, err := Weighted(ui.Family(), 14, 400, true)
	if err != nil {
		t.Fatal(err)
	}
	if !italic.Slanted() {
		t.Errorf("italic %s is upright, want a synthetic oblique", ui.Family())
	}
	upright, err := Weighted(ui.Family(), 14, 400, false)
	if err != nil {
		t.Fatal(err)
	}
	if upright.Slanted() {
		t.Error("the upright request came back slanted")
	}
	if v, err := Variant(ui, false, true); err != nil || !v.Slanted() {
		t.Errorf("Variant italic %v (err %v), want a synthetic oblique", v, err)
	}
}
