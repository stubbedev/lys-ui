package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/i18n"
)

// TestMessageCatalog pins the catalog contract: Tr is the identity
// without a catalog, the catalog translates what it knows and passes
// through what it does not, an empty translation falls back to the
// English key, and installing nil restores the identity.
func TestMessageCatalog(t *testing.T) {
	defer SetMessageCatalog(nil)

	if got := Tr("OK"); got != "OK" {
		t.Errorf("identity Tr = %q", got)
	}

	SetMessageCatalog(i18n.FromMap(map[string]string{"OK": "OK", "Cancel": "Avbryt", "broken": ""}))
	if got := Tr("Cancel"); got != "Avbryt" {
		t.Errorf("catalog Tr = %q, want Avbryt", got)
	}
	if got := Tr("Pick a color"); got != "Pick a color" {
		t.Errorf("unknown key translated itself: %q", got)
	}
	if got := Tr("broken"); got != "broken" {
		t.Errorf("empty translation did not fall back: %q", got)
	}

	SetMessageCatalog(nil)
	if got := Tr("Cancel"); got != "Cancel" {
		t.Errorf("nil did not restore the identity: %q", got)
	}
}

// TestChooserLabelsFlowThroughCatalog proves the built-in surface, not
// just the hook: the file chooser's places row and its title carry the
// catalog's answers once it is installed.
func TestChooserLabelsFlowThroughCatalog(t *testing.T) {
	face := bindingFace(t)
	defer SetMessageCatalog(nil)
	SetMessageCatalog(i18n.FromMap(map[string]string{
		"Up":           "Opp",
		"Home":         "Hjem",
		"Recent files": "Nylige filer",
	}))

	c := NewFileChooser(face, 14, FileModeOpen, "/home", nil)
	c.SetSource(func(string) ([]FileEntry, error) { return nil, nil })
	c.navigate("")
	if got := c.dirLabel(); got != "Nylige filer" {
		t.Errorf("recents title = %q, want the catalog's", got)
	}
}

const polishCatalog = `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=3; plural=(n==1 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2);\n"

msgctxt "menu"
msgid "Open"
msgstr "Otwórz…"

msgid "%d file"
msgid_plural "%d files"
msgstr[0] "%d plik"
msgstr[1] "%d pliki"
msgstr[2] "%d plików"
`

func TestTrNAndTrCtxRouteThroughTheCatalog(t *testing.T) {
	defer SetMessageCatalog(nil)
	if TrN("%d file", "%d files", 1) != "%d file" || TrN("%d file", "%d files", 3) != "%d files" || TrCtx("menu", "Open") != "Open" {
		t.Error("the identity catalog does not pick English forms")
	}
	c, err := i18n.ParsePO([]byte(polishCatalog))
	if err != nil {
		t.Fatal(err)
	}
	SetMessageCatalog(c)
	if got := TrN("%d file", "%d files", 5); got != "%d plików" {
		t.Errorf("TrN(5) = %q", got)
	}
	if got := TrCtx("menu", "Open"); got != "Otwórz…" {
		t.Errorf("TrCtx = %q", got)
	}
}

var _ Catalog = (*i18n.Catalog)(nil)
