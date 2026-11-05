package app

import (
	"context"
	"time"

	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/sysfont"
)

const fontWarmStopTimeout = 3 * time.Second

var warmFonts = sysfont.Warm

func startFontWarm(warm func(context.Context)) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	warmed := make(chan struct{})
	go func() {
		defer close(warmed)
		warm(ctx)
	}()
	return func() {
		cancel()
		select {
		case <-warmed:
		case <-time.After(fontWarmStopTimeout):
			debug.Log("config", "the font warm-up outlived the stop")
		}
	}
}
