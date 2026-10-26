// Command lys-ui-video plays a video: a file through ffmpeg (picture and
// sound) when one is named, otherwise a generated YUV4MPEG2 clip of
// moving bars decoded in pure Go. The controls show while the pointer
// is over the picture or playback is paused; Space toggles playback,
// Escape quits.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

const clipW, clipH, clipFrames = 320, 180, 75

func main() {
	loop := flag.Bool("loop", false, "restart at the end")
	flag.Parse()
	if err := play(flag.Arg(0), *loop); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func play(path string, loop bool) (err error) {
	src, err := openSource(path)
	if err != nil {
		return err
	}
	stream := media.NewStream(src, "lys-ui video")
	defer func() { err = errors.Join(err, stream.Close()) }()
	stream.SetLoop(loop)
	return run(stream)
}

func openSource(path string) (media.Source, error) {
	if path != "" {
		return media.OpenFile(path)
	}
	return media.NewY4M(bytes.NewReader(bars()))
}

func bars() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "YUV4MPEG2 W%d H%d F25:1 C444 XCOLORRANGE=FULL\n", clipW, clipH)
	luma := make([]byte, clipW*clipH)
	cb, cr := make([]byte, len(luma)), make([]byte, len(luma))
	for f := range clipFrames {
		for y := range clipH {
			for x := range clipW {
				band := (x + 4*f) * 7 / clipW % 7
				i := y*clipW + x
				luma[i] = byte(60 + 25*band)
				cb[i] = byte(128 + 40*(band%3-1))
				cr[i] = byte(128 + 40*(band%2*2-1))
			}
		}
		b.WriteString("FRAME\n")
		b.Write(luma)
		b.Write(cb)
		b.Write(cr)
	}
	return b.Bytes()
}

type player struct {
	env    ui.Env
	stream *media.Stream
}

func (p *player) view() ui.Node {
	return ui.Column(
		ui.Expand(ui.Video(p.stream).Autoplay(true)),
		ui.Label("Space plays and pauses, Escape quits").Font(nil, 12).Ink(widget.Current().TextMuted),
	).Spacing(8).Padding(render.UniformInsets(12))
}

func (p *player) Init(cx *component.Context[struct{}, struct{}]) widget.Widget {
	return ui.Mount(cx, p.env, p.view())
}

func (p *player) Update(*component.Context[struct{}, struct{}], struct{}) {}

func newPlayer(stream *media.Stream) (*player, error) {
	face, err := app.UIFont(13)
	if err != nil {
		return nil, err
	}
	return &player{env: ui.Env{Face: app.FontFallback(face), Size: 13}, stream: stream}, nil
}

func run(stream *media.Stream) error {
	w, h := stream.Size()
	return component.Run(app.WindowConfig{
		Title: "lys-ui video", AppID: "dev.stubbe.lys.ui.video",
		Width: pixels(max(w, clipW) + 24), Height: pixels(max(h, clipH) + 60), Background: widget.Current().Bg,
	}, func(a *app.Application) (*player, error) {
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		toggle := widget.NewAction("toggle", func() {
			if stream.Playing() {
				stream.Pause()
				return
			}
			stream.Play()
		})
		if err := a.AddAccel("space", toggle); err != nil {
			return nil, err
		}
		return newPlayer(stream)
	})
}

func pixels(v int) uint32 {
	return uint32(min(max(int64(v), 0), math.MaxUint32))
}
