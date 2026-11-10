package media

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/media/demux"
)

const (
	// packetPool is how many packets the packet goroutine reads ahead.
	packetPool = 16
	// texturesAhead is how many frames the stream decodes ahead of the
	// one it shows.
	texturesAhead = 3
	// heldTextures is how many frames the stream holds at most: the one
	// shown and the ones decoded ahead of it, within the decoder's
	// allowance.
	heldTextures = 8
	// catchUpPackets is how many packets a tick decodes while the frame
	// at a seek target is still to come, the frames before it dropped.
	catchUpPackets = 8
)

// packet is one of the packet goroutine's pooled packets: its data is
// copied into a buffer the packet keeps, so reading allocates nothing
// once the buffers have grown to the stream's largest packets.
type packet struct {
	p    demux.Packet
	data []byte
	gen  int
	err  error
}

// packetReader reads a TextureSource's packets on a goroutine of its
// own, a pool's worth ahead, and seeks it when asked.
type packetReader struct {
	src  TextureSource
	out  chan *packet
	free chan *packet
	seek chan seekRequest
	quit chan struct{}
	done chan struct{}
}

func startPacketReader(src TextureSource, first seekRequest, seek bool) *packetReader {
	r := &packetReader{
		src:  src,
		out:  make(chan *packet, packetPool),
		free: make(chan *packet, packetPool),
		seek: make(chan seekRequest, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	for range packetPool {
		r.free <- &packet{}
	}
	if seek {
		r.seek <- first
	}
	go r.run(first.gen)
	return r
}

func (r *packetReader) run(gen int) {
	defer close(r.done)
	var (
		atEnd bool
		fail  error
		owed  *packet
	)
	seekTo := func(req seekRequest) {
		gen, atEnd, fail = req.gen, false, nil
		if owed != nil {
			r.free <- owed
			owed = nil
		}
		if err := r.src.Seek(req.t); err != nil {
			atEnd, fail = true, err
		}
	}
	for {
		switch {
		case owed != nil:
			select {
			case r.out <- owed:
				owed = nil
			case req := <-r.seek:
				seekTo(req)
			case <-r.quit:
				return
			}
			continue
		case atEnd && fail == nil:
			select {
			case req := <-r.seek:
				seekTo(req)
			case <-r.quit:
				return
			}
			continue
		}
		var b *packet
		select {
		case b = <-r.free:
		case req := <-r.seek:
			seekTo(req)
			continue
		case <-r.quit:
			return
		}
		owed, b.gen = b, gen
		if fail != nil {
			b.err, fail = fail, nil
			continue
		}
		p, err := r.src.ReadPacket()
		if b.err = err; err != nil {
			atEnd = true
			continue
		}
		b.data = append(b.data[:0], p.Data...)
		b.p, b.p.Data = p, b.data
	}
}

func (r *packetReader) requestSeek(req seekRequest) {
	for {
		select {
		case r.seek <- req:
			return
		default:
			select {
			case <-r.seek:
			default:
			}
		}
	}
}

func (r *packetReader) recycle(b *packet) { r.free <- b }

// textureVideo is a Stream's GPU decoding: a TextureSource's packets,
// read on their own goroutine, decoded on the loop into frames of the
// device the Video showing the stream draws with.
type textureVideo struct {
	src TextureSource
	dev *gpu.Device
	dec *gpu.VideoDecoder
	rd  *packetReader
	// queue are the frames decoded ahead of cur, in presentation order,
	// and candidate the one that may show at the seek target, held
	// until a later one tells.
	queue     []*gpu.VideoFrame
	cur       *gpu.VideoFrame
	candidate *gpu.VideoFrame
	// target is the seek target: frames over before it are dropped as
	// they decode.
	target time.Duration
	// flushed marks the track's end reached and the decoder flushed,
	// landed a frame at the seek target decoded.
	flushed, landed bool
	// closed is closed once the source closed, closeErr why it failed.
	closed   chan struct{}
	closeErr error
}

func newTextureVideo(src TextureSource) *textureVideo {
	return &textureVideo{src: src, queue: make([]*gpu.VideoFrame, 0, heldTextures-1), closed: make(chan struct{})}
}

// attach decodes on dev from t on, the stream at generation gen.
func (v *textureVideo) attach(dev *gpu.Device, gen int, t time.Duration) error {
	dec, err := v.src.NewVideoDecoder(dev)
	if err != nil {
		return err
	}
	v.dev, v.dec, v.target = dev, dec, t
	v.rd = startPacketReader(v.src, seekRequest{gen: gen, t: t}, t > 0)
	return nil
}

// pump moves the stream's decoding along: it takes the frames the
// packets decoded on earlier ticks completed, whose decodes are done by
// now, then decodes the packets that keep texturesAhead frames ahead.
// It reports the track's end and the first failure.
func (v *textureVideo) pump(gen int) (queued []*gpu.VideoFrame, ended bool, err error) {
	start := len(v.queue)
	for len(v.queue) < cap(v.queue)-1 {
		f, err := v.dec.Next()
		if err != nil {
			return nil, false, fmt.Errorf("media: decode the video: %w", err)
		}
		if f == nil {
			if v.flushed && v.candidate != nil {
				v.land(nil)
			}
			ended = v.flushed
			break
		}
		if v.landed {
			v.queue = append(v.queue, f)
			continue
		}
		v.arrive(f)
	}
	budget := texturesAhead - len(v.queue)
	if !v.landed {
		budget = catchUpPackets
	}
	for ; budget > 0 && !v.flushed; budget-- {
		var b *packet
		select {
		case b = <-v.rd.out:
		default:
			return v.queue[start:], ended, nil
		}
		err := v.decode(b, gen)
		v.rd.recycle(b)
		if err != nil {
			return nil, false, err
		}
	}
	return v.queue[start:], ended, nil
}

// arrive takes f, decoded on the way to the seek target: a frame over
// before the target is dropped, one of unknown length presenting at or
// before it is the candidate for the frame showing there until a later
// one comes, and the first frame showing at the target or after it
// lands the seek.
func (v *textureVideo) arrive(f *gpu.VideoFrame) {
	pts, d := f.PTS(), f.Duration()
	switch {
	case pts > v.target:
		v.land(f)
	case d > 0 && pts+d <= v.target:
		f.Release()
	case d > 0:
		v.releaseCandidate()
		v.land(f)
	default:
		v.releaseCandidate()
		v.candidate = f
	}
}

// land queues the candidate, then f when there is one: the frames
// showing from the seek target on.
func (v *textureVideo) land(f *gpu.VideoFrame) {
	if v.candidate != nil {
		v.queue = append(v.queue, v.candidate)
		v.candidate = nil
	}
	if f != nil {
		v.queue = append(v.queue, f)
	}
	v.landed = true
}

func (v *textureVideo) releaseCandidate() {
	if v.candidate != nil {
		v.candidate.Release()
		v.candidate = nil
	}
}

func (v *textureVideo) decode(b *packet, gen int) error {
	switch {
	case b.gen != gen:
		return nil
	case errors.Is(b.err, io.EOF):
		v.dec.Flush()
		v.flushed = true
		return nil
	case b.err != nil:
		return b.err
	}
	if err := v.dec.Decode(b.p); err != nil {
		return fmt.Errorf("media: decode the video: %w", err)
	}
	return nil
}

// show makes the queue's first n frames shown in turn, the last of them
// current, releasing the ones they replace.
func (v *textureVideo) show(n int) {
	for _, f := range v.queue[:n] {
		if v.cur != nil {
			v.cur.Release()
		}
		v.cur = f
	}
	v.queue = v.queue[:copy(v.queue, v.queue[n:])]
}

// seek restarts the decoding at the keyframe before t.
func (v *textureVideo) seek(gen int, t time.Duration) error {
	if v.dec == nil {
		return nil
	}
	v.releaseQueue()
	v.flushed, v.landed, v.target = false, false, t
	if err := v.dec.Reset(); err != nil {
		return fmt.Errorf("media: seek the video decoder to %v: %w", t, err)
	}
	v.rd.requestSeek(seekRequest{gen: gen, t: t})
	return nil
}

func (v *textureVideo) releaseQueue() {
	v.releaseCandidate()
	for _, f := range v.queue {
		f.Release()
	}
	v.queue = v.queue[:0]
}

// close releases the frames and the decoder on the loop, which owns
// them, and returns at once: the packet goroutine and the source go on
// a goroutine of their own.
func (v *textureVideo) close() {
	v.releaseQueue()
	if v.cur != nil {
		v.cur.Release()
		v.cur = nil
	}
	if v.dec != nil {
		v.dec.Close()
	}
	rd := v.rd
	if rd != nil {
		close(rd.quit)
	}
	go func() {
		defer close(v.closed)
		if rd != nil {
			<-rd.done
		}
		v.closeErr = v.src.Close()
	}()
}
