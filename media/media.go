// Package media is lys-ui's playback framework, GTK's GtkMediaStream in
// pure Go: a Source decodes frames (and, when Audible, sound), and a
// Stream plays one on the loop with play, pause, seek, loop, volume and
// a clock that follows the audio when there is any. widget.Video paints
// a Stream and widget.MediaControls drives it.
//
// Built-in sources: animated images (NewAnimation), YUV4MPEG2 (NewY4M,
// decoded here), and any file ffmpeg reads (OpenFile, which runs
// ffmpeg and ffprobe from PATH as subprocesses). Sound plays through
// the PulseAudio native protocol, which PulseAudio and pipewire-pulse
// serve.
//
//	src, err := media.OpenFile("talk.webm")
//	stream := media.NewStream(src)
//	video := widget.NewVideo(face, 13, stream)
//	stream.Play()
package media

import (
	"errors"
	"image"
	"io"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// The PCM layout every Audible track delivers: interleaved signed
// 16-bit little-endian samples.
const (
	AudioRate     = 48000
	AudioChannels = 2
	audioFrame    = 2 * AudioChannels
)

// Info describes a source's video.
type Info struct {
	// Width and Height are the frame size in pixels.
	Width, Height int
	// Duration is the length; zero when the source cannot know it.
	Duration time.Duration
}

// Source decodes a video's frames in presentation order. ReadFrame and
// Seek are called from one goroutine; Close may be called from another
// at any time and must unblock a ReadFrame in progress.
type Source interface {
	Info() Info
	// ReadFrame decodes the next frame into dst, which is Info's size,
	// and returns its presentation time. It returns io.EOF after the
	// last frame.
	ReadFrame(dst *image.RGBA) (time.Duration, error)
	Close() error
}

// StillSource is a Source whose frames are rasters it keeps unchanged for
// its whole life, a decoded animation's: the stream shows them as they
// are, under their own identity, so each is copied nowhere and a GPU
// uploads it once however often it plays. ReadStill takes the place of
// ReadFrame.
type StillSource interface {
	ReadStill() (render.StillImage, time.Duration, error)
}

// Seeker is a Source that can reposition: after Seek(t) the next
// ReadFrame returns the frame showing at t.
type Seeker interface {
	Seek(t time.Duration) error
}

// Audible is a Source with a sound track.
type Audible interface {
	// OpenAudio returns the track from t as PCM in the AudioRate,
	// AudioChannels layout; closing it stops the decoding.
	OpenAudio(t time.Duration) (io.ReadCloser, error)
}

var errClosed = errors.New("media: the source is closed")

type muted struct{ Source }

// WithoutAudio returns src with its sound track hidden, for playback
// with no audio output at all (a muted Stream still opens one).
func WithoutAudio(src Source) Source {
	if s, ok := src.(Seeker); ok {
		return seekableMuted{muted{src}, s}
	}
	return muted{src}
}

type seekableMuted struct {
	muted
	Seeker
}
