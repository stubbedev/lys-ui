package media

import (
	"fmt"
	"image"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/render"
	memory "github.com/stubbedev/memory-go"
)

const (
	decodeAhead = 2
	framePool   = decodeAhead + 2
)

// frameBuf is one of a stream's pooled frame buffers, with the
// identity a GPU renderer keeps its texture by: the buffer's for its
// whole life, while the stream's sequence number versions its pixels.
type frameBuf struct {
	// img is the frame as RGBA, nil until a frame of a YUV source is
	// first converted.
	img *image.RGBA
	// yuv is the frame as decoded, nil for a source decoding no YUV.
	yuv *YUVFrame
	// rgba reports that img holds this frame: always for a source
	// decoding no YUV, once converted for one that does.
	rgba bool
	id   render.ImageID
	// still is a StillSource's frame, the source's to keep, never pooled.
	still bool
}

type decoded struct {
	img *frameBuf
	pts time.Duration
	gen int
	err error
}

type seekRequest struct {
	gen int
	t   time.Duration
}

type decoder struct {
	// pixels holds the frame buffers off the Go heap: a stream's frames
	// cost the GC nothing, and the pages go back to the system when the
	// stream closes rather than when a collection gets to them.
	pixels memory.Shared
	src    Source
	stills StillSource
	// yuv is src decoding YUV frames, nil when it decodes none.
	yuv YUVSource
	// skipRGBA stops the conversion of YUV frames to RGBA as they are
	// decoded.
	skipRGBA atomic.Bool
	held     map[render.ImageID]*frameBuf
	seeker   Seeker
	size     image.Rectangle
	out      chan decoded
	free     chan *frameBuf
	seek     chan seekRequest
	quit     chan struct{}
	done     chan struct{}
	closed   chan struct{}
	closeErr error
}

func startDecoder(src Source, seeker Seeker, info Info) *decoder {
	d := &decoder{
		src: src, seeker: seeker, size: image.Rect(0, 0, info.Width, info.Height),
		out:  make(chan decoded, decodeAhead),
		free: make(chan *frameBuf, framePool),
		seek: make(chan seekRequest, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),

		closed: make(chan struct{}),
	}
	if stills, ok := src.(StillSource); ok {
		d.stills, d.held = stills, map[render.ImageID]*frameBuf{}
	}
	d.yuv = yuvOf(src)
	go d.run()
	return d
}

func (d *decoder) run() {
	defer close(d.done)
	var (
		gen       int
		atEnd     bool
		buf       *frameBuf
		allocated int
		pending   decoded
		owed      bool
	)
	applySeek := func(r seekRequest) {
		gen, atEnd = r.gen, false
		if owed && pending.img != nil && buf == nil {
			buf = pending.img
		} else if owed && pending.img != nil {
			d.recycle(pending.img)
		}
		owed = false
		if err := d.seeker.Seek(r.t); err != nil {
			atEnd = true
			pending, owed = decoded{gen: gen, err: err}, true
		}
	}
	for {
		switch {
		case owed:
			select {
			case d.out <- pending:
				owed = false
			case r := <-d.seek:
				applySeek(r)
			case <-d.quit:
				return
			}
			continue
		case atEnd:
			select {
			case r := <-d.seek:
				applySeek(r)
			case <-d.quit:
				return
			}
			continue
		case d.stills != nil:
			still, pts, err := d.stills.ReadStill()
			pending, owed = decoded{pts: pts, gen: gen, err: err}, true
			if err == nil {
				pending.img = d.stillFrame(still)
			} else {
				atEnd = true
			}
			continue
		case buf == nil && allocated < framePool:
			var err error
			if buf, err = d.newFrameBuf(); err != nil {
				atEnd = true
				pending, owed = decoded{gen: gen, err: err}, true
				continue
			}
			allocated++
		case buf == nil:
			select {
			case buf = <-d.free:
			case r := <-d.seek:
				applySeek(r)
				continue
			case <-d.quit:
				return
			}
		}
		pts, err := d.read(buf)
		pending, owed = decoded{pts: pts, gen: gen, err: err}, true
		if err == nil {
			pending.img, buf = buf, nil
		} else {
			atEnd = true
		}
	}
}

// read decodes the next frame into f: as RGBA from a source decoding
// no YUV, as YUV otherwise, converted to RGBA too unless skipRGBA.
func (d *decoder) read(f *frameBuf) (time.Duration, error) {
	if d.yuv == nil {
		return d.src.ReadFrame(f.img)
	}
	f.rgba = false
	pts, err := d.yuv.ReadFrameYUV(f.yuv)
	if err != nil || d.skipRGBA.Load() {
		return pts, err
	}
	return pts, d.convert(f)
}

// convert fills f's RGBA from its YUV, allocating the RGBA buffer the
// first time. Whoever holds f calls it: the decoder before handing f
// over, the stream after.
func (d *decoder) convert(f *frameBuf) error {
	if f.img == nil {
		pix, err := d.alloc(4 * d.size.Dx() * d.size.Dy())
		if err != nil {
			return err
		}
		f.img = &image.RGBA{Pix: pix, Stride: 4 * d.size.Dx(), Rect: d.size}
	}
	f.yuv.toRGBA(f.img)
	f.rgba = true
	return nil
}

func (d *decoder) requestSeek(r seekRequest) {
	for {
		select {
		case d.seek <- r:
			return
		default:
			select {
			case <-d.seek:
			default:
			}
		}
	}
}

// stillFrame is the frame showing still, one per raster for the
// decoder's life.
func (d *decoder) stillFrame(still render.StillImage) *frameBuf {
	if f, ok := d.held[still.ID]; ok {
		return f
	}
	f := &frameBuf{img: still.RGBA, rgba: true, id: still.ID, still: true}
	d.held[still.ID] = f
	return f
}

func (d *decoder) recycle(img *frameBuf) {
	if img == nil || img.still {
		return
	}
	select {
	case d.free <- img:
	default:
	}
}

func (d *decoder) alloc(n int) ([]byte, error) {
	pix, err := memory.MakeSlice[byte](&d.pixels, n)
	if err != nil {
		return nil, fmt.Errorf("media: a %dx%d frame buffer: %w", d.size.Dx(), d.size.Dy(), err)
	}
	return pix.Elems(), nil
}

// newFrameBuf allocates one frame buffer of the stream's size off the
// Go heap: YUV planes for a source decoding YUV, RGBA for any other.
func (d *decoder) newFrameBuf() (*frameBuf, error) {
	f := &frameBuf{id: render.NewImageID()}
	w, h := d.size.Dx(), d.size.Dy()
	if d.yuv == nil {
		pix, err := d.alloc(4 * w * h)
		if err != nil {
			return nil, err
		}
		f.img, f.rgba = &image.RGBA{Pix: pix, Stride: 4 * w, Rect: d.size}, true
		return f, nil
	}
	pix, err := d.alloc(YUVFrameBytes(w, h))
	if err != nil {
		return nil, err
	}
	f.yuv = &YUVFrame{Width: w, Height: h, Pix: pix}
	return f, nil
}

// close stops the decoder and returns at once: closing the source and
// joining the goroutine may wait on a subprocess, so they run on their
// own goroutine. The frame buffers go once nothing can read them: the
// decoding goroutine has stopped, the stream has dropped its frames,
// and every recording that may draw one has ended (render.Retire).
func (d *decoder) close() {
	close(d.quit)
	go func() {
		defer close(d.closed)
		d.closeErr = d.src.Close()
		<-d.done
		render.Retire(func() {
			if err := d.pixels.Close(); err != nil {
				panic(fmt.Sprintf("media: free the frame buffers: %v", err))
			}
		})
	}()
}
