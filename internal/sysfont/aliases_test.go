package sysfont

import (
	"encoding/binary"
	"errors"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
)

// nameRecord is one record for nameTable.
type nameRecord struct {
	platform, encoding, language, id uint16
	value                            string
}

// nameTable encodes records as a format-0 'name' table: Unicode and
// Windows strings as UTF-16BE, everything else as raw bytes.
func nameTable(records ...nameRecord) []byte {
	var storage []byte
	head := make([]byte, 6+12*len(records))
	binary.BigEndian.PutUint16(head[2:], uint16(len(records)))
	binary.BigEndian.PutUint16(head[4:], uint16(len(head)))
	for i, r := range records {
		var b []byte
		if r.platform == 0 || r.platform == 3 {
			for _, u := range utf16.Encode([]rune(r.value)) {
				b = binary.BigEndian.AppendUint16(b, u)
			}
		} else {
			b = []byte(r.value)
		}
		rec := head[6+12*i:]
		binary.BigEndian.PutUint16(rec, r.platform)
		binary.BigEndian.PutUint16(rec[2:], r.encoding)
		binary.BigEndian.PutUint16(rec[4:], r.language)
		binary.BigEndian.PutUint16(rec[6:], r.id)
		binary.BigEndian.PutUint16(rec[8:], uint16(len(b)))
		binary.BigEndian.PutUint16(rec[10:], uint16(len(storage)))
		storage = append(storage, b...)
	}
	return append(head, storage...)
}

func TestFamilyNamesReadsEveryFamilyRecord(t *testing.T) {
	raw := nameTable(
		nameRecord{1, 0, 0, 1, "JetBrainsMono NF Medium"},
		nameRecord{3, 1, 1033, 16, "JetBrainsMono Nerd Font"},
		nameRecord{3, 1, 2057, 16, "JetBrainsMono NF"},
		nameRecord{3, 1, 1033, 21, "JetBrainsMono WWS"},
		nameRecord{3, 1, 1033, 1, "JetBrainsMono NF Medium"},
		nameRecord{3, 1, 1033, 4, "JetBrainsMono NF Medium Full"},
		nameRecord{1, 1, 0, 1, "Japanese-encoded"},
	)
	got, err := familyNames(raw)
	if err != nil {
		t.Fatalf("familyNames: %v", err)
	}
	want := []string{"jetbrainsmononfmedium", "jetbrainsmononerdfont", "jetbrainsmononf", "jetbrainsmonowws"}
	if !slices.Equal(got, want) {
		t.Errorf("familyNames = %q, want %q (families 1/16/21 on every platform and language, deduped; the full name and unconvertible Mac encodings out)", got, want)
	}
}

func TestFamilyNamesRejectsMalformedTables(t *testing.T) {
	good := nameTable(nameRecord{3, 1, 1033, 1, "Family"})
	for name, raw := range map[string][]byte{
		"short header":     good[:4],
		"truncated record": good[:10],
		"string past end":  good[:len(good)-2],
	} {
		if _, err := familyNames(raw); !errors.Is(err, errNameTable) {
			t.Errorf("%s: err = %v, want errNameTable", name, err)
		}
	}
}

// faces builds alias faces at the given aspects, located by order.
func faces(aspects ...font.Aspect) []aliasFace {
	out := make([]aliasFace, len(aspects))
	for i, a := range aspects {
		out[i] = aliasFace{loc: fontscan.Location{File: "f", Index: uint16(i)}, aspect: a}
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

func TestBuildAliasIndexSkipsUnreadableFaces(t *testing.T) {
	ok := fontscan.Footprint{Location: fontscan.Location{File: "ok.ttf"}, Aspect: aspect(400, font.StyleNormal)}
	bad := fontscan.Footprint{Location: fontscan.Location{File: "bad.ttf"}}
	index := buildAliasIndex([]fontscan.Footprint{bad, ok}, func(loc fontscan.Location) ([]string, error) {
		if loc.File == "bad.ttf" {
			return nil, errNameTable
		}
		return []string{"alias", "canonical"}, nil
	})
	for _, name := range []string{"alias", "canonical"} {
		if got := index[name]; len(got) != 1 || got[0].loc != ok.Location {
			t.Errorf("index[%q] = %v, want the readable face only", name, got)
		}
	}
	if len(index) != 2 {
		t.Errorf("index has %d names, want 2", len(index))
	}
}

func TestFileFamilyNamesReadsAFontFile(t *testing.T) {
	names, err := fileFamilyNames(fontscan.Location{File: "../../render/testdata/Cantarell-Regular.ttf"})
	if err != nil {
		t.Fatalf("fileFamilyNames: %v", err)
	}
	if !slices.Contains(names, "cantarell") {
		t.Errorf("names = %q, want cantarell among them", names)
	}
	if _, err := fileFamilyNames(fontscan.Location{File: "../../render/testdata/Cantarell-Regular.ttf", Index: 3}); err == nil {
		t.Error("an out-of-range face index read names")
	}
}

// TestWeightedResolvesAFamilyByAnotherOfItsNames pins the lookup
// path: a family the store indexes under a different name resolves
// to its own face instead of the matcher's substitute.
func TestWeightedResolvesAFamilyByAnotherOfItsNames(t *testing.T) {
	mustSans(t)
	storeMu.Lock()
	saved, savedBuilt := aliasIndex, aliasIndexBuilt
	aliasIndex = map[string][]aliasFace{
		"lystestaliasfamily": faces(aspect(400, font.StyleNormal)),
	}
	aliasIndex["lystestaliasfamily"][0].loc = fontscan.Location{File: "../../render/testdata/Cantarell-Regular.ttf"}
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
