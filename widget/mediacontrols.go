package widget

import (
	"fmt"
	"time"

	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// MediaControls is GTK's GtkMediaControls: a play/pause button, a seek
// bar over the stream's duration, the position and duration, and, when
// the stream has sound, a mute button and a volume slider. The seek
// bar is inert for a stream that cannot seek or does not know its
// length, and every control is inert once the stream closed or failed.
// It styles as `controls` with `.play`, `.seek`, `.time`, `.mute` and
// `.volume` parts.
type MediaControls struct {
	composite
	face     render.Font
	sizePx   float64
	row      *Box
	play     *Button
	playIcon *Icon
	seek     *Slider
	clock    *Label
	mute     *Button
	muteIcon *Icon
	volume   *Slider
	stream   *media.Stream
	unsub    func()
	syncing  bool
}

// NewMediaControls returns controls for stream, which may be nil.
func NewMediaControls(face render.Font, sizePx float64, stream *media.Stream) *MediaControls {
	face = requireFace("widget.NewMediaControls", face)
	m := &MediaControls{face: face, sizePx: sizePx, row: NewBox(Row, 6, 4)}
	icon := int(sizePx)
	m.playIcon = NewThemeIcon("media-playback-start-symbolic", icon)
	m.play = NewButton(m.playIcon, 4, 4)
	m.play.AddClass("play")
	m.play.OnClick = m.togglePlay
	m.clock = NewLabel(face, sizePx, "", 0)
	m.clock.AddClass("time", css.DimLabel)
	m.muteIcon = NewThemeIcon("audio-volume-high-symbolic", icon)
	m.mute = NewButton(m.muteIcon, 4, 4)
	m.mute.AddClass("mute")
	m.mute.OnClick = func() { m.stream.SetMuted(!m.stream.Muted()) }
	m.volume = NewSlider(0, 1, 0.01, 1)
	m.volume.AddClass("volume")
	m.volume.OnChanged = func(v float64) {
		if !m.syncing {
			m.stream.SetVolume(v)
		}
	}
	m.initComposite(m, m.row)
	m.SetElement("controls")
	m.SetStream(stream)
	return m
}

// Stream returns the controlled stream.
func (m *MediaControls) Stream() *media.Stream { return m.stream }

// SetStream controls stream instead; nil leaves the controls inert.
func (m *MediaControls) SetStream(stream *media.Stream) {
	if m.unsub != nil {
		m.unsub()
		m.unsub = nil
	}
	m.stream = stream
	duration := 0.0
	if stream != nil {
		duration = stream.Duration().Seconds()
		m.unsub = stream.Subscribe(m.sync)
	}
	m.seek = NewSlider(0, max(duration, 1), 0, 0)
	m.seek.AddClass("seek")
	m.seek.OnChanged = func(v float64) {
		if !m.syncing {
			m.check(m.stream.Seek(time.Duration(v * float64(time.Second))))
		}
	}
	m.row.Clear()
	m.row.AppendAligned(m.play, false, AlignCenter)
	m.row.AppendAligned(m.seek, true, AlignCenter)
	m.row.AppendAligned(m.clock, false, AlignCenter)
	m.row.AppendAligned(m.mute, false, AlignCenter)
	m.row.AppendAligned(m.volume, false, AlignCenter)
	m.sync()
}

func (m *MediaControls) togglePlay() {
	if m.stream.Playing() {
		m.stream.Pause()
		return
	}
	m.stream.Play()
}

func (m *MediaControls) check(err error) {
	if err != nil {
		m.row.SetEnabled(false)
	}
}

func (m *MediaControls) sync() {
	s := m.stream
	live := s != nil && !s.Closed() && s.Err() == nil
	m.row.SetEnabled(live)
	if s == nil {
		m.clock.SetText("")
		m.mute.SetVisible(false)
		m.volume.SetVisible(false)
		return
	}
	m.syncing = true
	defer func() { m.syncing = false }()
	playIcon, playTip := "media-playback-start-symbolic", Tr("Play")
	if s.Playing() {
		playIcon, playTip = "media-playback-pause-symbolic", Tr("Pause")
	}
	m.playIcon.SetThemeName(playIcon)
	m.play.SetTooltip(playTip)
	m.seek.SetEnabled(live && s.Seekable() && s.Duration() > 0)
	if !m.seek.Pressed {
		m.seek.SetValue(s.Position().Seconds())
	}
	m.clock.SetText(clockText(s.Position(), s.Duration()))
	audio := s.HasAudio()
	m.mute.SetVisible(audio)
	m.volume.SetVisible(audio)
	muteIcon, muteTip := "audio-volume-high-symbolic", Tr("Mute")
	if s.Muted() {
		muteIcon, muteTip = "audio-volume-muted-symbolic", Tr("Unmute")
	}
	m.muteIcon.SetThemeName(muteIcon)
	m.mute.SetTooltip(muteTip)
	if !m.volume.Pressed {
		m.volume.SetValue(s.Volume())
	}
}

func clockText(pos, duration time.Duration) string {
	if duration <= 0 {
		return formatClock(pos, pos)
	}
	return formatClock(pos, duration) + " / " + formatClock(duration, duration)
}

func formatClock(d, scale time.Duration) string {
	secs := int64(max(d, 0) / time.Second)
	if scale >= time.Hour {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}
