package widget

import (
	"bytes"
	"image"
	"os"
	"sync"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/icons"
)

// bgImages caches background-image files decoded at their natural
// size: a paint-time lookup is a map hit, and a changed file re-reads
// through the icon cache's generation like any theme asset.
var bgImages sync.Map // path -> *render.Icon

// bgImageFor decodes path once and caches it; a failed read returns
// nil, and the box paints its background color alone.
func bgImageFor(path string) *render.Icon {
	if path == "" {
		return nil
	}
	if ic, ok := bgImages.Load(path); ok {
		typed, _ := ic.(*render.Icon)
		return typed
	}
	// #nosec G304 -- the path is the stylesheet's own background-image declaration.
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		bgImages.Store(path, (*render.Icon)(nil))
		return nil
	}
	w, h := 256, 256
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil && cfg.Width > 0 {
		w, h = min(cfg.Width, 2048), min(cfg.Height, 2048)
	}
	ic, err := icons.DecodeImage(data, path, w, h)
	if err != nil {
		ic = nil
	}
	bgImages.Store(path, ic)
	return ic
}
