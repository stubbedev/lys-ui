package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"time"
)

const probeTimeout = 10 * time.Second

// OpenFile opens a media file through ffmpeg, which decodes anything it
// knows (H.264, VP9, AV1, ...): ffprobe reads the streams, an ffmpeg
// subprocess decodes the first video stream, and a second one decodes
// the first audio stream when there is one, so the source is Audible
// exactly when the file has sound. Both binaries come from PATH; their
// absence is an error, as is a file without video. Seeking restarts
// the decoders at the new position.
func OpenFile(path string) (Source, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", path, err)
	}
	probeBin, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", path, err)
	}
	input := "file:" + path
	duration, audio, err := probe(probeBin, input)
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", path, err)
	}
	s := &ffmpegSource{bin: bin, input: input}
	if err := s.Seek(0); err != nil {
		return nil, fmt.Errorf("media: open %s: %w", path, err)
	}
	s.info = Info{Width: s.dec.h.width, Height: s.dec.h.height, Duration: duration}
	if audio {
		return &audibleFFmpeg{s}, nil
	}
	return s, nil
}

type probeStream struct {
	CodecType string `json:"codec_type"`
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probe(bin, input string) (time.Duration, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-v", "error", "-of", "json", "-show_entries", "format=duration:stream=codec_type", "-i", input) //nolint:gosec // ffprobe from PATH with an argv the package builds; the input is passed as file:PATH, never through a shell
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, false, fmt.Errorf("ffprobe: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	var r probeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return 0, false, fmt.Errorf("ffprobe output: %w", err)
	}
	has := func(kind string) bool {
		return slices.ContainsFunc(r.Streams, func(s probeStream) bool { return s.CodecType == kind })
	}
	if !has("video") {
		return 0, false, errors.New("the file has no video stream")
	}
	var duration time.Duration
	if secs, err := strconv.ParseFloat(r.Format.Duration, 64); err == nil && secs > 0 {
		duration = time.Duration(secs * float64(time.Second))
	}
	return duration, has("audio"), nil
}

type ffmpegSource struct {
	bin, input string
	info       Info
	mu         sync.Mutex
	video      *proc
	dec        *y4mStream
	closed     bool
}

func (s *ffmpegSource) Info() Info { return s.info }

func seconds(t time.Duration) string { return strconv.FormatFloat(t.Seconds(), 'f', 6, 64) }

func (s *ffmpegSource) Seek(t time.Duration) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errClosed
	}
	p, err := startProc(s.bin, "video", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", seconds(t), "-i", s.input, "-map", "0:v:0", "-an", "-sn",
		"-vf", "scale=out_color_matrix=bt601:out_range=full,format=yuv420p",
		"-f", "yuv4mpegpipe", "pipe:1")
	if err != nil {
		return err
	}
	dec, _, err := openY4MStream(p)
	if err != nil {
		_ = p.Close()
		return fmt.Errorf("media: ffmpeg video at %v: %w", t, err)
	}
	dec.base = t
	if s.info.Width != 0 && (dec.h.width != s.info.Width || dec.h.height != s.info.Height) {
		_ = p.Close()
		return fmt.Errorf("media: ffmpeg video at %v is %dx%d, the stream %dx%d", t, dec.h.width, dec.h.height, s.info.Width, s.info.Height)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = p.Close()
		return errClosed
	}
	old := s.video
	s.video, s.dec = p, dec
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (s *ffmpegSource) decoder() *y4mStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dec
}

func (s *ffmpegSource) ReadFrame(dst *image.RGBA) (time.Duration, error) {
	return s.decoder().ReadFrame(dst)
}

func (s *ffmpegSource) YUV() (YUVColor, bool) { return s.decoder().YUV() }

func (s *ffmpegSource) ReadFrameYUV(dst *YUVFrame) (time.Duration, error) {
	return s.decoder().ReadFrameYUV(dst)
}

func (s *ffmpegSource) Close() error {
	s.mu.Lock()
	s.closed = true
	p := s.video
	s.mu.Unlock()
	return p.Close()
}

type audibleFFmpeg struct{ *ffmpegSource }

func (s *audibleFFmpeg) OpenAudio(t time.Duration) (io.ReadCloser, error) {
	return startProc(s.bin, "audio", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", seconds(t), "-i", s.input, "-map", "0:a:0", "-vn", "-sn",
		"-f", "s16le", "-ac", strconv.Itoa(AudioChannels), "-ar", strconv.Itoa(AudioRate), "pipe:1")
}

const stderrLimit = 4096

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := stderrLimit - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

type proc struct {
	what     string
	cmd      *exec.Cmd
	out      io.ReadCloser
	stderr   cappedBuffer
	waitOnce sync.Once
	waitErr  error
}

func startProc(bin, what string, args ...string) (*proc, error) {
	p := &proc{what: what, cmd: exec.Command(bin, args...)} //nolint:gosec // ffmpeg from PATH with an argv the package builds; the input is passed as file:PATH, never through a shell
	p.cmd.Stderr = &p.stderr
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("media: ffmpeg %s: %w", what, err)
	}
	p.out = out
	if err := p.cmd.Start(); err != nil {
		return nil, fmt.Errorf("media: start ffmpeg %s: %w", what, err)
	}
	return p, nil
}

func (p *proc) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	if errors.Is(err, io.EOF) {
		if werr := p.wait(); werr != nil {
			return n, werr
		}
	}
	return n, err
}

func (p *proc) wait() error {
	p.waitOnce.Do(func() {
		if err := p.cmd.Wait(); err != nil {
			p.waitErr = fmt.Errorf("media: ffmpeg %s: %w: %s", p.what, err, bytes.TrimSpace(p.stderr.Bytes()))
		}
	})
	return p.waitErr
}

func (p *proc) Close() error {
	_ = p.cmd.Process.Kill()
	_ = p.wait()
	return nil
}
