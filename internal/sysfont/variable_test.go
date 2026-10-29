package sysfont

import (
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/typesetting/font"

	"github.com/stubbedev/lys-ui/internal/uifont"
)

// A variable face the store matched at its default instance serves a
// requested weight through its wght axis; an already-matching weight
// and a static face come back unchanged.
func TestInstantiateVariable(t *testing.T) {
	data, err := os.ReadFile("testdata/Quicksand-VF.ttf")
	if err != nil {
		t.Fatal(err)
	}
	vf, err := render.LoadFont(data)
	if err != nil {
		t.Fatal(err)
	}
	def := vf.Describe().Aspect.Weight
	bold := instantiate(vf, font.Aspect{Style: font.StyleNormal, Weight: font.WeightBold})
	if bold == vf || len(bold.Variations()) != 1 || bold.Variations()[0].Value != 700 {
		t.Errorf("bold = %v", bold.Variations())
	}
	if same := instantiate(vf, font.Aspect{Style: font.StyleNormal, Weight: def}); same != vf {
		t.Error("the described weight made an instance")
	}
	static, _ := render.NewFixtureTypeface()
	if instantiate(static, font.Aspect{Weight: font.WeightBold}) != static {
		t.Error("a static face was instanced")
	}
}

// The embedded UI faces resolve by name with nothing installed, and as
// variable fonts serve every weight through the wght axis (#194).
func TestEmbeddedUIFaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		get  func() (*render.Typeface, error)
		want string
	}{{"UI", UI, uifont.Family}, {"UIMono", UIMono, uifont.MonoFamily}} {
		tf, err := tc.get()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if tf.Family() != tc.want || !tf.HasAxis("wght") {
			t.Errorf("%s = %s (wght axis %v), want the variable %s", tc.name, tf.Family(), tf.HasAxis("wght"), tc.want)
		}
		byName, err := Best(tc.want, 14)
		if err != nil || byName != tf {
			t.Errorf("Best(%q) = %v, %v; want the embedded face", tc.want, byName, err)
		}
		medium, err := Weighted(tc.want, 14, 500, false)
		if err != nil {
			t.Fatal(err)
		}
		if vs := medium.Variations(); len(vs) != 1 || vs[0].Tag != "wght" || vs[0].Value != 500 {
			t.Errorf("%s at 500 = %v, want the wght 500 instance", tc.name, vs)
		}
	}
}
