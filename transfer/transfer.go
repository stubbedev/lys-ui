// Package transfer is the content shape every data transfer shares:
// clipboard claims, the primary selection, and drag and drop all offer
// a payload as mime types, best first, with a writer for each, and all
// read one back by mime preference. The builders cover the formats the
// desktop speaks - text, images (PNG and JPEG), uri lists (file
// copies), HTML (formatted text) - and encode up front, so a peer's
// transfer deadline never runs during encoding.
package transfer

import (
	"bytes"
	"io"
	"slices"
)

// Well-known mime types.
const (
	MimeText    = "text/plain;charset=utf-8"
	MimeURIList = "text/uri-list"
	MimeHTML    = "text/html"
	MimePNG     = "image/png"
	MimeJPEG    = "image/jpeg"
	MimeWebP    = "image/webp"
	MimeGIF     = "image/gif"
)

// TextMimes are the text types offered and accepted, best first: the
// X11 atoms after the mime types, for clients bridged through
// Xwayland.
var TextMimes = []string{
	MimeText,
	"UTF8_STRING",
	"text/plain",
	"COMPOUND_TEXT",
	"TEXT",
	"STRING",
}

// ImageMimes are the image types accepted, best first: PNG
// round-trips exactly, the rest decode lossy or flattened.
var ImageMimes = []string{MimePNG, MimeWebP, MimeJPEG, MimeGIF}

// Content is one transfer's payload: the mimes it is offered as, best
// first, and Write, which writes the bytes for one of them. Write runs
// off the event loop, possibly several at once, so it must not touch
// widgets.
type Content struct {
	Mimes []string
	Write func(mime string, w io.Writer) error
}

// Empty reports whether c offers nothing.
func (c Content) Empty() bool { return len(c.Mimes) == 0 || c.Write == nil }

// Has reports whether c offers mime.
func (c Content) Has(mime string) bool { return slices.Contains(c.Mimes, mime) }

// Bytes writes c's payload for mime into a buffer - the same-process
// path, where no pipe is needed.
func (c Content) Bytes(mime string) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.Write(mime, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Pick returns the first of prefs that present reports, "" when none.
func Pick(prefs []string, present func(string) bool) string {
	for _, m := range prefs {
		if present(m) {
			return m
		}
	}
	return ""
}

// Payloads is content from finished bytes per mime, offered in the
// order given; the writers copy, so the content may be served any
// number of times.
func Payloads(mimes []string, data map[string][]byte) Content {
	return Content{
		Mimes: mimes,
		Write: func(mime string, w io.Writer) error {
			_, err := w.Write(data[mime])
			return err
		},
	}
}

// Merge offers every part's mimes in order; a mime two parts share is
// served by the first. Formatted text merges HTML before plain text,
// files their uri list before the paths as text.
func Merge(parts ...Content) Content {
	var mimes []string
	owner := map[string]Content{}
	for _, p := range parts {
		for _, m := range p.Mimes {
			if _, dup := owner[m]; dup {
				continue
			}
			owner[m] = p
			mimes = append(mimes, m)
		}
	}
	return Content{
		Mimes: mimes,
		Write: func(mime string, w io.Writer) error {
			p, ok := owner[mime]
			if !ok {
				return nil
			}
			return p.Write(mime, w)
		},
	}
}

// Text is s offered under every text mime.
func Text(s string) Content {
	b := []byte(s)
	data := make(map[string][]byte, len(TextMimes))
	for _, m := range TextMimes {
		data[m] = b
	}
	return Payloads(TextMimes, data)
}

// Bytes is data offered as one mime.
func Bytes(mime string, data []byte) Content {
	return Payloads([]string{mime}, map[string][]byte{mime: data})
}
