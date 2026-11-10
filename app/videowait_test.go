package app_test

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget"
)

func TestAVideoWaitingForTheGPUDeviceIsWokenOnce(t *testing.T) {
	c := newVideoCompositor()
	_, a := connectFakeWith(t, videoGlobals, c.setup)
	release := a.HoldGPUOpen()
	stream := media.NewStream(app.UndecodedTextures{}, "test")
	t.Cleanup(stream.Close)
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	v := widget.NewVideo(face, 13, stream)
	if _, err := a.NewWindow(app.WindowConfig{Title: "video", Width: fakeSurfaceW, Height: fakeSurfaceH, Root: v}); err != nil {
		t.Fatal(err)
	}
	runUntilCleanup(t, a)
	awaitOnLoop(t, a, "the Video to ask for its surface", func() bool { return a.GPUWaiting() > 0 })

	const arranges = 5
	waiting := onLoop(a, func() int {
		for i := range arranges {
			v.Arrange(render.Rect{W: fakeSurfaceW - 1 - i, H: fakeSurfaceH})
		}
		return a.GPUWaiting()
	})
	if waiting != 1 {
		t.Errorf("after %d more arranges %d wakes wait for the device, want the Video's one", arranges, waiting)
	}

	left := onLoop(a, func() int {
		release()
		return a.GPUWaiting()
	})
	if err := onLoop(a, stream.Err); left != 0 || !errors.Is(err, app.ErrNoGPURenderer) {
		t.Errorf("once no device came: %d wakes waiting, stream error %v; want the Video woken to learn there is no GPU renderer", left, err)
	}
}
