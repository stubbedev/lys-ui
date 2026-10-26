package media

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

const maxY4MLine = 4096

type y4mHeader struct {
	width, height    int
	rateNum, rateDen int64
	mono             bool
	xs, ys           uint
	full             bool
}

var y4mChroma = map[string]struct{ xs, ys uint }{
	"420jpeg": {1, 1}, "420paldv": {1, 1}, "420mpeg2": {1, 1}, "420": {1, 1},
	"422": {1, 0}, "444": {0, 0},
}

func parseY4MHeader(line []byte) (y4mHeader, error) {
	fields := strings.Fields(string(line))
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return y4mHeader{}, errors.New("media: not a YUV4MPEG2 stream")
	}
	h := y4mHeader{xs: 1, ys: 1}
	for _, f := range fields[1:] {
		key, val := f[0], f[1:]
		var err error
		switch key {
		case 'W':
			h.width, err = strconv.Atoi(val)
		case 'H':
			h.height, err = strconv.Atoi(val)
		case 'F':
			h.rateNum, h.rateDen, err = parseRatio(val)
		case 'I':
			if val != "p" && val != "?" {
				return y4mHeader{}, fmt.Errorf("media: y4m interlacing %q is not supported, only progressive frames", val)
			}
		case 'C':
			if val == "mono" {
				h.mono = true
				break
			}
			c, ok := y4mChroma[val]
			if !ok {
				return y4mHeader{}, fmt.Errorf("media: y4m colorspace %q is not supported (8-bit 420, 422, 444 and mono are)", val)
			}
			h.xs, h.ys = c.xs, c.ys
		case 'X':
			switch val {
			case "COLORRANGE=FULL":
				h.full = true
			case "COLORRANGE=LIMITED":
				h.full = false
			}
		}
		if err != nil {
			return y4mHeader{}, fmt.Errorf("media: y4m header field %q: %w", f, err)
		}
	}
	if h.width <= 0 || h.height <= 0 {
		return y4mHeader{}, fmt.Errorf("media: y4m frame size %dx%d is not positive", h.width, h.height)
	}
	if h.rateNum <= 0 || h.rateDen <= 0 {
		return y4mHeader{}, errors.New("media: y4m header has no positive frame rate")
	}
	return h, nil
}

func parseRatio(s string) (int64, int64, error) {
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, errors.New("want num:den")
	}
	num, err := strconv.ParseInt(a, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	den, err := strconv.ParseInt(b, 10, 64)
	return num, den, err
}

func (h y4mHeader) chromaSize() (w, ht int) {
	if h.mono {
		return 0, 0
	}
	return (h.width + 1<<h.xs - 1) >> h.xs, (h.height + 1<<h.ys - 1) >> h.ys
}

func (h y4mHeader) frameBytes() int {
	cw, ch := h.chromaSize()
	return h.width*h.height + 2*cw*ch
}

func mulDiv(a, b, c int64) int64 {
	q, _ := mulDivRem(a, b, c)
	return q
}

func mulDivCeil(a, b, c int64) int64 {
	q, r := mulDivRem(a, b, c)
	if r != 0 {
		q++
	}
	return q
}

func mulDivRem(a, b, c int64) (int64, uint64) {
	hi, lo := bits.Mul64(uint64(max(a, 0)), uint64(max(b, 0)))
	q, r := bits.Div64(hi, lo, uint64(max(c, 0)))
	return int64(min(q, math.MaxInt64)), r
}

func (h y4mHeader) pts(n int64) time.Duration {
	return time.Duration(mulDivCeil(n, h.rateDen*int64(time.Second), h.rateNum))
}

func (h y4mHeader) frameAt(t time.Duration) int64 {
	return mulDiv(int64(max(t, 0)), h.rateNum, h.rateDen*int64(time.Second))
}

type y4mStream struct {
	h      y4mHeader
	br     *bufio.Reader
	closer io.Closer
	planes []byte
	n      int64
	base   time.Duration
	bare   bool
}

// NewY4M reads a YUV4MPEG2 stream (8-bit 4:2:0, 4:2:2, 4:4:4 or mono,
// BT.601, limited range unless XCOLORRANGE=FULL). When r is also an
// io.ReadSeeker the source seeks and knows its duration; that needs
// every frame header to be a bare FRAME line, as ffmpeg writes. Close
// closes r when it is an io.Closer.
func NewY4M(r io.Reader) (Source, error) {
	rs, seekable := r.(io.ReadSeeker)
	var start int64
	if seekable {
		var err error
		if start, err = rs.Seek(0, io.SeekCurrent); err != nil {
			return nil, fmt.Errorf("media: y4m: find the stream start: %w", err)
		}
	}
	s, headerLen, err := openY4MStream(r)
	if err != nil {
		return nil, err
	}
	if !seekable {
		return s, nil
	}
	s.bare = true
	data := start + headerLen
	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("media: y4m: find the stream end: %w", err)
	}
	if _, err := rs.Seek(data, io.SeekStart); err != nil {
		return nil, fmt.Errorf("media: y4m: return to the first frame: %w", err)
	}
	s.br.Reset(rs)
	stride := int64(len("FRAME\n") + s.h.frameBytes())
	return &y4mFile{y4mStream: s, rs: rs, data: data, stride: stride, frames: (end - data) / stride}, nil
}

func openY4MStream(r io.Reader) (*y4mStream, int64, error) {
	br := bufio.NewReader(r)
	line, err := readLine(br)
	if err != nil {
		return nil, 0, fmt.Errorf("media: y4m header: %w", err)
	}
	h, err := parseY4MHeader(line)
	if err != nil {
		return nil, 0, err
	}
	s := &y4mStream{h: h, br: br, planes: make([]byte, h.frameBytes())}
	if c, ok := r.(io.Closer); ok {
		s.closer = c
	}
	return s, int64(len(line)) + 1, nil
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxY4MLine {
			return nil, fmt.Errorf("a header line runs past %d bytes", maxY4MLine)
		}
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return nil, io.ErrUnexpectedEOF
		default:
			return nil, err
		}
	}
}

func (s *y4mStream) Info() Info { return Info{Width: s.h.width, Height: s.h.height} }

func (s *y4mStream) ReadFrame(dst *image.RGBA) (time.Duration, error) {
	if dst.Rect.Dx() != s.h.width || dst.Rect.Dy() != s.h.height {
		return 0, fmt.Errorf("media: y4m frame is %dx%d, the buffer %v", s.h.width, s.h.height, dst.Rect)
	}
	line, err := readLine(s.br)
	if errors.Is(err, io.EOF) {
		return 0, io.EOF
	}
	if err != nil {
		return 0, fmt.Errorf("media: y4m frame %d header: %w", s.n, err)
	}
	if !bytes.HasPrefix(line, []byte("FRAME")) || s.bare && len(line) != len("FRAME") {
		return 0, fmt.Errorf("media: y4m frame %d header %.40q, want a bare FRAME line", s.n, line)
	}
	if _, err := io.ReadFull(s.br, s.planes); err != nil {
		return 0, fmt.Errorf("media: y4m frame %d is truncated: %w", s.n, err)
	}
	yuvToRGBA(dst, s.planes, s.h)
	pts := s.base + s.h.pts(s.n)
	s.n++
	return pts, nil
}

func (s *y4mStream) Close() error {
	if s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

type y4mFile struct {
	*y4mStream
	rs           io.ReadSeeker
	data, stride int64
	frames       int64
}

func (f *y4mFile) Info() Info {
	i := f.y4mStream.Info()
	i.Duration = f.h.pts(f.frames)
	return i
}

func (f *y4mFile) Seek(t time.Duration) error {
	n := min(f.h.frameAt(t), max(f.frames-1, 0))
	if _, err := f.rs.Seek(f.data+n*f.stride, io.SeekStart); err != nil {
		return fmt.Errorf("media: y4m seek to frame %d: %w", n, err)
	}
	f.br.Reset(f.rs)
	f.n = n
	return nil
}

type yuvMatrix struct{ ky, yoff, rv, gu, gv, bu int32 }

var (
	bt601Full    = yuvMatrix{ky: 65536, yoff: 0, rv: 91881, gu: 22554, gv: 46802, bu: 116130}
	bt601Limited = yuvMatrix{ky: 76309, yoff: 16, rv: 104597, gu: 25675, gv: 53279, bu: 132201}
)

func clamp8(v int32) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v)
}

func yuvToRGBA(dst *image.RGBA, planes []byte, h y4mHeader) {
	m := bt601Limited
	if h.full {
		m = bt601Full
	}
	w := h.width
	cw, ch := h.chromaSize()
	luma := planes[:w*h.height]
	u, v := planes[len(luma):len(luma)+cw*ch], planes[len(luma)+cw*ch:]
	for y := range h.height {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+4*w]
		yr := luma[y*w : (y+1)*w]
		var ur, vr []byte
		if !h.mono {
			off := (y >> h.ys) * cw
			ur, vr = u[off:off+cw], v[off:off+cw]
		}
		for x := range w {
			yy := (int32(yr[x])-m.yoff)*m.ky + 1<<15
			var cb, cr int32
			if !h.mono {
				cb, cr = int32(ur[x>>h.xs])-128, int32(vr[x>>h.xs])-128
			}
			p := row[4*x : 4*x+4]
			p[0] = clamp8((yy + m.rv*cr) >> 16)
			p[1] = clamp8((yy - m.gu*cb - m.gv*cr) >> 16)
			p[2] = clamp8((yy + m.bu*cb) >> 16)
			p[3] = 255
		}
	}
}
