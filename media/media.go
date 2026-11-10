// Package media is lys-ui's playback framework, GTK's GtkMediaStream in
// pure Go: a Source decodes frames (and, when Audible, sound), and a
// Stream plays one on the loop with play, pause, seek, loop, volume and
// a clock that follows the audio when there is any. widget.Video paints
// a Stream and widget.MediaControls drives it.
//
// Built-in sources: animated images (NewAnimation), YUV4MPEG2 (NewY4M),
// both decoded on the CPU, and MP4 and Matroska/WebM files (OpenFile),
// demuxed in process, their H.264 and H.265 video decoded on the GPU
// the windows draw with and their AAC, Opus and Vorbis sound in Go.
// Sound plays through the PulseAudio native protocol, which PulseAudio
// and pipewire-pulse serve.
//
//	src, err := media.OpenFile("talk.mp4")
//	stream := media.NewStream(src, "talk")
//	video := widget.NewVideo(face, 13, stream)
//	stream.Play()
package media

import (
	"errors"
	"fmt"
	"image"
	"io"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/pulse/native"
	"github.com/stubbedev/lys-lib/render"
)

// AudioFormat is the PCM an Audible source delivers: interleaved
// signed 16-bit little-endian samples at Rate, one per speaker of
// Layout in its order. The sound server converts it to its sink's.
type AudioFormat struct {
	Rate   int
	Layout native.ChannelMap
}

func (f AudioFormat) frameBytes() int { return 2 * len(f.Layout) }

// Info describes a source's video.
type Info struct {
	// Width and Height are the frame size in pixels.
	Width, Height int
	// Duration is the length; zero when the source cannot know it.
	Duration time.Duration
}

// Source is a video a Stream plays: a FrameSource, decoded on the CPU,
// or a TextureSource, decoded on the GPU. Close may be called from any
// goroutine at any time and must unblock a read in progress.
type Source interface {
	Info() Info
	Close() error
}

// FrameSource is a Source decoding its frames on the CPU, in
// presentation order. ReadFrame and Seek are called from one goroutine.
type FrameSource interface {
	Source
	// ReadFrame decodes the next frame into dst, which is Info's size,
	// and returns its presentation time. It returns io.EOF after the
	// last frame.
	ReadFrame(dst *image.RGBA) (time.Duration, error)
}

// TextureSource is a Source whose video the GPU decodes: it hands out
// the video track's packets in decode order, and the Stream decodes
// them on the device of the Video showing it (Stream.UseDevice).
// ReadPacket and Seek are called from one goroutine, NewVideoDecoder
// from the device's.
type TextureSource interface {
	Source
	Seeker
	// NewVideoDecoder opens a decoder of the video track on dev, an
	// error naming the codec and the file where dev decodes none. A
	// Stream that moves to another device closes the decoder and opens
	// one on the next.
	NewVideoDecoder(dev *gpu.Device) (VideoDecoder, error)
	// ReadPacket returns the video track's next packet, its Data valid
	// until the next ReadPacket or Seek, and io.EOF after the last.
	// After Seek(t) the packets start at the keyframe at or before t.
	ReadPacket() (demux.Packet, error)
}

// VideoDecoder decodes a TextureSource's packets on one GPU device,
// what a *gpu.VideoDecoder does: Decode takes the packets in decode
// order, Next hands out the frames they complete in presentation order,
// nil when there is none yet, Flush hands out the frames held back at
// the track's end, Reset drops what is decoding for a seek, and Close
// releases the decoder with every frame it handed out. A device that
// was lost fails them with gpu.ErrDeviceLost.
type VideoDecoder interface {
	Decode(p demux.Packet) error
	Next() (*gpu.VideoFrame, error)
	Flush()
	Reset() error
	Close()
}

// StillSource is a Source whose frames are rasters it keeps unchanged for
// its whole life, a decoded animation's: the stream shows them as they
// are, under their own identity, so each is copied nowhere and a GPU
// uploads it once however often it plays. ReadStill takes the place of
// ReadFrame.
type StillSource interface {
	ReadStill() (render.StillImage, time.Duration, error)
}

// Seeker is a Source that can reposition: after Seek(t) the next frame
// read is the one showing at t.
type Seeker interface {
	Seek(t time.Duration) error
}

// Audible is a Source with a sound track.
type Audible interface {
	// AudioFormat is the layout of the PCM OpenAudio returns.
	AudioFormat() AudioFormat
	// OpenAudio returns the track from t as PCM in AudioFormat;
	// closing it stops the decoding.
	OpenAudio(t time.Duration) (io.ReadCloser, error)
}

var (
	errClosed = errors.New("media: the source is closed")
	errNoYUV  = errors.New("media: the source decodes no 4:2:0 YUV")
)

type mutedFrames struct{ FrameSource }

type seekableMutedFrames struct {
	mutedFrames
	Seeker
}

type mutedTextures struct{ TextureSource }

// WithoutAudio returns src with its sound track hidden, for playback
// with no audio output at all (a muted Stream still opens one).
func WithoutAudio(src Source) Source {
	switch s := src.(type) {
	case TextureSource:
		return mutedTextures{s}
	case FrameSource:
		if seeker, ok := s.(Seeker); ok {
			return seekableMutedFrames{mutedFrames{s}, seeker}
		}
		return mutedFrames{s}
	}
	panic(undecodable(src))
}

func undecodable(src Source) string {
	return fmt.Sprintf("media: a %T is neither a FrameSource nor a TextureSource", src)
}
