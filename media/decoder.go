package media

import (
	"image"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

const (
	decodeAhead = 2
	framePool   = decodeAhead + 2
)

// frameBuf is one of a stream's pooled frame buffers, with the
// identity a GPU renderer keeps its texture by: the buffer's for its
// whole life, while the stream's sequence number versions its pixels.
type frameBuf struct {
	img *image.RGBA
	id  render.ImageID
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
	src    Source
	seeker Seeker
	size   image.Rectangle
	out    chan decoded
	free   chan *frameBuf
	seek   chan seekRequest
	quit   chan struct{}
	done   chan struct{}
}

func startDecoder(src Source, seeker Seeker, info Info) *decoder {
	d := &decoder{
		src: src, seeker: seeker, size: image.Rect(0, 0, info.Width, info.Height),
		out:  make(chan decoded, decodeAhead),
		free: make(chan *frameBuf, framePool),
		seek: make(chan seekRequest, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
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
		pending   *decoded
	)
	applySeek := func(r seekRequest) {
		gen, atEnd = r.gen, false
		if pending != nil && pending.img != nil && buf == nil {
			buf = pending.img
		} else if pending != nil && pending.img != nil {
			d.recycle(pending.img)
		}
		pending = nil
		if err := d.seeker.Seek(r.t); err != nil {
			atEnd = true
			pending = &decoded{gen: gen, err: err}
		}
	}
	for {
		switch {
		case pending != nil:
			select {
			case d.out <- *pending:
				pending = nil
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
		case buf == nil && allocated < framePool:
			buf = &frameBuf{img: image.NewRGBA(d.size), id: render.NewImageID()}
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
		pts, err := d.src.ReadFrame(buf.img)
		msg := decoded{pts: pts, gen: gen, err: err}
		if err == nil {
			msg.img, buf = buf, nil
		} else {
			atEnd = true
		}
		pending = &msg
	}
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

func (d *decoder) recycle(img *frameBuf) {
	if img == nil {
		return
	}
	select {
	case d.free <- img:
	default:
	}
}

func (d *decoder) close() error {
	close(d.quit)
	err := d.src.Close()
	<-d.done
	return err
}
