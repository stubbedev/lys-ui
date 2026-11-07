package highlight

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/stubbedev/lys-ui/widget"
)

// modelineLines is how many lines at each end of a text a modeline is
// looked for in, as vim's 'modelines' default.
const modelineLines = 5

var (
	// vimModeline is a vim modeline's filetype: "vim: set ft=go:",
	// "vi: filetype=python", "ex: syntax=sh".
	vimModeline = regexp.MustCompile(`(?:^|\s)(?:vim?|ex):.*?\b(?:ft|filetype|syntax)=([\w+#-]+)`)
	// emacsModeline is an Emacs mode line: "-*- mode: python -*-" or
	// "-*- python -*-".
	emacsModeline = regexp.MustCompile(`-\*-\s*(?:.*?mode:\s*([\w+#-]+)|([\w+#-]+))\s*(?:;.*?)?-\*-`)
)

// Detect finds text's highlighter the way bat does: by the file's name
// (name may be "" for text from no file), then the interpreter its #!
// line names, then what its first line gives away ("package main",
// "<?xml"), then a vim or Emacs modeline in its first or last lines.
func Detect(name, text string) (widget.Highlighter, bool) {
	if name != "" {
		if h, ok := ForFile(name); ok {
			return h, true
		}
	}
	first, _, _ := strings.Cut(text, "\n")
	if interp, ok := shebang(first); ok {
		if h, ok := forInterpreter(interp); ok {
			return h, true
		}
	}
	if h, ok := forFirstLine(first); ok {
		return h, true
	}
	return forModeline(text)
}

// shebang is the program a #! line runs, through env when it names env
// ("#!/usr/bin/env -S python3 -u" runs python3).
func shebang(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return "", false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	prog := filepath.Base(fields[0])
	if prog != "env" {
		return prog, true
	}
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") && !strings.Contains(f, "=") {
			return filepath.Base(f), true
		}
	}
	return "", false
}

// forInterpreter finds the language interp runs, a version suffix
// ("python3.12", "lua5.4") ignored.
func forInterpreter(interp string) (widget.Highlighter, bool) {
	bare := strings.TrimRight(interp, "0123456789.")
	registry.RLock()
	defer registry.RUnlock()
	for _, l := range registry.langs {
		if slices.Contains(l.Interpreters, interp) || slices.Contains(l.Interpreters, bare) {
			return l.Highlighter, true
		}
	}
	return nil, false
}

// forFirstLine finds the language whose first-line pattern line matches.
func forFirstLine(line string) (widget.Highlighter, bool) {
	registry.RLock()
	defer registry.RUnlock()
	for _, l := range registry.langs {
		if l.FirstLine != nil && l.FirstLine.MatchString(line) {
			return l.Highlighter, true
		}
	}
	return nil, false
}

// forModeline finds the language a modeline in text's first or last
// modelineLines lines names.
func forModeline(text string) (widget.Highlighter, bool) {
	lines := strings.SplitN(text, "\n", modelineLines+1)
	head := lines[:min(len(lines), modelineLines)]
	for _, line := range head {
		if h, ok := modeline(line); ok {
			return h, true
		}
	}
	if len(lines) <= modelineLines {
		return nil, false
	}
	tail := strings.TrimRight(text, "\n")
	for range modelineLines {
		at := strings.LastIndexByte(tail, '\n')
		if h, ok := modeline(tail[at+1:]); ok {
			return h, true
		}
		if at < 0 {
			break
		}
		tail = tail[:at]
	}
	return nil, false
}

// modeline is the language line's modeline names.
func modeline(line string) (widget.Highlighter, bool) {
	if m := vimModeline.FindStringSubmatch(line); m != nil {
		return Lookup(m[1])
	}
	if m := emacsModeline.FindStringSubmatch(line); m != nil {
		if m[1] != "" {
			return Lookup(m[1])
		}
		return Lookup(m[2])
	}
	return nil, false
}
