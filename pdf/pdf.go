// Package pdf writes PDF documents in pure Go: pages of RGB raster
// images, Flate-compressed and streamed to an io.Writer as they are
// added. Output is deterministic (no timestamps or document IDs), so
// the same pages always produce the same bytes.
//
//	w := pdf.NewWriter(out, "Report")
//	a4 := pdf.Size{W: pdf.Millimetres(210), H: pdf.Millimetres(297)}
//	if err := w.AddPage(a4, page); err != nil { ... }
//	if err := w.Close(); err != nil { ... }
package pdf

import (
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Points is a length in PostScript points, 1/72 inch.
type Points float64

// Millimetres converts mm to points.
func Millimetres(mm float64) Points { return Points(mm / 25.4 * 72) }

// Size is a page's width and height.
type Size struct{ W, H Points }

// ErrNoPages reports a Close before any AddPage; a PDF has at least
// one page.
var ErrNoPages = errors.New("pdf: a document needs at least one page")

var errClosed = errors.New("pdf: the writer is closed")

const (
	catalogObj = 1
	pagesObj   = 2
	infoObj    = 3
)

// Writer streams a PDF document. Pages are written as they are added;
// Close writes the page tree and cross-reference table. A write error
// sticks: every later call returns it.
type Writer struct {
	out     countingWriter
	title   string
	offsets []int64
	pages   []int
	row     []byte
	zw      *zlib.Writer
	closed  bool
}

// NewWriter starts a document on out titled title (empty for none).
func NewWriter(out io.Writer, title string) *Writer {
	w := &Writer{out: countingWriter{w: out}, title: title, offsets: make([]int64, infoObj)}
	w.out.printf("%%PDF-1.4\n%%\xe2\xe3\xcf\xd3\n")
	return w
}

// AddPage appends a page of size, covered by img stretched to fill it.
// Transparent pixels show white, as on paper.
func (w *Writer) AddPage(size Size, img image.Image) error {
	if w.closed {
		return errClosed
	}
	if size.W <= 0 || size.H <= 0 {
		return fmt.Errorf("pdf: page size %vx%v pt is not positive", size.W, size.H)
	}
	b := img.Bounds()
	if b.Empty() {
		return fmt.Errorf("pdf: page image bounds %v are empty", b)
	}
	imageObj, lengthObj, contentObj, pageObj := w.alloc(), w.alloc(), w.alloc(), w.alloc()
	w.begin(imageObj)
	w.out.printf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d 0 R >>\nstream\n", b.Dx(), b.Dy(), lengthObj)
	start := w.out.n
	w.writePixels(img)
	length := w.out.n - start
	w.out.printf("\nendstream\nendobj\n")
	w.begin(lengthObj)
	w.out.printf("%d\nendobj\n", length)
	content := fmt.Sprintf("q %s 0 0 %s 0 0 cm /Im0 Do Q", num(size.W), num(size.H))
	w.begin(contentObj)
	w.out.printf("<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	w.begin(pageObj)
	w.out.printf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>\nendobj\n",
		pagesObj, num(size.W), num(size.H), imageObj, contentObj)
	w.pages = append(w.pages, pageObj)
	return w.out.err
}

// Close finishes the document. It does not close the underlying
// writer.
func (w *Writer) Close() error {
	if w.closed {
		return errClosed
	}
	w.closed = true
	if w.out.err != nil {
		return w.out.err
	}
	if len(w.pages) == 0 {
		return ErrNoPages
	}
	w.begin(catalogObj)
	w.out.printf("<< /Type /Catalog /Pages %d 0 R >>\nendobj\n", pagesObj)
	kids := make([]string, len(w.pages))
	for i, p := range w.pages {
		kids[i] = strconv.Itoa(p) + " 0 R"
	}
	w.begin(pagesObj)
	w.out.printf("<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), len(w.pages))
	w.begin(infoObj)
	w.out.printf("<< /Producer (lys-ui)")
	if w.title != "" {
		w.out.printf(" /Title %s", textString(w.title))
	}
	w.out.printf(" >>\nendobj\n")
	xref := w.out.n
	w.out.printf("xref\n0 %d\n0000000000 65535 f \n", len(w.offsets)+1)
	for _, off := range w.offsets {
		w.out.printf("%010d 00000 n \n", off)
	}
	w.out.printf("trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(w.offsets)+1, catalogObj, infoObj, xref)
	return w.out.err
}

func (w *Writer) alloc() int {
	w.offsets = append(w.offsets, 0)
	return len(w.offsets)
}

func (w *Writer) begin(obj int) {
	w.offsets[obj-1] = w.out.n
	w.out.printf("%d 0 obj\n", obj)
}

func (w *Writer) writePixels(img image.Image) {
	b := img.Bounds()
	if w.zw == nil {
		w.zw = zlib.NewWriter(&w.out)
	} else {
		w.zw.Reset(&w.out)
	}
	if n := 3 * b.Dx(); cap(w.row) < n {
		w.row = make([]byte, n)
	} else {
		w.row = w.row[:n]
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		overWhite(w.row, img, y)
		if _, err := w.zw.Write(w.row); err != nil {
			return
		}
	}
	_ = w.zw.Close()
}

func overWhite(dst []byte, img image.Image, y int) {
	b := img.Bounds()
	if rgba, ok := img.(*image.RGBA); ok {
		src := rgba.Pix[rgba.PixOffset(b.Min.X, y):]
		for x := range b.Dx() {
			s, d := src[4*x:4*x+4], dst[3*x:3*x+3]
			inv := 255 - s[3]
			d[0], d[1], d[2] = s[0]+inv, s[1]+inv, s[2]+inv
		}
		return
	}
	for x := range b.Dx() {
		r, g, bl, a := img.At(b.Min.X+x, y).RGBA()
		inv := 0xffff - a
		dst[3*x], dst[3*x+1], dst[3*x+2] = byte(min((r+inv)>>8, 0xff)), byte(min((g+inv)>>8, 0xff)), byte(min((bl+inv)>>8, 0xff))
	}
}

func num(p Points) string { return strconv.FormatFloat(float64(p), 'f', -1, 64) }

func textString(s string) string {
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteByte('>')
	return b.String()
}

type countingWriter struct {
	w   io.Writer
	n   int64
	err error
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	if err != nil {
		c.err = fmt.Errorf("pdf: write: %w", err)
	}
	return n, c.err
}

func (c *countingWriter) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(c, format, args...)
}
