package app

import (
	"errors"
	"image"
	"io"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/testbuild"
)

// firstVideoFrame decodes the first frame of path's video on dev.
func firstVideoFrame(t *testing.T, dev *gpu.Device, path string) *gpu.VideoFrame {
	t.Helper()
	f, err := demux.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	track := f.Tracks()[0]
	if !slices.Contains(dev.DecodeCodecs(), track.Codec) {
		t.Skipf("%s decodes no %s", dev.Name(), track.Codec)
	}
	dec, err := dev.NewVideoDecoder(track)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dec.Close)
	if err := f.Select(track.Index); err != nil {
		t.Fatal(err)
	}
	for eof := false; ; {
		if !eof {
			p, err := f.ReadPacket()
			switch {
			case errors.Is(err, io.EOF):
				dec.Flush()
				eof = true
			case err != nil:
				t.Fatal(err)
			default:
				if err := dec.Decode(p); err != nil {
					t.Fatal(err)
				}
			}
		}
		if frame := dec.Next(); frame != nil {
			return frame
		}
		if eof {
			t.Fatal("the video decoded no frame")
		}
	}
}

type videoDraw struct {
	dev   *gpu.Device
	tg    *gpu.Target
	whole []image.Rectangle
	frame *gpu.VideoFrame
}

func (d videoDraw) draw(t *testing.T) {
	t.Helper()
	if err := drawVideoFrame(d.dev, d.tg, d.whole, d.frame); err != nil {
		t.Fatal(err)
	}
}

func newVideoDraw(t *testing.T, w, h int) videoDraw {
	t.Helper()
	if !gputest.Enabled() {
		t.Skip("LYS_TEST_GPU is unset")
	}
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	dev, err := gpu.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Close)
	frame := firstVideoFrame(t, dev, "../media/testdata/h264-aac.mp4")
	mods, err := dev.Modifiers(fourcc.ARGB8888)
	if err != nil {
		t.Fatal(err)
	}
	tg, err := dev.NewTarget(w, h, fourcc.ARGB8888, mods)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tg.Close)
	return videoDraw{dev: dev, tg: tg, whole: []image.Rectangle{image.Rect(0, 0, w, h)}, frame: frame}
}

func TestAVideoFrameIsDrawnOnTheGPUIntoTheBuffer(t *testing.T) {
	const w, h = 96, 72
	d := newVideoDraw(t, w, h)
	d.draw(t)
	if err := d.dev.Finish(); err != nil {
		t.Fatal(err)
	}
	img, err := d.tg.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range []image.Point{{0, 0}, {w / 2, h / 2}, {w - 1, h - 1}} {
		if c := img.RGBAAt(pt.X, pt.Y); c.R < 200 || c.G > 60 || c.B > 60 || c.A != 255 {
			t.Errorf("(%d,%d) = %v, want the frame's opaque red scaled over the whole buffer", pt.X, pt.Y, c)
		}
	}
}

func TestAllocBudgetGPUVideoFrameDraw(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	d := newVideoDraw(t, 96, 72)
	d.draw(t)
	allocs := testing.AllocsPerRun(20, func() { d.draw(t) })
	t.Logf("%v heap allocations per frame drawn", allocs)
	if allocs != 0 {
		t.Errorf("%v heap allocations per frame drawn, want none", allocs)
	}
}
