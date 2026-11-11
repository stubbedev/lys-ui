// Package gtksettings reads GTK 4's settings.ini: the [Settings] group
// GTK falls back to for every setting the desktop does not publish
// through its portal. The system files (each $XDG_CONFIG_DIRS entry,
// /etc/xdg by default) load first and the user's
// ($XDG_CONFIG_HOME/gtk-4.0/settings.ini) overrides them key by key,
// as gtksettings.c loads them. GTK 4 never reads gtk-3.0's file, so
// neither does Load.
package gtksettings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/xdg"
)

// Settings are the settings.ini keys a desktop's GTK apps take their
// look and feel from. A zero field is one no file sets.
type Settings struct {
	// ThemeName is gtk-theme-name.
	ThemeName string
	// IconThemeName is gtk-icon-theme-name.
	IconThemeName string
	// CursorThemeName is gtk-cursor-theme-name.
	CursorThemeName string
	// CursorThemeSize is gtk-cursor-theme-size, in logical pixels.
	CursorThemeSize uint32
	// FontName is gtk-font-name, a Pango font description.
	FontName string
	// XftDPI is gtk-xft-dpi.
	XftDPI XftDPI
	// Animations is gtk-enable-animations.
	Animations Toggle
	// DecorationLayout is gtk-decoration-layout, the buttons of a
	// client-side titlebar ("icon:minimize,maximize,close").
	DecorationLayout string
	// XftAntialias is gtk-xft-antialias.
	XftAntialias Toggle
	// XftHinting is gtk-xft-hinting.
	XftHinting Toggle
	// XftHintStyle is gtk-xft-hintstyle.
	XftHintStyle HintStyle
	// XftRGBA is gtk-xft-rgba.
	XftRGBA SubpixelOrder
	// CursorBlink is gtk-cursor-blink.
	CursorBlink Toggle
	// CursorBlinkTime is gtk-cursor-blink-time, one whole blink cycle.
	CursorBlinkTime time.Duration
	// CursorBlinkTimeout is gtk-cursor-blink-timeout, how long the
	// caret keeps blinking after the last keypress.
	CursorBlinkTimeout time.Duration
	// DoubleClickTime is gtk-double-click-time.
	DoubleClickTime time.Duration
	// DndDragThreshold is gtk-dnd-drag-threshold, in logical pixels.
	DndDragThreshold uint32
	// EnablePrimaryPaste is gtk-enable-primary-paste.
	EnablePrimaryPaste Toggle
	// OverlayScrolling is gtk-overlay-scrolling.
	OverlayScrolling Toggle
	// ShowStatusShapes is gtk-show-status-shapes.
	ShowStatusShapes Toggle
	// SoundThemeName is gtk-sound-theme-name.
	SoundThemeName string
	// EnableEventSounds is gtk-enable-event-sounds.
	EnableEventSounds Toggle
}

// Toggle is a boolean key: unset when no file sets it, so GTK's own
// default stands.
type Toggle uint8

const (
	// ToggleUnset means no file sets the key.
	ToggleUnset Toggle = iota
	// ToggleOn is the key set true.
	ToggleOn
	// ToggleOff is the key set false.
	ToggleOff
)

// HintStyle is gtk-xft-hintstyle.
type HintStyle uint8

const (
	// HintStyleUnset means no file sets gtk-xft-hintstyle.
	HintStyleUnset HintStyle = iota
	// HintNone is hintnone.
	HintNone
	// HintSlight is hintslight.
	HintSlight
	// HintMedium is hintmedium.
	HintMedium
	// HintFull is hintfull.
	HintFull
)

// SubpixelOrder is gtk-xft-rgba, the layout of a pixel's subpixels.
type SubpixelOrder uint8

const (
	// SubpixelUnset means no file sets gtk-xft-rgba.
	SubpixelUnset SubpixelOrder = iota
	// SubpixelNone is none: grayscale antialiasing.
	SubpixelNone
	// SubpixelRGB is rgb.
	SubpixelRGB
	// SubpixelBGR is bgr.
	SubpixelBGR
	// SubpixelVRGB is vrgb.
	SubpixelVRGB
	// SubpixelVBGR is vbgr.
	SubpixelVBGR
)

var hintStyles = map[string]HintStyle{
	"hintnone": HintNone, "hintslight": HintSlight, "hintmedium": HintMedium, "hintfull": HintFull,
}

var subpixelOrders = map[string]SubpixelOrder{
	"none": SubpixelNone, "rgb": SubpixelRGB, "bgr": SubpixelBGR, "vrgb": SubpixelVRGB, "vbgr": SubpixelVBGR,
}

// XftDPI is gtk-xft-dpi: the text resolution in 1024ths of a dot per
// inch, 0 when unset (GTK's -1, "default", reads as unset).
type XftDPI int32

// DotsPerInch is the resolution in dots per inch, 0 when unset.
func (d XftDPI) DotsPerInch() float64 { return float64(d) / 1024 }

// Load reads every settings.ini GTK 4 would under env, later files
// overriding earlier ones key by key. A missing file sets nothing. A
// file that exists but cannot be read is skipped and reported in the
// error, which joins one such failure per file; the Settings returned
// alongside hold what the readable files set.
func Load(env xdg.Env) (Settings, error) {
	var s Settings
	var errs []error
	for _, path := range Paths(env) {
		data, err := os.ReadFile(path) //nolint:gosec // the XDG settings.ini locations GTK itself reads
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("gtksettings: read %s: %w", path, err))
			continue
		}
		s = s.merge(Parse(string(data)))
	}
	return s, errors.Join(errs...)
}

// Paths lists the settings.ini files Load reads, in load order: the
// system config dirs from least to most important ($XDG_CONFIG_DIRS
// lists the most important first), then the user's.
func Paths(env xdg.Env) []string {
	paths := env.ConfigSearchPath()
	slices.Reverse(paths)
	for i, dir := range paths {
		paths[i] = filepath.Join(dir, "gtk-4.0", "settings.ini")
	}
	return paths
}

// merge overlays the keys next sets.
func (s Settings) merge(next Settings) Settings {
	overlay(&s.ThemeName, next.ThemeName)
	overlay(&s.IconThemeName, next.IconThemeName)
	overlay(&s.CursorThemeName, next.CursorThemeName)
	overlay(&s.CursorThemeSize, next.CursorThemeSize)
	overlay(&s.FontName, next.FontName)
	overlay(&s.XftDPI, next.XftDPI)
	overlay(&s.Animations, next.Animations)
	overlay(&s.DecorationLayout, next.DecorationLayout)
	overlay(&s.XftAntialias, next.XftAntialias)
	overlay(&s.XftHinting, next.XftHinting)
	overlay(&s.XftHintStyle, next.XftHintStyle)
	overlay(&s.XftRGBA, next.XftRGBA)
	overlay(&s.CursorBlink, next.CursorBlink)
	overlay(&s.CursorBlinkTime, next.CursorBlinkTime)
	overlay(&s.CursorBlinkTimeout, next.CursorBlinkTimeout)
	overlay(&s.DoubleClickTime, next.DoubleClickTime)
	overlay(&s.DndDragThreshold, next.DndDragThreshold)
	overlay(&s.EnablePrimaryPaste, next.EnablePrimaryPaste)
	overlay(&s.OverlayScrolling, next.OverlayScrolling)
	overlay(&s.ShowStatusShapes, next.ShowStatusShapes)
	overlay(&s.SoundThemeName, next.SoundThemeName)
	overlay(&s.EnableEventSounds, next.EnableEventSounds)
	return s
}

func overlay[T comparable](dst *T, next T) {
	var zero T
	if next != zero {
		*dst = next
	}
}

// Parse reads the [Settings] group of one settings.ini: key = value
// lines, # and ; comments, values optionally double-quoted. Keys in
// other groups, malformed lines and malformed numbers are ignored, as
// GTK's permissive load ignores what it cannot use.
func Parse(data string) Settings {
	var s Settings
	group := ""
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			group = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || group != "Settings" {
			continue
		}
		s.set(strings.TrimSpace(key), unquote(strings.TrimSpace(value)))
	}
	return s
}

func (s *Settings) set(key, value string) {
	switch key {
	case "gtk-theme-name":
		s.ThemeName = value
	case "gtk-icon-theme-name":
		s.IconThemeName = value
	case "gtk-cursor-theme-name":
		s.CursorThemeName = value
	case "gtk-cursor-theme-size":
		if n, err := strconv.ParseUint(value, 10, 32); err == nil {
			s.CursorThemeSize = uint32(n)
		}
	case "gtk-font-name":
		s.FontName = value
	case "gtk-enable-animations":
		s.Animations = parseToggle(value)
	case "gtk-xft-dpi":
		if n, err := strconv.ParseInt(value, 10, 32); err == nil && n > 0 {
			s.XftDPI = XftDPI(n)
		}
	case "gtk-decoration-layout":
		s.DecorationLayout = value
	case "gtk-xft-antialias":
		s.XftAntialias = parseXftToggle(value)
	case "gtk-xft-hinting":
		s.XftHinting = parseXftToggle(value)
	case "gtk-xft-hintstyle":
		s.XftHintStyle = hintStyles[value]
	case "gtk-xft-rgba":
		s.XftRGBA = subpixelOrders[value]
	case "gtk-cursor-blink":
		s.CursorBlink = parseToggle(value)
	case "gtk-cursor-blink-time":
		s.CursorBlinkTime = time.Duration(parsePositive(value)) * time.Millisecond
	case "gtk-cursor-blink-timeout":
		s.CursorBlinkTimeout = time.Duration(parsePositive(value)) * time.Second
	case "gtk-double-click-time":
		s.DoubleClickTime = time.Duration(parsePositive(value)) * time.Millisecond
	case "gtk-dnd-drag-threshold":
		s.DndDragThreshold = parsePositive(value)
	case "gtk-enable-primary-paste":
		s.EnablePrimaryPaste = parseToggle(value)
	case "gtk-overlay-scrolling":
		s.OverlayScrolling = parseToggle(value)
	case "gtk-show-status-shapes":
		s.ShowStatusShapes = parseToggle(value)
	case "gtk-sound-theme-name":
		s.SoundThemeName = value
	case "gtk-enable-event-sounds":
		s.EnableEventSounds = parseToggle(value)
	}
}

func parseToggle(value string) Toggle {
	switch strings.ToLower(value) {
	case "1", "true", "yes":
		return ToggleOn
	case "0", "false", "no":
		return ToggleOff
	}
	return ToggleUnset
}

// parseXftToggle reads gtk-xft-antialias and gtk-xft-hinting, integers
// whose -1 is GTK's "default".
func parseXftToggle(value string) Toggle {
	switch value {
	case "1":
		return ToggleOn
	case "0":
		return ToggleOff
	}
	return ToggleUnset
}

// parsePositive is value as a positive int32, 0 when it is not one.
func parsePositive(value string) uint32 {
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil || n <= 0 {
		return 0
	}
	return uint32(n)
}

func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	return value
}
