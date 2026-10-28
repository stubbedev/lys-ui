// GPU presentation: wl_subcompositor places an application-rendered
// surface inside a window (widget.GPUArea), and zwp_linux_dmabuf_v1
// turns the application's dmabufs into wl_buffers on this connection.
// Both globals are optional; without them GPUArea reports why.
package wlsession

import (
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxDmabufVersion is 4, the first with feedback: the compositor names
// the device its buffers must be importable on, and lists the layouts
// there instead of the format and modifier events.
const maxDmabufVersion = 4

// DmabufLayout is a format and modifier pair the compositor imports.
type DmabufLayout struct {
	Format   fourcc.Format
	Modifier fourcc.Modifier
}

type dmabufFormats struct {
	s       *Session
	version uint32
}

func (f dmabufFormats) HandleZwpDmabufV1Format(ev wlr.ZwpDmabufV1FormatEvent) {
	if f.version < 3 {
		f.s.dmabufLayouts[DmabufLayout{Format: fourcc.Format(ev.Format), Modifier: fourcc.Invalid}] = true
	}
}

func (f dmabufFormats) HandleZwpDmabufV1Modifier(ev wlr.ZwpDmabufV1ModifierEvent) {
	f.s.dmabufLayouts[DmabufLayout{Format: fourcc.Format(ev.Format), Modifier: fourcc.Modifier(uint64(ev.ModifierHi)<<32 | uint64(ev.ModifierLo))}] = true
}

func (s *Session) bindSubcompositor(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	sub := wl.NewSubcompositor(ctx)
	if !s.bindOptional(ev, 1, sub) {
		return
	}
	s.subcompositor = sub
	debug.Log("shell", "wl_subcompositor bound")
}

func (s *Session) bindDmabuf(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpDmabufV1(ctx)
	version := bindVersion(ev.Version, maxDmabufVersion)
	if version < 2 {
		return
	}
	s.dmabufLayouts = map[DmabufLayout]bool{}
	formats := dmabufFormats{s: s, version: version}
	mgr.AddFormatHandler(formats)
	mgr.AddModifierHandler(formats)
	if !s.bindOptional(ev, maxDmabufVersion, mgr) {
		return
	}
	s.dmabuf, s.dmabufVersion = mgr, version
	debug.Log("shell", "linux-dmabuf v%d bound", version)
}

// readDmabufFeedback reads a v4 linux-dmabuf's default feedback into
// the session's layouts and main device; an earlier version sent its
// layouts as events already and names no device.
func (s *Session) readDmabufFeedback() error {
	if s.dmabuf == nil || s.dmabufVersion < 4 {
		return nil
	}
	fb, err := dmabufwl.DefaultFeedback(s.dmabuf, s.dmabufVersion, s.Roundtrip)
	if err != nil {
		return err
	}
	s.applyDmabufFeedback(fb)
	return nil
}

// applyDmabufFeedback takes the feedback's main device and layouts.
func (s *Session) applyDmabufFeedback(fb dmabufwl.Feedback) {
	s.dmabufDevice, s.hasDmabufDevice = fb.Device, true
	for format, mods := range fb.Modifiers {
		for _, m := range mods {
			s.dmabufLayouts[DmabufLayout{Format: format, Modifier: m}] = true
		}
	}
}

// DmabufDevice is the compositor's main device, the dev_t of the DRM
// node whose memory it imports buffers from; ok is false where the
// compositor names none (linux-dmabuf before v4).
func (s *Session) DmabufDevice() (dev uint64, ok bool) {
	return s.dmabufDevice, s.hasDmabufDevice
}

// Subcompositor returns the bound wl_subcompositor, or nil.
func (s *Session) Subcompositor() *wl.Subcompositor { return s.subcompositor }

// Dmabuf returns the bound zwp_linux_dmabuf_v1 (version 2 or later),
// or nil.
func (s *Session) Dmabuf() *wlr.ZwpDmabufV1 { return s.dmabuf }

// DmabufImports reports whether the compositor advertised the layout;
// importing any other is a fatal protocol error.
func (s *Session) DmabufImports(l DmabufLayout) bool { return s.dmabufLayouts[l] }

// DmabufModifiers lists, sorted, the explicit modifiers the compositor
// imports format in (fourcc.Invalid, an implicit layout, left
// out).
func (s *Session) DmabufModifiers(format fourcc.Format) []fourcc.Modifier {
	var mods []fourcc.Modifier
	for l := range s.dmabufLayouts {
		if l.Format == format && l.Modifier != fourcc.Invalid {
			mods = append(mods, l.Modifier)
		}
	}
	slices.Sort(mods)
	return mods
}
