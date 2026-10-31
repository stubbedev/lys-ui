package widget

import (
	"image"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// imagePlayback steps an animated source (a GIF or APNG decoded to a
// render.Animation) on the animation clock: each frame holds for its
// delay - a scheduled deadline, no goroutine or timer - and repaints
// only the image's rect. It plays only while the image is arranged on
// screen, stops after the animation's loop count on its last frame,
// and under reduced motion shows the first frame still.
type imagePlayback struct {
	frame  int
	plays  int
	cancel anim.Cancel
}

// animation is the source's animation when it has more than one frame.
func (im *Image) animation() *render.Animation {
	if a, ok := im.img.(*render.Animation); ok && len(a.Frames) > 1 {
		return a
	}
	return nil
}

// currentFrame is the image to paint and its frame index.
func (im *Image) currentFrame() (image.Image, int) {
	if a := im.animation(); a != nil {
		return a.Frames[im.play.frame], im.play.frame
	}
	return im.img, 0
}

// Frame reports the frame an animated source shows (0 for a still).
func (im *Image) Frame() int { return im.play.frame }

// Arrange records the rect and starts or stops playback with
// visibility.
func (im *Image) Arrange(r render.Rect) {
	im.node.Arrange(r)
	im.syncPlayback()
}

// restartPlayback rewinds for a new source.
func (im *Image) restartPlayback() {
	im.stopPlayback()
	im.play.frame, im.play.plays = 0, 0
	im.syncPlayback()
}

// stopPlayback cancels the pending frame deadline.
func (im *Image) stopPlayback() {
	if im.play.cancel != nil {
		im.play.cancel()
		im.play.cancel = nil
	}
}

// syncPlayback schedules the next frame when the image should be
// playing, and stops it otherwise.
func (im *Image) syncPlayback() {
	a := im.animation()
	done := a != nil && a.Loops > 0 && im.play.plays >= a.Loops
	if a == nil || !onScreen(im) || anim.Instant() || done {
		im.stopPlayback()
		return
	}
	if im.play.cancel != nil {
		return
	}
	gen := im.gen
	im.play.cancel = anim.Play(anim.Delay(a.Delays[im.play.frame]), anim.Animate(0, func(float64) {
		im.play.cancel = nil
		if gen != im.gen || !onScreen(im) {
			return
		}
		next := im.play.frame + 1
		if next == len(a.Frames) {
			im.play.plays++
			if a.Loops > 0 && im.play.plays >= a.Loops {
				return // rest on the last frame
			}
			next = 0
		}
		im.play.frame = next
		im.Invalidate()
		im.syncPlayback()
	}))
}
