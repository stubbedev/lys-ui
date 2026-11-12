package widget

import (
	"errors"
	"fmt"
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

type fakeVideoDecoder struct {
	dev    *gpu.Device
	lost   error
	closed bool
}

func (d *fakeVideoDecoder) Busy() (bool, error) { return false, nil }

func (d *fakeVideoDecoder) Decode(demux.Packet) error { return d.lost }

func (d *fakeVideoDecoder) Next() *gpu.VideoFrame { return nil }

func (d *fakeVideoDecoder) Flush() {}

func (d *fakeVideoDecoder) Reset() error { return nil }

func (d *fakeVideoDecoder) Close() { d.closed = true }

type fakeTextureSource struct {
	decodes  bool
	decoders []*fakeVideoDecoder
}

func (*fakeTextureSource) Info() media.Info {
	return media.Info{Width: 32, Height: 16, Duration: time.Second}
}

func (*fakeTextureSource) Close() error { return nil }

func (*fakeTextureSource) Seek(time.Duration) error { return nil }

func (f *fakeTextureSource) ReadPacket() (demux.Packet, error) {
	if !f.decodes {
		return demux.Packet{}, io.EOF
	}
	return demux.Packet{Key: true}, nil
}

func (f *fakeTextureSource) NewVideoDecoder(dev *gpu.Device) (media.VideoDecoder, error) {
	if !f.decodes {
		return nil, errNoDecoderHere
	}
	d := &fakeVideoDecoder{dev: dev}
	f.decoders = append(f.decoders, d)
	return d, nil
}

type fakeTextureSurface struct {
	dev    *gpu.Device
	closed bool
	lost   []error
}

func (f *fakeTextureSurface) Place(render.Rect, image.Rectangle) {}

func (f *fakeTextureSurface) Close() { f.closed = true }

func (f *fakeTextureSurface) Device() *gpu.Device { return f.dev }

func (f *fakeTextureSurface) DeviceLost(err error) { f.lost = append(f.lost, err) }

func (f *fakeTextureSurface) Present(*gpu.VideoFrame) (bool, error) { return true, nil }

func withTextureHost(t *testing.T, host TextureHost) {
	t.Helper()
	SetTextureHost(host)
	t.Cleanup(func() { SetTextureHost(nil) })
}

func textureVideo(t *testing.T, src *fakeTextureSource) (*Video, *media.Stream) {
	t.Helper()
	s := media.NewStream(pumpClock(t), src, "texture")
	t.Cleanup(s.Close)
	v := NewVideo(testFace(t), 13, s)
	SizeWithin(v, Size{W: 80, H: 80})
	return v, s
}

type fakeTextureHost struct {
	device     *gpu.Device
	pending    bool
	refusal    error
	asked      int
	wake, lost func()
	surfaces   []*fakeTextureSurface
}

func (h *fakeTextureHost) surface(_ *Video, _, _ int, wake, lost func()) (TextureSurface, error) {
	h.asked++
	h.wake, h.lost = wake, lost
	switch {
	case h.refusal != nil:
		return nil, h.refusal
	case h.pending:
		return nil, ErrVideoDevicePending
	}
	s := &fakeTextureSurface{dev: h.device}
	h.surfaces = append(h.surfaces, s)
	return s, nil
}

func TestATextureStreamWithoutASurfaceFails(t *testing.T) {
	boom := errors.New("the compositor imports none of the device's buffers")
	for _, c := range []struct {
		name string
		host TextureHost
		want error
	}{
		{"no host", nil, errNoTextureHost},
		{"refused", (&fakeTextureHost{refusal: boom}).surface, boom},
	} {
		t.Run(c.name, func(t *testing.T) {
			withTextureHost(t, c.host)
			v, s := textureVideo(t, &fakeTextureSource{})
			v.Arrange(render.Rect{W: 80, H: 80})
			if !errors.Is(v.OffloadErr(), c.want) || !errors.Is(s.Err(), c.want) {
				t.Errorf("offload error %v, stream error %v, want %v", v.OffloadErr(), s.Err(), c.want)
			}
		})
	}
}

func TestATextureVideoWaitsForTheDeviceThenDecodesOnIt(t *testing.T) {
	host := &fakeTextureHost{pending: true}
	withTextureHost(t, func(v *Video, w, h int, wake, lost func()) (TextureSurface, error) {
		if w != 32 || h != 16 {
			t.Errorf("surface for %dx%d frames, want 32x16", w, h)
		}
		return host.surface(v, w, h, wake, lost)
	})
	v, s := textureVideo(t, &fakeTextureSource{})
	v.Arrange(render.Rect{W: 80, H: 80})
	if host.asked != 1 || v.OffloadErr() != nil || s.Err() != nil {
		t.Fatalf("while the device opens: asked %d, offload error %v, stream error %v", host.asked, v.OffloadErr(), s.Err())
	}
	host.pending = false
	host.wake()
	if host.asked != 2 || !host.surfaces[0].closed || !errors.Is(v.OffloadErr(), errNoDecoderHere) || !errors.Is(s.Err(), errNoDecoderHere) {
		t.Errorf("once the device opened: asked %d, surface closed %v, offload error %v, stream error %v", host.asked, host.surfaces[0].closed, v.OffloadErr(), s.Err())
	}
}

func TestATextureVideoFollowsItsHostToANewDevice(t *testing.T) {
	first, second := &gpu.Device{}, &gpu.Device{}
	host := &fakeTextureHost{device: first}
	withTextureHost(t, host.surface)
	src := &fakeTextureSource{decodes: true}
	v, s := textureVideo(t, src)
	v.Arrange(render.Rect{W: 80, H: 80})
	if len(src.decoders) != 1 || src.decoders[0].dev != first {
		t.Fatalf("%d decoders opened, want one on the host's device", len(src.decoders))
	}
	s.Play()

	host.pending, host.device = true, second
	host.lost()
	if !host.surfaces[0].closed || !src.decoders[0].closed {
		t.Fatalf("as the device goes: surface closed %v, decoder closed %v", host.surfaces[0].closed, src.decoders[0].closed)
	}
	v.Arrange(render.Rect{W: 80, H: 80})
	if len(host.surfaces) != 1 || v.OffloadErr() != nil || s.Err() != nil || !s.Playing() {
		t.Fatalf("while the next device opens: %d surfaces, offload error %v, stream error %v, playing %v", len(host.surfaces), v.OffloadErr(), s.Err(), s.Playing())
	}

	host.pending = false
	host.wake()
	if len(host.surfaces) != 2 || host.surfaces[1].closed || len(src.decoders) != 2 || src.decoders[1].dev != second || src.decoders[1].closed {
		t.Fatalf("once the next device opened: %d surfaces, %d decoders", len(host.surfaces), len(src.decoders))
	}
	if v.OffloadErr() != nil || s.Err() != nil || !s.Playing() {
		t.Errorf("on the next device: offload error %v, stream error %v, playing %v", v.OffloadErr(), s.Err(), s.Playing())
	}
}

func TestATextureVideoFailsWhenNoDeviceFollowsALostOne(t *testing.T) {
	host := &fakeTextureHost{device: &gpu.Device{}}
	withTextureHost(t, host.surface)
	v, s := textureVideo(t, &fakeTextureSource{decodes: true})
	v.Arrange(render.Rect{W: 80, H: 80})
	s.Play()

	gone := errors.New("the windows draw on the CPU")
	host.refusal = gone
	host.lost()
	host.wake()
	if !errors.Is(v.OffloadErr(), gone) || !errors.Is(s.Err(), gone) || s.Playing() {
		t.Errorf("offload error %v, stream error %v, playing %v; want the stream failed by the host's refusal", v.OffloadErr(), s.Err(), s.Playing())
	}
}

func TestATextureVideoTellsItsHostOfADeviceItsDecodingFoundLost(t *testing.T) {
	host := &fakeTextureHost{device: &gpu.Device{}}
	withTextureHost(t, host.surface)
	src := &fakeTextureSource{decodes: true}
	v, s := textureVideo(t, src)
	v.Arrange(render.Rect{W: 80, H: 80})
	surface := host.surfaces[0]

	src.decoders[0].lost = fmt.Errorf("fake decoder: %w", gpu.ErrDeviceLost)
	s.Play()
	pumpUntil(t, func() bool { return len(surface.lost) > 0 })
	if len(surface.lost) != 1 || !errors.Is(surface.lost[0], gpu.ErrDeviceLost) {
		t.Errorf("the host heard %v, want the loss once", surface.lost)
	}
	if s.Err() != nil || !s.Playing() || !src.decoders[0].closed {
		t.Errorf("stream error %v, playing %v, decoder closed %v; want the stream playing on, off the lost device", s.Err(), s.Playing(), src.decoders[0].closed)
	}
}
