package widget

import (
	"errors"
	"image"
	"io"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/media"
)

var errNoDecoderHere = errors.New("this device decodes no such video")

type fakeTextureSource struct{}

func (fakeTextureSource) Info() media.Info {
	return media.Info{Width: 32, Height: 16, Duration: time.Second}
}

func (fakeTextureSource) Close() error { return nil }

func (fakeTextureSource) Seek(time.Duration) error { return nil }

func (fakeTextureSource) ReadPacket() (demux.Packet, error) { return demux.Packet{}, io.EOF }

func (fakeTextureSource) NewVideoDecoder(*gpu.Device) (*gpu.VideoDecoder, error) {
	return nil, errNoDecoderHere
}

type fakeTextureSurface struct{ closed bool }

func (f *fakeTextureSurface) Place(render.Rect, image.Rectangle) {}

func (f *fakeTextureSurface) Close() { f.closed = true }

func (f *fakeTextureSurface) Device() *gpu.Device { return nil }

func (f *fakeTextureSurface) Present(*gpu.VideoFrame) (bool, error) { return true, nil }

func withTextureHost(t *testing.T, host TextureHost) {
	t.Helper()
	SetTextureHost(host)
	t.Cleanup(func() { SetTextureHost(nil) })
}

func textureVideo(t *testing.T) (*Video, *media.Stream) {
	t.Helper()
	s := media.NewStream(fakeTextureSource{}, "texture")
	t.Cleanup(s.Close)
	v := NewVideo(testFace(t), 13, s)
	SizeWithin(v, Size{W: 80, H: 80})
	return v, s
}

// A stream decoding on the GPU has nowhere else to show its frames:
// without a surface for them the stream fails with the reason.
func TestATextureStreamWithoutASurfaceFails(t *testing.T) {
	boom := errors.New("the compositor imports none of the device's buffers")
	for _, c := range []struct {
		name string
		host TextureHost
		want error
	}{
		{"no host", nil, errNoTextureHost},
		{"refused", func(*Video, int, int, func()) (TextureSurface, error) { return nil, boom }, boom},
	} {
		t.Run(c.name, func(t *testing.T) {
			withTextureHost(t, c.host)
			v, s := textureVideo(t)
			v.Arrange(render.Rect{W: 80, H: 80})
			if !errors.Is(v.OffloadErr(), c.want) || !errors.Is(s.Err(), c.want) {
				t.Errorf("offload error %v, stream error %v, want %v", v.OffloadErr(), s.Err(), c.want)
			}
		})
	}
}

// A host whose device is still opening has the Video wait for its
// wake, then decode on the device it gives: one that decodes none of
// the video fails the stream with the decoder's error.
func TestATextureVideoWaitsForTheDeviceThenDecodesOnIt(t *testing.T) {
	var (
		wake    func()
		pending = true
		surface = &fakeTextureSurface{}
		asked   int
	)
	withTextureHost(t, func(_ *Video, w, h int, wakeFn func()) (TextureSurface, error) {
		asked++
		if w != 32 || h != 16 {
			t.Errorf("surface for %dx%d frames, want 32x16", w, h)
		}
		wake = wakeFn
		if pending {
			return nil, ErrVideoDevicePending
		}
		return surface, nil
	})
	v, s := textureVideo(t)
	v.Arrange(render.Rect{W: 80, H: 80})
	if asked != 1 || v.OffloadErr() != nil || s.Err() != nil {
		t.Fatalf("while the device opens: asked %d, offload error %v, stream error %v", asked, v.OffloadErr(), s.Err())
	}
	pending = false
	wake()
	if asked != 2 || !surface.closed || !errors.Is(v.OffloadErr(), errNoDecoderHere) || !errors.Is(s.Err(), errNoDecoderHere) {
		t.Errorf("once the device opened: asked %d, surface closed %v, offload error %v, stream error %v", asked, surface.closed, v.OffloadErr(), s.Err())
	}
}
