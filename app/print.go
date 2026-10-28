package app

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/stubbedev/lys-lib/atomicfile"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/pdf"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	portalPrintIface   = "org.freedesktop.portal.Print"
	portalPreparePrint = portalPrintIface + ".PreparePrint"
	portalPrint        = portalPrintIface + ".Print"

	cssDPI    = 96
	mmPerInch = 25.4
)

// DefaultPrintDPI is the raster resolution of a PrintJob that sets
// none.
const DefaultPrintDPI = 300

// Paper is a paper size in portrait, in millimetres.
type Paper struct {
	// Name is the PWG 5101.1 name, such as "iso_a4"; the print dialog
	// preselects it.
	Name          string
	Width, Height float64
}

var (
	// PaperA4 is ISO A4, 210 x 297 mm.
	PaperA4 = Paper{Name: "iso_a4", Width: 210, Height: 297}
	// PaperLetter is US Letter, 8.5 x 11 in.
	PaperLetter = Paper{Name: "na_letter", Width: 215.9, Height: 279.4}
)

// PageOrientation is how the page sits on the paper.
type PageOrientation uint8

const (
	// Portrait is the paper upright.
	Portrait PageOrientation = iota
	// Landscape turns the paper a quarter counterclockwise.
	Landscape
	// ReversePortrait is portrait upside down.
	ReversePortrait
	// ReverseLandscape is landscape upside down.
	ReverseLandscape
)

var orientationNames = [...]string{"portrait", "landscape", "reverse_portrait", "reverse_landscape"}

func (o PageOrientation) String() string { return orientationNames[o] }

func parseOrientation(s string) (PageOrientation, error) {
	for i, name := range orientationNames {
		if name == s {
			return PageOrientation(i), nil
		}
	}
	return 0, fmt.Errorf("app: print: unknown page orientation %q", s)
}

func (o PageOrientation) sideways() bool { return o == Landscape || o == ReverseLandscape }

// PageMargins are a page's margins in millimetres, on the page as
// oriented.
type PageMargins struct{ Top, Bottom, Left, Right float64 }

// PageSetup is the paper, orientation and margins pages are laid out
// on.
type PageSetup struct {
	Paper       Paper
	Orientation PageOrientation
	Margins     PageMargins
}

func (s PageSetup) validate() error {
	if !(s.Paper.Width > 0 && s.Paper.Height > 0) {
		return fmt.Errorf("app: print: paper %q is %vx%v mm, not a positive size", s.Paper.Name, s.Paper.Width, s.Paper.Height)
	}
	w, h := s.sizeMM()
	m := s.Margins
	if !(m.Top >= 0 && m.Bottom >= 0 && m.Left >= 0 && m.Right >= 0) || m.Left+m.Right >= w || m.Top+m.Bottom >= h {
		return fmt.Errorf("app: print: margins %+v mm leave no printable area on a %vx%v mm page", m, w, h)
	}
	return nil
}

func (s PageSetup) sizeMM() (w, h float64) {
	if s.Orientation.sideways() {
		return s.Paper.Height, s.Paper.Width
	}
	return s.Paper.Width, s.Paper.Height
}

// PageLayout is what a PrintJob paints against: sizes in logical
// pixels, 1/96 inch as in CSS, so a widget tree laid out at Size prints
// at its on-screen size.
type PageLayout struct {
	Setup PageSetup
	// DPI is the raster resolution, device pixels per inch.
	DPI int
	// Size is the whole page.
	Size widget.Size
	// Printable is the area inside the margins.
	Printable render.Rect
}

func newPageLayout(setup PageSetup, dpi int) (PageLayout, error) {
	if err := setup.validate(); err != nil {
		return PageLayout{}, err
	}
	if dpi <= 0 {
		return PageLayout{}, fmt.Errorf("app: print: resolution %d dpi is not positive", dpi)
	}
	px := func(mm float64) int { return int(math.Round(mm / mmPerInch * cssDPI)) }
	w, h := setup.sizeMM()
	m := setup.Margins
	return PageLayout{
		Setup: setup,
		DPI:   dpi,
		Size:  widget.Size{W: px(w), H: px(h)},
		Printable: render.Rect{
			X: px(m.Left), Y: px(m.Top),
			W: px(w) - px(m.Left) - px(m.Right), H: px(h) - px(m.Top) - px(m.Bottom),
		},
	}, nil
}

// PrintJob describes a document for Print or ExportPDF.
type PrintJob struct {
	// Title names the document in the print dialog, the queue and the
	// PDF.
	Title string
	// Setup is the page setup ExportPDF uses and the print dialog
	// starts from; Print uses the setup the user confirms.
	Setup PageSetup
	// DPI is the raster resolution; zero is DefaultPrintDPI.
	DPI int
	// Pages returns how many pages the document has on layout. It runs
	// on the caller's goroutine (the loop for Print).
	Pages func(layout PageLayout) int
	// Paint draws page (from 0) on cv, which starts transparent and
	// prints as white. cv is in logical pixels: layout.Size covers the
	// page. It runs where Pages runs.
	Paint func(cv *render.Canvas, page int, layout PageLayout)
}

func (j PrintJob) dpi() int {
	if j.DPI == 0 {
		return DefaultPrintDPI
	}
	return j.DPI
}

func (j PrintJob) writePDF(out io.Writer, setup PageSetup) error {
	if j.Pages == nil || j.Paint == nil {
		return errors.New("app: print: the job needs both Pages and Paint")
	}
	layout, err := newPageLayout(setup, j.dpi())
	if err != nil {
		return err
	}
	n := j.Pages(layout)
	if n < 1 {
		return fmt.Errorf("app: print %q: Pages returned %d, want at least one", j.Title, n)
	}
	wmm, hmm := setup.sizeMM()
	device := func(mm float64) int { return int(math.Round(mm / mmPerInch * float64(layout.DPI))) }
	dw, dh := device(wmm), device(hmm)
	data := make([]byte, render.Stride(dw)*dh)
	page := image.NewRGBA(image.Rect(0, 0, dw, dh))
	doc := pdf.NewWriter(out, j.Title)
	size := pdf.Size{W: pdf.Millimetres(wmm), H: pdf.Millimetres(hmm)}
	for i := range n {
		clear(data)
		j.Paint(render.NewScaled(data, render.Stride(dw), dw, dh, layout.DPI, cssDPI), i, layout)
		canvasToRGBA(page.Pix, data)
		if err := doc.AddPage(size, page); err != nil {
			return fmt.Errorf("app: print %q page %d: %w", j.Title, i+1, err)
		}
	}
	if err := doc.Close(); err != nil {
		return fmt.Errorf("app: print %q: %w", j.Title, err)
	}
	return nil
}

func canvasToRGBA(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], src[i+3]
	}
}

// ExportPDF renders job on job.Setup into a PDF at path, replacing it
// atomically. It runs Pages and Paint on the caller's goroutine.
func ExportPDF(path string, job PrintJob) error {
	return atomicfile.Write(path, 0o644, func(w io.Writer) error { return job.writePDF(w, job.Setup) })
}

// Print prints job through the xdg-desktop-portal Print portal (GTK's
// GtkPrintDialog): the portal's dialog picks the printer and page
// setup, lys-ui renders the pages on the loop into a PDF at that setup,
// and the portal sends it to the printer. parent (nil for none) is the
// window the dialog belongs to. done runs on the loop with nil once
// the job is queued, ErrPortalDenied when the user cancels,
// ErrPortalUnavailable without a print portal, or the failure. The
// next Print starts from the settings the user chose.
func (a *Application) Print(parent *Window, job PrintJob, done func(error)) {
	d := a.desktop()
	window := portalParent(parent)
	d.mu.Lock()
	settings, setup := d.printSettings, job.Setup
	if d.printSetup != nil {
		setup = *d.printSetup
	}
	d.mu.Unlock()
	finish := func(err error) { a.Invoke(func() { done(err) }) }
	go func() {
		prepared, err := d.preparePrint(window, job.Title, settings, setup)
		if err != nil {
			finish(err)
			return
		}
		a.Invoke(func() {
			f, err := spoolPDF(job, prepared.setup)
			if err != nil {
				done(err)
				return
			}
			go func() {
				defer f.Close()
				finish(d.print(window, job.Title, f, prepared.token))
			}()
		})
	}()
}

func spoolPDF(job PrintJob, setup PageSetup) (*os.File, error) {
	f, err := os.CreateTemp("", "lys-ui-print-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("app: print %q: spool file: %w", job.Title, err)
	}
	if err := os.Remove(f.Name()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("app: print %q: unlink the spool file: %w", job.Title, err)
	}
	if err := job.writePDF(f, setup); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("app: print %q: rewind the spool file: %w", job.Title, err)
	}
	return f, nil
}

type preparedPrint struct {
	setup PageSetup
	token uint32
}

func (d *desktopPortals) preparePrint(window, title string, settings map[string]dbus.Variant, setup PageSetup) (preparedPrint, error) {
	if settings == nil {
		settings = map[string]dbus.Variant{}
	}
	resp, _, err := d.request(portalPreparePrint, map[string]dbus.Variant{"modal": dbus.MakeVariant(true)}, window, title, settings, encodePageSetup(setup))
	if err != nil {
		return preparedPrint{}, err
	}
	if resp.code != 0 {
		return preparedPrint{}, fmt.Errorf("app: print %q: %w", title, ErrPortalDenied)
	}
	token, ok := resp.results["token"].Value().(uint32)
	if !ok {
		return preparedPrint{}, fmt.Errorf("app: print %q: the portal sent no print token", title)
	}
	raw, _ := resp.results["page-setup"].Value().(map[string]dbus.Variant)
	chosen, err := decodePageSetup(raw)
	if err != nil {
		return preparedPrint{}, fmt.Errorf("app: print %q: %w", title, err)
	}
	newSettings, _ := resp.results["settings"].Value().(map[string]dbus.Variant)
	d.mu.Lock()
	d.printSettings, d.printSetup = newSettings, &chosen
	d.mu.Unlock()
	return preparedPrint{setup: chosen, token: token}, nil
}

func (d *desktopPortals) print(window, title string, f *os.File, token uint32) error {
	fd, err := intconv.Exact[int32](f.Fd())
	if err != nil {
		return fmt.Errorf("app: print %s: fd: %w", f.Name(), err)
	}
	resp, _, err := d.request(portalPrint, map[string]dbus.Variant{"modal": dbus.MakeVariant(true), "token": dbus.MakeVariant(token)}, window, title, dbus.UnixFD(fd))
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return fmt.Errorf("app: print %q: %w", title, ErrPortalDenied)
	}
	return nil
}

func encodePageSetup(s PageSetup) map[string]dbus.Variant {
	if s.Paper == (Paper{}) {
		return map[string]dbus.Variant{}
	}
	return map[string]dbus.Variant{
		"Name":         dbus.MakeVariant(s.Paper.Name),
		"Width":        dbus.MakeVariant(s.Paper.Width),
		"Height":       dbus.MakeVariant(s.Paper.Height),
		"MarginTop":    dbus.MakeVariant(s.Margins.Top),
		"MarginBottom": dbus.MakeVariant(s.Margins.Bottom),
		"MarginLeft":   dbus.MakeVariant(s.Margins.Left),
		"MarginRight":  dbus.MakeVariant(s.Margins.Right),
		"Orientation":  dbus.MakeVariant(s.Orientation.String()),
	}
}

func decodePageSetup(m map[string]dbus.Variant) (PageSetup, error) {
	length := func(key string) (float64, error) {
		v, ok := m[key].Value().(float64)
		if !ok {
			return 0, fmt.Errorf("the portal's page setup has no %s", key)
		}
		return v, nil
	}
	var s PageSetup
	var err error
	for _, f := range []struct {
		key string
		dst *float64
	}{
		{"Width", &s.Paper.Width},
		{"Height", &s.Paper.Height},
		{"MarginTop", &s.Margins.Top},
		{"MarginBottom", &s.Margins.Bottom},
		{"MarginLeft", &s.Margins.Left},
		{"MarginRight", &s.Margins.Right},
	} {
		if *f.dst, err = length(f.key); err != nil {
			return PageSetup{}, err
		}
	}
	s.Paper.Name, _ = m["Name"].Value().(string)
	o, ok := m["Orientation"].Value().(string)
	if !ok {
		return PageSetup{}, errors.New("the portal's page setup has no Orientation")
	}
	if s.Orientation, err = parseOrientation(o); err != nil {
		return PageSetup{}, err
	}
	return s, s.validate()
}
