package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/third_party/gomath"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

const (
	// kineticWindow is how far back the velocity looks: the motion of
	// the fingers' last moments, not the whole gesture.
	kineticWindow = 100 * time.Millisecond
	// kineticTau is the glide's decay time constant: the speed falls
	// to 1/e every tau, and the glide travels v*tau in all.
	kineticTau = 325 * time.Millisecond
	// kineticMinSpeed is the slowest flick that glides, and the speed
	// a glide settles at, in pixels per second.
	kineticMinSpeed = 60.0
	// kineticSamples bounds the velocity ring.
	kineticSamples = 8
)

// kineticSample is one pixel delta and when it came.
type kineticSample struct {
	at time.Time
	d  [2]float64
}

// kinetic is the touchpad glide the pixel scrollers share (Scroll,
// List): pixel deltas are sampled as they come, and when the fingers
// lift (ScrollEnd) the content glides on at the gesture's velocity,
// decaying exponentially. Any new input stops a glide. Under reduced
// motion the glide lands at once.
type kinetic struct {
	ring   [kineticSamples]kineticSample
	n      int
	cancel anim.Cancel
}

// sample records a delta of the gesture in flight, stopping any glide.
func (k *kinetic) sample(dx, dy float64) {
	k.halt()
	k.ring[k.n%kineticSamples] = kineticSample{animclock.Now(), [2]float64{dx, dy}}
	k.n++
}

// halt stops a running glide.
func (k *kinetic) halt() {
	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
	}
}

// stop halts the glide and forgets the gesture: input that is not a
// finger scroll (a wheel step, a drag, a key) ends it.
func (k *kinetic) stop() {
	k.halt()
	k.n = 0
}

// velocity is the gesture's recent speed per axis in pixels per second:
// the deltas inside the window over the time they span.
func (k *kinetic) velocity() [2]float64 {
	now := animclock.Now()
	var sum [2]float64
	var first time.Time
	count := 0
	for i := max(0, k.n-kineticSamples); i < k.n; i++ {
		s := k.ring[i%kineticSamples]
		if now.Sub(s.at) > kineticWindow {
			continue
		}
		if count == 0 {
			first = s.at // its delta covers time before the window
		} else {
			sum[0] += s.d[0]
			sum[1] += s.d[1]
		}
		count++
	}
	span := now.Sub(first).Seconds()
	if count < 2 || span <= 0 {
		return [2]float64{}
	}
	return [2]float64{sum[0] / span, sum[1] / span}
}

// fling glides from the gesture's velocity: move applies each frame's
// delta and reports false once the content can go no further that way
// (an edge), which ends the glide. done runs when the glide stops,
// however it stopped short of being halted.
func (k *kinetic) fling(move func(dx, dy float64) bool, done func()) {
	v := k.velocity()
	k.n = 0
	speed := gomath.Hypot(v[0], v[1])
	if speed < kineticMinSpeed {
		done()
		return
	}
	tau := kineticTau.Seconds()
	dur := time.Duration(tau * gomath.Log(speed/kineticMinSpeed) * float64(time.Second))
	var travelled [2]float64
	var cancel anim.Cancel
	finished := false
	cancel = anim.Play(anim.Animate(dur, func(t float64) {
		if finished {
			return
		}
		// Distance so far: v*tau*(1 - e^(-elapsed/tau)).
		f := tau * (1 - gomath.Exp(-t*dur.Seconds()/tau))
		dx, dy := float64(v[0]*f)-travelled[0], float64(v[1]*f)-travelled[1]
		travelled[0], travelled[1] = v[0]*f, v[1]*f
		if move(dx, dy) && t < 1 {
			return
		}
		finished = true
		if cancel != nil {
			cancel()
		}
		k.cancel = nil
		done()
	}).Easing(anim.Linear))
	if !finished {
		k.cancel = cancel
	}
}
