// The one shm pixel format lys-ui submits: the constant every buffer
// the session arena hands out is created with, and the diagnostics
// line the doctor block reports. It lives here so the doctor quotes
// the wire truth instead of restating it.
package buffer

import (
	"strconv"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// FormatName describes the shm pixel format every lys-ui buffer uses:
// ARGB8888 premultiplied, byte order B, G, R, A - the format
// wl_shm.format advertises as wl.ShmFormatArgb8888 and the only one
// the session requires (see wlsession.Connect).
var FormatName = "ARGB8888 premultiplied (wl_shm format 0x" +
	strconv.FormatUint(uint64(wl.ShmFormatArgb8888), 16) + ")"
