package clipboard

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/transfer"
)

// testImage builds a small deterministic image.
func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 0x80, A: 0xff})
		}
	}
	return img
}

// TestPickImageMime pins the image read priority: png over jpeg, none
// when the offer carries no image type.
func TestPickImageMime(t *testing.T) {
	if got := pickImageMime(func(m string) bool { return m == "image/jpeg" }); got != "image/jpeg" {
		t.Errorf("jpeg-only offer = %q", got)
	}
	if got := pickImageMime(func(m string) bool { return m == "image/jpeg" || m == "image/png" }); got != "image/png" {
		t.Errorf("both offered = %q, want png first", got)
	}
	if got := pickImageMime(func(m string) bool { return m == "text/plain" }); got != "" {
		t.Errorf("text offer = %q, want none", got)
	}
}

// TestImageSendByMime pins the claim's send half (#69, #112): a
// consumer asking image/png receives exactly the encoded PNG bytes,
// image/jpeg a decodable JPEG, a mime the claim lacks nothing; an
// empty image never claims.
func TestImageSendByMime(t *testing.T) {
	img := testImage(24, 12)
	content, err := transfer.Image(img)
	if err != nil {
		t.Fatal(err)
	}
	c := &Clipboard{content: content}
	send := func(mime string) []byte {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: mime, Fd: w.Fd()})
		w.Close()
		data, _ := io.ReadAll(r)
		return data
	}
	var want bytes.Buffer
	if err := png.Encode(&want, img); err != nil {
		t.Fatal(err)
	}
	if got := send(transfer.MimePNG); !bytes.Equal(got, want.Bytes()) {
		t.Errorf("image/png payload = %d bytes, want the %d encoded ones", len(got), want.Len())
	}
	if j, err := jpeg.Decode(bytes.NewReader(send(transfer.MimeJPEG))); err != nil || j.Bounds() != img.Bounds() {
		t.Errorf("image/jpeg payload: %v", err)
	}
	if got := send(transfer.MimeText); len(got) != 0 {
		t.Errorf("text from an image claim: %d bytes", len(got))
	}

	// dropClaim retires the claim; on an empty clipboard it is a no-op.
	c.dropClaim()
	if !c.content.Empty() {
		t.Error("dropClaim left the payload behind")
	}
	c.dropClaim()

	// An empty image never claims - the guard fires before any session
	// use, so the empty session cannot panic either.
	if err := c.WriteImage(image.NewRGBA(image.Rect(0, 0, 0, 0))); err == nil {
		t.Error("an empty image claimed the selection")
	}
	// A healthy image without a data device reports unavailable, not a
	// panic.
	if err := c.WriteImage(testImage(2, 2)); !errors.Is(err, ErrUnavailable) {
		t.Errorf("WriteImage without a device = %v, want ErrUnavailable", err)
	}
}

// TestReadOfferImage pins the read half: the best image mime wins, an
// honest peer's bytes round-trip through the bounded transfer, and a
// text-only offer reports ErrUnavailable.
func TestReadOfferImage(t *testing.T) {
	t.Run("round-trips a small png payload", func(t *testing.T) {
		receive := func(mime string, fd uintptr) error {
			if mime != "image/png" {
				t.Errorf("requested %q, want image/png", mime)
			}
			wait := fakePeer(t, fd, []byte("fake-png-bytes"))
			return wait()
		}
		data, mime, err := readOfferImage(receive, func() error { return nil },
			map[string]bool{"image/png": true})
		if err != nil || mime != "image/png" {
			t.Fatalf("read = %q, %v", mime, err)
		}
		if string(data) != "fake-png-bytes" {
			t.Errorf("payload = %q", data)
		}
	})

	t.Run("a text-only offer is unavailable", func(t *testing.T) {
		if _, _, err := readOfferImage(func(string, uintptr) error { return nil },
			func() error { return nil }, map[string]bool{"text/plain": true}); !errors.Is(err, ErrUnavailable) {
			t.Errorf("text offer read = %v, want ErrUnavailable", err)
		}
	})
}
