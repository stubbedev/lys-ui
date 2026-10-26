package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/vinput"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// shotsAppID is the app id of the windows -shots opens; the compositor
// floats them at its origin (just docshots).
const shotsAppID = "dev.stubbe.lys.ui.shots"

// gesture is how a shot opens a demo's overlay: the pointer input a
// user would give its trigger.
type gesture uint8

const (
	noGesture gesture = iota
	click
	secondaryClick
	hover
)

// evdev pointer buttons.
const (
	btnLeft  = 0x110
	btnRight = 0x111
)

// shot timing: how long a hover dwells before the capture settles (the
// tooltip and hover-card delays are shorter), how often the settle
// polls, how many polls in a row must match - long enough for a
// dialog to map after its trigger repainted - and how long it waits.
const (
	hoverDwell   = 1200 * time.Millisecond
	settlePoll   = 80 * time.Millisecond
	settleRuns   = 8
	settleBudget = 10 * time.Second
)

// shotPad is the margin around a demo in its shot window, and
// shotMaxWidth the widest a shot window measures its demo at. A shot
// frames its content shotMargin from each edge of a stage at least
// shotStageW by shotStageH, all logical pixels.
const (
	shotPad      = 24
	shotMaxWidth = 720
	shotMargin   = 32
	shotStageW   = 560
	shotStageH   = 200
)

// palettes are the shots' themes, their file suffixes, and where the
// index records each shot's size.
var palettes = []struct {
	suffix string
	theme  func() *widget.Theme
	size   func(*shotEntry) *shotSize
}{
	{"", widget.DarkTheme, func(e *shotEntry) *shotSize { return &e.Dark }},
	{"-light", widget.LightTheme, func(e *shotEntry) *shotSize { return &e.Light }},
}

// slug is the file name stem of the demo titled title.
func slug(title string) string { return strings.ReplaceAll(strings.ToLower(title), " ", "-") }

// runShots writes a PNG of every demo, overlay opened, in each palette
// to dir. It runs on whatever compositor the session connects to; just
// docshots gives it a private headless sway that floats its windows at
// the origin, so window coordinates are compositor coordinates.
func runShots(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("shots: create %s: %w", dir, err)
	}
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	cp, err := capture.Connect()
	if err != nil {
		return fmt.Errorf("shots: capture: %w", err)
	}
	defer func() { _ = cp.Close() }()
	outputs := cp.Outputs()
	if len(outputs) == 0 || !cp.HasScreencopy() {
		return errors.New("shots: the compositor offers no output to capture")
	}
	pointer, err := vinput.Dial("", vinput.Options{})
	if err != nil {
		return fmt.Errorf("shots: virtual pointer: %w", err)
	}
	defer pointer.Close()

	defer anim.SetInstant(true)()
	application := app.NewApplication(sess)
	widget.LoadStylesheet(galleryCSS)
	s := &shooter{
		app:     application,
		env:     ui.Env{Face: app.FontFallback(face), Size: 14},
		cp:      cp,
		out:     outputs[0],
		pointer: pointer,
		dir:     dir,
	}
	release := application.Hold()
	done := make(chan error, 1)
	go func() {
		err := s.all()
		application.Invoke(func() {
			release()
			application.Quit()
		})
		done <- err
	}()
	if err := application.Run(); err != nil && !errors.Is(err, app.ErrClosed) {
		return err
	}
	return <-done
}

// shooter drives the shots from its own goroutine: widget work hops to
// the loop through on, capture and input go over their own
// connections.
type shooter struct {
	app     *app.Application
	env     ui.Env
	cp      *capture.Client
	out     capture.Output
	pointer *vinput.Device
	dir     string
}

// on runs fn on the loop and waits for it.
func (s *shooter) on(fn func()) {
	done := make(chan struct{})
	s.app.Invoke(func() {
		defer close(done)
		fn()
	})
	<-done
}

// shotEntry is one demo in the shots' index: its title, the stem of
// its files, and each palette's shot size.
type shotEntry struct {
	Title string   `json:"title"`
	Slug  string   `json:"slug"`
	Dark  shotSize `json:"dark"`
	Light shotSize `json:"light"`
}

// shotSize is a shot's size in logical pixels: the PNG's pixels over
// the output scale, the size the docs site shows it at.
type shotSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// all shoots every demo in every palette, starting with the pointer
// parked (the compositor puts it mid-output, over the first window),
// then writes index.json, the demos in gallery order, which the docs
// site lists them by.
func (s *shooter) all() error {
	if err := s.park(); err != nil {
		return err
	}
	var index []shotEntry
	s.on(func() {
		for _, d := range (&gallery{env: s.env}).demos(func(galleryMsg) {}) {
			index = append(index, shotEntry{Title: d.title, Slug: slug(d.title)})
		}
	})
	for _, p := range palettes {
		for i := range index {
			size, err := s.one(i, p.theme, p.suffix)
			if err != nil {
				return err
			}
			*p.size(&index[i]) = size
		}
	}
	return writeAtomic(filepath.Join(s.dir, "index.json"), func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(index)
	})
}

// one shoots demo i under theme into its file and reports the shot's
// logical size.
func (s *shooter) one(i int, theme func() *widget.Theme, suffix string) (size shotSize, err error) {
	var (
		d      demo
		g      *gallery
		win    *app.Window
		view   *ui.View
		window widget.Size
	)
	s.on(func() {
		widget.SetTheme(theme())
		g = &gallery{env: s.env, app: s.app}
		d = g.demos(func(galleryMsg) {})[i]
		var root widget.Widget
		root, view = ui.Build(s.env, d.host(d.bodyNode().Padding(render.UniformInsets(shotPad)).Classes(css.Background)))
		window = d.windowSize(root)
		win, err = s.app.NewWindow(app.WindowConfig{
			Title: d.title, AppID: shotsAppID,
			Width: pixels(window.W), Height: pixels(window.H),
			Root: root, Background: widget.Current().Bg, Opaque: true,
		})
		g.win = win
	})
	if err != nil {
		return shotSize{}, fmt.Errorf("shots: %s window: %w", d.title, err)
	}
	defer func() {
		s.on(func() {
			if g.shown != nil {
				g.shown.Close()
			}
			win.Close()
			view.Close()
		})
		if cerr := s.cleared(); cerr != nil && err == nil {
			err = fmt.Errorf("shots: %s: %w", d.title, cerr)
		}
	}()
	img, err := s.settle(nil)
	if err != nil {
		return shotSize{}, fmt.Errorf("shots: %s: %w", d.title, err)
	}
	if d.gesture != noGesture {
		if err := s.open(d); err != nil {
			return shotSize{}, fmt.Errorf("shots: %s: %w", d.title, err)
		}
		img, err = s.settle(img)
	}
	if err != nil {
		return shotSize{}, fmt.Errorf("shots: %s: %w", d.title, err)
	}
	if err := s.park(); err != nil {
		return shotSize{}, err
	}
	var stage color.RGBA
	s.on(func() {
		bg := widget.Current().Bg.Straight()
		stage = color.RGBA{R: bg[0], G: bg[1], B: bg[2], A: 255}
	})
	sc := int(s.out.Scale)
	content := contentBounds(img, stage)
	if inside := image.Rect(0, 0, window.W*sc, window.H*sc); !content.In(inside) {
		return shotSize{}, fmt.Errorf("shots: %s: the content spans %v, past its %v window: give the demo more room", d.title, content, inside)
	}
	shot := frame(img, content, stage, sc)
	size = shotSize{Width: shot.Bounds().Dx() / sc, Height: shot.Bounds().Dy() / sc}
	return size, writeAtomic(filepath.Join(s.dir, slug(d.title)+suffix+".png"), func(w io.Writer) error { return png.Encode(w, shot) })
}

// open gives the demo's trigger the gesture that opens its overlay.
func (s *shooter) open(d demo) error {
	var at image.Point
	s.on(func() { at = d.target() })
	if err := s.move(at); err != nil {
		return err
	}
	switch d.gesture {
	case click:
		if err := s.press(btnLeft); err != nil {
			return err
		}
		return s.park()
	case secondaryClick:
		return s.press(btnRight)
	case hover:
		time.Sleep(hoverDwell)
	}
	return nil
}

// move puts the pointer at p in logical compositor coordinates; the
// motion's extents are the output's mode, in hardware pixels.
func (s *shooter) move(p image.Point) error {
	sc := int(s.out.Scale)
	if err := s.pointer.MotionAbsolute(pixels(p.X*sc), pixels(p.Y*sc), pixels(int(s.out.Width)), pixels(int(s.out.Height))); err != nil {
		return fmt.Errorf("shots: pointer motion: %w", err)
	}
	return s.pointer.Roundtrip()
}

// press clicks button where the pointer is.
func (s *shooter) press(button uint32) error {
	for _, down := range []bool{true, false} {
		if err := s.pointer.Button(button, down); err != nil {
			return fmt.Errorf("shots: pointer button: %w", err)
		}
	}
	return s.pointer.Roundtrip()
}

// park moves the pointer to the output's far corner, off every shot.
func (s *shooter) park() error {
	sc := int(s.out.Scale)
	return s.move(image.Pt(int(s.out.Width)/sc-1, int(s.out.Height)/sc-1))
}

// settle captures the output until settleRuns frames in a row match
// and differ from before (nil: from the bare output): the windows
// mapped and painted, or a gesture's overlay opened.
func (s *shooter) settle(before *image.RGBA) (*image.RGBA, error) {
	var last *image.RGBA
	runs := 0
	deadline := time.Now().Add(settleBudget)
	for time.Now().Before(deadline) {
		time.Sleep(settlePoll)
		f, err := s.cp.CaptureOutput(s.out, capture.Options{})
		if err != nil {
			return nil, fmt.Errorf("capture: %w", err)
		}
		img, err := f.Image()
		if err != nil {
			return nil, fmt.Errorf("capture: %w", err)
		}
		changed := !blank(img)
		if before != nil {
			changed = !bytes.Equal(before.Pix, img.Pix)
		}
		if last != nil && bytes.Equal(last.Pix, img.Pix) && changed {
			runs++
		} else {
			runs = 0
		}
		if runs == settleRuns {
			return img, nil
		}
		last = img
	}
	return nil, fmt.Errorf("the output did not settle within %v", settleBudget)
}

// cleared waits for the output to show nothing again once a shot's
// windows closed, so the next shot starts from the bare output.
func (s *shooter) cleared() error {
	deadline := time.Now().Add(settleBudget)
	for time.Now().Before(deadline) {
		time.Sleep(settlePoll)
		f, err := s.cp.CaptureOutput(s.out, capture.Options{})
		if err != nil {
			return fmt.Errorf("capture: %w", err)
		}
		img, err := f.Image()
		if err != nil {
			return fmt.Errorf("capture: %w", err)
		}
		if blank(img) {
			return nil
		}
	}
	return fmt.Errorf("its windows were still on the output %v after closing", settleBudget)
}

// blank reports whether img is the bare output: nothing mapped yet.
func blank(img *image.RGBA) bool {
	bare := bareColor(img)
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) != bare {
				return false
			}
		}
	}
	return true
}

// bareColor is the output's own background, where nothing is mapped:
// the capture's far corner, which no shot window reaches.
func bareColor(img *image.RGBA) color.RGBA {
	b := img.Bounds()
	return img.RGBAAt(b.Max.X-1, b.Max.Y-1)
}

// contentBounds is what of a capture is neither the output's
// background nor the stage, the window's own fill.
func contentBounds(img *image.RGBA, stage color.RGBA) image.Rectangle {
	b := img.Bounds()
	bare := bareColor(img)
	content := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := img.RGBAAt(x, y); c != bare && c != stage {
				content = content.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return content
}

// frame stages a capture's content centered on a stage of the theme's
// background, shotMargin from its edges and at least shotStageW by
// shotStageH, the output's bare pixels between a window and its popups
// filled with the stage too. scale is the output's, logical to device
// pixels.
func frame(img *image.RGBA, content image.Rectangle, stage color.RGBA, scale int) *image.RGBA {
	bare := bareColor(img)
	m := shotMargin * scale
	w := max(content.Dx()+2*m, shotStageW*scale)
	h := max(content.Dy()+2*m, shotStageH*scale)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: stage}, image.Point{}, draw.Src)
	at := image.Pt((w-content.Dx())/2, (h-content.Dy())/2)
	for y := content.Min.Y; y < content.Max.Y; y++ {
		for x := content.Min.X; x < content.Max.X; x++ {
			if c := img.RGBAAt(x, y); c != bare {
				out.SetRGBA(at.X+x-content.Min.X, at.Y+y-content.Min.Y, c)
			}
		}
	}
	return out
}

// writeAtomic writes path through write, atomically: a temporary file
// renamed into place.
func writeAtomic(path string, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".shot-*")
	if err != nil {
		return fmt.Errorf("shots: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("shots: encode %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("shots: write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("shots: %w", err)
	}
	return nil
}

func pixels(v int) uint32 {
	return uint32(min(max(int64(v), 0), math.MaxUint32))
}
