package app_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget"
)

const videoLossBound = 60 * time.Second

func onLoop[T any](a *app.Application, fn func() T) T {
	got := make(chan T, 1)
	a.Invoke(func() { got <- fn() })
	return <-got
}

func awaitOnLoop(t *testing.T, a *app.Application, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(videoLossBound); !onLoop(a, done); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for %s", videoLossBound, what)
		}
	}
}

type videoState struct {
	offload, stream error
	playing         bool
}

func h264Modifiers(t *testing.T) []fourcc.Modifier {
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
	defer dev.Close()
	if !slices.Contains(dev.DecodeCodecs(), demux.H264) {
		t.Skipf("%s decodes no H.264", dev.Name())
	}
	mods, err := dev.Modifiers(fourcc.ARGB8888)
	if err != nil {
		t.Fatal(err)
	}
	return mods
}

func TestAGPUVideoPlaysOnThroughALostDevice(t *testing.T) {
	c := newVideoCompositor()
	c.argb = h264Modifiers(t)
	_, a := connectFakeWith(t, videoGlobals, c.setup)
	t.Setenv("LYS_UI_RENDERER", "auto")
	src, err := media.OpenFile("../media/testdata/h264-aac.mp4")
	if err != nil {
		t.Fatal(err)
	}
	stream := media.NewStream(a, media.WithoutAudio(src), "test")
	t.Cleanup(stream.Close)
	stream.SetLoop(true)
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	v := widget.NewVideo(face, 13, stream)
	v.SetAutoplay(true)
	if _, err := a.NewWindow(app.WindowConfig{Title: "video", Width: fakeSurfaceW, Height: fakeSurfaceH, Root: v}); err != nil {
		t.Fatal(err)
	}
	runUntilCleanup(t, a)

	healthy := func(when string) {
		t.Helper()
		got := onLoop(a, func() videoState { return videoState{v.OffloadErr(), stream.Err(), stream.Playing()} })
		if got != (videoState{playing: true}) {
			t.Fatalf("%s: offload error %v, stream error %v, playing %v", when, got.offload, got.stream, got.playing)
		}
	}
	c.awaitVideoCommits(t, 10)
	healthy("on the first device")
	first := onLoop(a, a.GPUDevice)
	if first == nil {
		t.Fatal("frames decoded on the GPU are shown with no GPU device")
	}

	a.Invoke(func() { a.FaultGPU(fmt.Errorf("app test: injected: %w", gpu.ErrDeviceLost)) })
	awaitOnLoop(t, a, "a new device", func() bool { return a.GPUDevice() != nil && a.GPUDevice() != first })
	c.awaitVideoCommits(t, c.counts().videoCommits+30)
	healthy("on the device opened after the loss")
}
