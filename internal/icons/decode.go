package icons

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// currentColor is the SVG paint keyword that means "the context's
// foreground". oksvg rejects it in strict mode, so DecodeImage rewrites
// it to opaque white before parsing; recoloring goes through
// render.Icon.Tint, which keeps only the alpha.
const currentColor = "currentColor"

// DecodeImage decodes icon data named by fileName into a w x h pixel
// box. The extension picks the codec: .png, .svg, or .svgz (gzipped
// SVG); anything else sniffs the PNG magic and falls back to SVG. A
// currentColor paint decodes as white - pass the result through
// render.Icon.Tint, Cache.SymbolicIcon, or Cache.Tinted to color it.
func DecodeImage(data []byte, fileName string, w, h int) (*render.Icon, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".svgz" || ext == ".gz" {
		unzipped, err := gunzip(data)
		if err != nil {
			return nil, fmt.Errorf("icons: decode %s: %w", fileName, err)
		}
		data, ext = unzipped, ".svg"
	}
	if bytes.Contains(data, []byte(currentColor)) {
		data = bytes.ReplaceAll(data, []byte(currentColor), []byte("#ffffff"))
	}
	if ext == ".png" || (ext != ".svg" && hasPNGMagic(data)) {
		return render.LoadPNG(data, w, h)
	}
	if ext == ".jpg" || ext == ".jpeg" || hasJPEGMagic(data) {
		return render.LoadJPEG(data, w, h)
	}
	return render.LoadSVG(data, w, h)
}

// hasJPEGMagic reports whether data starts with the JPEG SOI marker.
func hasJPEGMagic(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}

// IsSymbolic reports whether an icon is symbolic and should follow the
// theme accent: its file stem carries -symbolic, or the SVG paints with
// currentColor.
func IsSymbolic(fileName string, data []byte) bool {
	stem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if strings.Contains(stem, symbolicSuffix) {
		return true
	}
	return bytes.Contains(data, []byte(currentColor))
}

// pngMagic is the first bytes of every PNG file.
var pngMagic = []byte{0x89, 'P', 'N', 'G'}

func hasPNGMagic(data []byte) bool {
	return bytes.HasPrefix(data, pngMagic)
}

func gunzip(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}
	return out, nil
}
