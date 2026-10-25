package widget

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stubbedev/lys-ui/gpu"
	"github.com/stubbedev/lys-ui/internal/gputest"
	"github.com/stubbedev/lys-ui/render"
)

// goldenDevice is the GPU the goldens also render on when
// LYS_UI_TEST_GPU asks for it, opened once for the run.
var goldenDevice = sync.OnceValues(func() (*gpu.Device, error) {
	icd, _, err := gputest.Driver()
	if err != nil {
		return nil, err
	}
	if icd != "" {
		if err := os.Setenv("VK_DRIVER_FILES", icd); err != nil {
			return nil, fmt.Errorf("setting VK_DRIVER_FILES: %w", err)
		}
	}
	return gpu.Open()
})

// checkGoldenGPU renders the recorded golden on the GPU and fails on
// any pixel differing from want, the CPU's: the GPU's shaders run the
// CPU's integer and float32 arithmetic step for step.
func checkGoldenGPU(t *testing.T, name string, l *render.List, want []byte, w, h int) {
	t.Helper()
	d, err := goldenDevice()
	if err != nil {
		t.Fatalf("%s: GPU for LYS_UI_TEST_GPU=%s: %v", name, os.Getenv("LYS_UI_TEST_GPU"), err)
	}
	tg, err := d.NewOffscreen(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	if err := d.Render(l, tg); err != nil {
		t.Fatalf("%s: rendering on the GPU: %v", name, err)
	}
	got, err := tg.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	stride := render.Stride(w)
	bad := 0
	var first string
	for y := range h {
		for x := range w {
			c := render.ColorFromBytes(want[y*stride+x*4:])
			g := got.RGBAAt(x, y)
			cpu := [4]int{int(c.R()), int(c.G()), int(c.B()), int(c.A())}
			gp := [4]int{int(g.R), int(g.G), int(g.B), int(g.A)}
			for i := range cpu {
				if cpu[i] != gp[i] {
					if bad == 0 {
						first = fmt.Sprintf("(%d, %d): gpu %v cpu %v", x, y, gp, cpu)
					}
					bad++
					break
				}
			}
		}
	}
	if bad > 0 {
		t.Errorf("%s: the GPU's pixels differ from the CPU's at %d pixels, first %s", name, bad, first)
	}
}
