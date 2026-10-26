// GPU presentation: wl_subcompositor places an application-rendered
// surface inside a window (widget.GPUArea), and zwp_linux_dmabuf_v1
// turns the application's dmabufs into wl_buffers on this connection.
// Both globals are optional; without them GPUArea reports why.
package wlsession

import (
	"slices"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/wlr"
)

const maxDmabufVersion = 3

// DmabufLayout is a format and modifier pair the compositor imports.
type DmabufLayout struct {
	Format   uint32
	Modifier uint64
}

type dmabufFormats struct {
	s       *Session
	version uint32
}

func (f dmabufFormats) HandleZwpDmabufV1Format(ev wlr.ZwpDmabufV1FormatEvent) {
	if f.version < 3 {
		f.s.dmabufLayouts[DmabufLayout{Format: ev.Format, Modifier: dmabuf.ModifierInvalid}] = true
	}
}

func (f dmabufFormats) HandleZwpDmabufV1Modifier(ev wlr.ZwpDmabufV1ModifierEvent) {
	f.s.dmabufLayouts[DmabufLayout{Format: ev.Format, Modifier: uint64(ev.ModifierHi)<<32 | uint64(ev.ModifierLo)}] = true
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
	s.dmabuf = mgr
	debug.Log("shell", "linux-dmabuf v%d bound", version)
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
// imports format in (dmabuf.ModifierInvalid, an implicit layout, left
// out).
func (s *Session) DmabufModifiers(format uint32) []uint64 {
	var mods []uint64
	for l := range s.dmabufLayouts {
		if l.Format == format && l.Modifier != dmabuf.ModifierInvalid {
			mods = append(mods, l.Modifier)
		}
	}
	slices.Sort(mods)
	return mods
}
