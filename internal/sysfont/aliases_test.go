package sysfont

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/typesetting/font"
	"github.com/stubbedev/lys-lib/third_party/typesetting/fontscan"
)

// faces builds system faces at the given aspects, located by order.
func faces(aspects ...font.Aspect) []systemFace {
	out := make([]systemFace, len(aspects))
	for i, a := range aspects {
		out[i] = systemFace{loc: fontscan.Location{File: "f", Index: uint16(i)}, aspect: a}
	}
	return out
}

func aspect(weight font.Weight, style font.Style) font.Aspect {
	return font.Aspect{Style: style, Weight: weight, Stretch: font.StretchNormal}
}

func TestClosestFaceFollowsTheCSSMatchingSteps(t *testing.T) {
	n, i := font.StyleNormal, font.StyleItalic
	for _, tc := range []struct {
		name string
		have []font.Aspect
		want font.Aspect
		pick uint16
	}{
		{"exact weight", []font.Aspect{aspect(300, n), aspect(500, n), aspect(700, n)}, aspect(500, n), 1},
		{"450 takes heavier up to 500", []font.Aspect{aspect(300, n), aspect(500, n), aspect(700, n)}, aspect(450, n), 1},
		{"500 falls to lighter before past 500", []font.Aspect{aspect(300, n), aspect(400, n), aspect(700, n)}, aspect(500, n), 1},
		{"300 takes lighter first", []font.Aspect{aspect(200, n), aspect(400, n)}, aspect(300, n), 0},
		{"300 without lighter takes heavier", []font.Aspect{aspect(400, n), aspect(700, n)}, aspect(300, n), 0},
		{"600 takes heavier first", []font.Aspect{aspect(400, n), aspect(800, n)}, aspect(600, n), 1},
		{"600 without heavier takes lighter", []font.Aspect{aspect(300, n), aspect(500, n)}, aspect(600, n), 1},
		{"style before weight", []font.Aspect{aspect(700, n), aspect(400, i)}, aspect(700, i), 1},
		{"italic missing falls to normal", []font.Aspect{aspect(400, n), aspect(700, n)}, aspect(700, i), 1},
		{"stretch before style", []font.Aspect{{Style: i, Weight: 400, Stretch: font.StretchCondensed}, aspect(400, n)}, aspect(400, i), 1},
	} {
		if got := closestFace(faces(tc.have...), tc.want).loc.Index; got != tc.pick {
			t.Errorf("%s: picked face %d (%v), want %d (%v)", tc.name, got, tc.have[got], tc.pick, tc.have[tc.pick])
		}
	}
}

func TestBuildAliasIndexFilesFacesUnderTheirAliases(t *testing.T) {
	regular := fontscan.Footprint{Location: fontscan.Location{File: "regular.ttf"}, Family: "canonical", Aliases: []string{"alias", "other"}}
	bold := fontscan.Footprint{Location: fontscan.Location{File: "bold.ttf"}, Family: "canonical", Aliases: []string{"alias"}}
	plain := fontscan.Footprint{Location: fontscan.Location{File: "plain.ttf"}, Family: "plain"}
	index := buildAliasIndex([]fontscan.Footprint{regular, bold, plain})
	if got := index["alias"]; len(got) != 2 || got[0].loc != regular.Location || got[1].loc != bold.Location {
		t.Errorf("index[alias] = %v, want both faces in order", got)
	}
	if got := index["other"]; len(got) != 1 || got[0].loc != regular.Location {
		t.Errorf("index[other] = %v, want the regular face", got)
	}
	if len(index) != 2 {
		t.Errorf("index has %d names, want 2", len(index))
	}
}

// TestWeightedResolvesAFamilyByAnotherOfItsNames pins the lookup
// path: a family the store indexes under a different name resolves
// to its own face instead of the matcher's substitute.
func TestWeightedResolvesAFamilyByAnotherOfItsNames(t *testing.T) {
	mustSans(t)
	storeMu.Lock()
	saved, savedBuilt := aliasIndex, aliasIndexBuilt
	aliasIndex = map[string][]systemFace{
		"lystestaliasfamily": faces(aspect(400, font.StyleNormal)),
	}
	aliasIndex["lystestaliasfamily"][0].loc = fontscan.Location{File: "testdata/Cantarell-Regular.ttf"}
	aliasIndexBuilt = true
	storeMu.Unlock()
	t.Cleanup(func() {
		storeMu.Lock()
		aliasIndex, aliasIndexBuilt = saved, savedBuilt
		storeMu.Unlock()
	})

	tf, err := Weighted("Lys Test Alias Family", 14, 400, false)
	if err != nil {
		t.Fatalf("Weighted: %v", err)
	}
	if got := font.NormalizeFamily(tf.Family()); got != "cantarell" {
		t.Errorf("alias resolved to %q, want the aliased cantarell face", tf.Family())
	}
	tf, err = Weighted("lys-ui Unknown Family", 14, 400, false)
	if err != nil {
		t.Fatalf("Weighted(unknown): %v", err)
	}
	if font.NormalizeFamily(tf.Family()) == "cantarell" {
		t.Error("an unknown family resolved to the alias face")
	}
}

// A family's display name keys to the name Families lists it under.
func TestFamilyKeyMatchesFamilies(t *testing.T) {
	if got := FamilyKey("JetBrains Mono"); got != font.NormalizeFamily("jetbrainsmono") || got != "jetbrainsmono" {
		t.Errorf("FamilyKey(JetBrains Mono) = %q, want jetbrainsmono", got)
	}
	fams, err := Families()
	if err != nil {
		t.Skip(err)
	}
	for _, f := range fams {
		if FamilyKey(FamilyDisplay(f)) != f {
			t.Errorf("family %q displays as %q, which keys to %q", f, FamilyDisplay(f), FamilyKey(FamilyDisplay(f)))
		}
	}
}
