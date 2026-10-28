package app

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/dbustest"
	"github.com/stubbedev/lys-lib/render"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/widget"
)

type mockPrint struct {
	cancel   bool
	settings []map[string]dbus.Variant
	setups   []map[string]dbus.Variant
	tokens   []uint32
	docs     [][]byte
}

type printIface struct{ d *mockDesktop }

func (p printIface) PreparePrint(sender dbus.Sender, _, _ string, settings, setup, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	p.d.mu.Lock()
	p.d.print.settings = append(p.d.print.settings, settings)
	p.d.print.setups = append(p.d.print.setups, setup)
	cancel := p.d.print.cancel
	p.d.mu.Unlock()
	path := p.d.handle(sender, opts)
	if cancel {
		p.d.respond(sender, path, 1, map[string]dbus.Variant{})
		return path, nil
	}
	p.d.respond(sender, path, 0, map[string]dbus.Variant{
		"token":    dbus.MakeVariant(uint32(7)),
		"settings": dbus.MakeVariant(map[string]dbus.Variant{"printer": dbus.MakeVariant("office"), "n-copies": dbus.MakeVariant("2")}),
		"page-setup": dbus.MakeVariant(map[string]dbus.Variant{
			"Name": dbus.MakeVariant("na_letter"), "Width": dbus.MakeVariant(215.9), "Height": dbus.MakeVariant(279.4),
			"MarginTop": dbus.MakeVariant(10.0), "MarginBottom": dbus.MakeVariant(10.0),
			"MarginLeft": dbus.MakeVariant(10.0), "MarginRight": dbus.MakeVariant(10.0),
			"Orientation": dbus.MakeVariant("landscape"),
		}),
	})
	return path, nil
}

func (p printIface) Print(sender dbus.Sender, _, _ string, fd dbus.UnixFD, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	f := os.NewFile(uintptr(fd), "print")
	doc, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		p.d.t.Error(err)
	}
	token, _ := opts["token"].Value().(uint32)
	p.d.mu.Lock()
	p.d.print.docs = append(p.d.print.docs, doc)
	p.d.print.tokens = append(p.d.print.tokens, token)
	p.d.mu.Unlock()
	path := p.d.handle(sender, opts)
	p.d.respond(sender, path, 0, map[string]dbus.Variant{})
	return path, nil
}

var a4Margins = PageSetup{Paper: PaperA4, Margins: PageMargins{Top: 20, Bottom: 20, Left: 10, Right: 10}}

func TestPageLayoutIsInCSSPixels(t *testing.T) {
	l, err := newPageLayout(a4Margins, 300)
	if err != nil {
		t.Fatal(err)
	}
	if l.Size != (widget.Size{W: 794, H: 1123}) || l.Printable != (render.Rect{X: 38, Y: 76, W: 718, H: 971}) {
		t.Errorf("A4 portrait: size %v printable %v", l.Size, l.Printable)
	}
	turned := a4Margins
	turned.Orientation = Landscape
	if l, _ = newPageLayout(turned, 300); l.Size != (widget.Size{W: 1123, H: 794}) {
		t.Errorf("A4 landscape: size %v", l.Size)
	}
	for _, bad := range []PageSetup{
		{},
		{Paper: Paper{Width: 100, Height: -1}},
		{Paper: PaperA4, Margins: PageMargins{Left: 105, Right: 105}},
		{Paper: PaperA4, Margins: PageMargins{Top: -1}},
	} {
		if _, err := newPageLayout(bad, 300); err == nil {
			t.Errorf("setup %+v was accepted", bad)
		}
	}
	if _, err := newPageLayout(a4Margins, -1); err == nil {
		t.Error("a negative resolution was accepted")
	}
}

func paintedJob(pages int) PrintJob {
	return PrintJob{
		Title: "Report",
		Setup: a4Margins,
		DPI:   96,
		Pages: func(PageLayout) int { return pages },
		Paint: func(cv *render.Canvas, page int, l PageLayout) {
			cv.FillRect(l.Printable, []render.Color{render.RGB(0, 0, 255), render.RGB(255, 0, 0)}[page%2])
		},
	}
}

func pagePixels(t *testing.T, doc []byte) [][]byte {
	t.Helper()
	var pages [][]byte
	for _, m := range regexp.MustCompile(`(?s)/Subtype /Image .*?stream\n(.*?)\nendstream`).FindAllSubmatch(doc, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			t.Fatal(err)
		}
		px, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, px)
	}
	return pages
}

func rgbAt(px []byte, width, x, y int) [3]byte {
	i := 3 * (y*width + x)
	return [3]byte{px[i], px[i+1], px[i+2]}
}

func TestExportPDFPaintsEachPageOnTheSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := ExportPDF(path, paintedJob(2)); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(doc, []byte("/MediaBox [0 0 595.2755905511812 841.8897637795276]")) || !bytes.Contains(doc, []byte("/Count 2")) {
		t.Fatal("the document is not two A4 pages")
	}
	pages := pagePixels(t, doc)
	if len(pages) != 2 {
		t.Fatalf("%d page images", len(pages))
	}
	white, blue, red := [3]byte{255, 255, 255}, [3]byte{0, 0, 255}, [3]byte{255, 0, 0}
	for i, want := range [][3]byte{blue, red} {
		if got := rgbAt(pages[i], 794, 5, 5); got != white {
			t.Errorf("page %d margin is %v", i+1, got)
		}
		if got := rgbAt(pages[i], 794, 400, 500); got != want {
			t.Errorf("page %d body is %v, want %v", i+1, got, want)
		}
	}
}

func TestFailedExportLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	for name, job := range map[string]PrintJob{
		"no pages":  paintedJob(0),
		"no paint":  {Setup: a4Margins, Pages: func(PageLayout) int { return 1 }},
		"bad setup": {Pages: func(PageLayout) int { return 1 }, Paint: func(*render.Canvas, int, PageLayout) {}},
	} {
		if err := ExportPDF(filepath.Join(dir, "out.pdf"), job); err == nil {
			t.Errorf("%s: export succeeded", name)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("failed exports left %d files", len(entries))
	}
}

func TestPrintRendersOnTheConfirmedSetupAndRemembersIt(t *testing.T) {
	d := startMockDesktop(t)
	a := testApp(nil)
	defer a.desktop().close()
	print := func() error {
		var result *error
		a.Print(nil, paintedJob(1), func(err error) { result = &err })
		pumpUntil(t, a, func() bool { return result != nil })
		return *result
	}
	if err := print(); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	first := d.print.setups[0]
	doc, token := d.print.docs[0], d.print.tokens[0]
	d.mu.Unlock()
	if first["Name"].Value() != "iso_a4" || first["Orientation"].Value() != "portrait" {
		t.Errorf("the dialog did not start from the job's setup: %v", first)
	}
	if token != 7 {
		t.Errorf("Print carried token %d, want PreparePrint's 7", token)
	}
	if !bytes.HasPrefix(doc, []byte("%PDF-1.4")) || !bytes.Contains(doc, []byte("/MediaBox [0 0 792 612]")) {
		t.Errorf("the spooled document is not one landscape Letter page: %.80q", doc)
	}
	if pages := pagePixels(t, doc); len(pages) != 1 || rgbAt(pages[0], 1056, 528, 408) != [3]byte{0, 0, 255} {
		t.Error("the spooled page was not painted at the confirmed setup")
	}
	if err := print(); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	second, settings := d.print.setups[1], d.print.settings[1]
	d.print.cancel = true
	d.mu.Unlock()
	if second["Name"].Value() != "na_letter" || second["Orientation"].Value() != "landscape" || settings["printer"].Value() != "office" {
		t.Errorf("the second dialog did not start from the first's choices: %v %v", second, settings)
	}
	if err := print(); !errors.Is(err, ErrPortalDenied) {
		t.Errorf("a cancelled dialog returned %v", err)
	}
	d.mu.Lock()
	printed := len(d.print.docs)
	d.mu.Unlock()
	if printed != 2 {
		t.Errorf("%d documents reached the portal, want 2", printed)
	}
}

func TestPrintWithoutAPortalSaysSo(t *testing.T) {
	dbustest.Session(t)
	a := testApp(nil)
	defer a.desktop().close()
	var result *error
	a.Print(nil, paintedJob(1), func(err error) { result = &err })
	pumpUntil(t, a, func() bool { return result != nil })
	if !errors.Is(*result, ErrPortalUnavailable) || !strings.Contains((*result).Error(), "PreparePrint") {
		t.Errorf("printing without a portal returned %v", *result)
	}
}
