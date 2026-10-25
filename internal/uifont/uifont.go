// Package uifont embeds the faces lys-ui's default look is drawn in:
// Geist and Geist Mono, as
// variable fonts so every weight is one file. internal/sysfont
// registers them in the font store, so they resolve by family name
// like any installed font. README.md records the source and checksums;
// OFL.txt is their license, which travels with them.
package uifont

import _ "embed"

// Family and MonoFamily are the embedded faces' family names.
const (
	Family     = "Geist"
	MonoFamily = "Geist Mono"
)

// Sans is Geist's variable font file (wght 100-900).
//
//go:embed Geist-Variable.ttf
var Sans []byte

// Mono is Geist Mono's variable font file (wght 100-900).
//
//go:embed GeistMono-Variable.ttf
var Mono []byte
