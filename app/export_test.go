package app

import (
	"errors"
	"io"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/media/demux"

	"github.com/stubbedev/lys-ui/media"
)

// OpenDialogs returns the dialogs a test cannot reach through the API
// that opened them, oldest first.
func (a *Application) OpenDialogs() []*Dialog { return a.dialogs }

// SetVideoMemory makes a's Video surfaces take their buffers from alloc
// instead of /dev/udmabuf: an fd the compositor imports, its mapping,
// and how to free both. Writes through the mapping need no bracket.
func (a *Application) SetVideoMemory(alloc func(size int) (fd int, mem []byte, free func() error, err error)) {
	a.videoMemory = func(size int) (videoMemory, error) {
		fd, mem, free, err := alloc(size)
		if err != nil {
			return nil, err
		}
		return plainMemory{fdv: fd, mem: mem, free: free}, nil
	}
}

type plainMemory struct {
	fdv  int
	mem  []byte
	free func() error
}

func (m plainMemory) fd() int           { return m.fdv }
func (m plainMemory) bytes() []byte     { return m.mem }
func (m plainMemory) beginWrite() error { return nil }
func (m plainMemory) endWrite() error   { return nil }
func (m plainMemory) close() error      { return m.free() }

// GPUDevice is the device a's windows draw on, nil while they draw on
// the CPU.
func (a *Application) GPUDevice() *gpu.Device {
	if a.gpu == nil {
		return nil
	}
	return a.gpu.dev
}

// FaultGPU has a's renderer take err as its frame's failure.
func (a *Application) FaultGPU(err error) { a.gpu.fault(err) }

// HoldGPUOpen has a answer as it does while its GPU device opens, until
// the returned release: no device came of it, and the Videos waiting are
// woken.
func (a *Application) HoldGPUOpen() (release func()) {
	a.gpuTried, a.gpuPending = true, true
	return func() {
		a.gpuPending = false
		a.wakeGPUWaiting()
	}
}

// GPUWaiting is how many Videos wait for a's GPU device.
func (a *Application) GPUWaiting() int { return len(a.gpuWaiting) }

// UndecodedTextures is a media.TextureSource no device decodes: what a
// Video shows before it has a device, or with none.
type UndecodedTextures struct{}

// Info is a 32x16 video a second long.
func (UndecodedTextures) Info() media.Info {
	return media.Info{Width: 32, Height: 16, Duration: time.Second}
}

// Close closes nothing.
func (UndecodedTextures) Close() error { return nil }

// Seek moves nothing.
func (UndecodedTextures) Seek(time.Duration) error { return nil }

// ReadPacket has no packets.
func (UndecodedTextures) ReadPacket() (demux.Packet, error) { return demux.Packet{}, io.EOF }

// NewVideoDecoder refuses every device.
func (UndecodedTextures) NewVideoDecoder(*gpu.Device) (media.VideoDecoder, error) {
	return nil, errors.New("app test: no device decodes this")
}
