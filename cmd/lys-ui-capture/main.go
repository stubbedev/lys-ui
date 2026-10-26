// Command lys-ui-capture is the capture package's consumer: it lists
// the compositor's outputs and toplevels, screenshots any of them to
// a PNG (whole outputs through wlr-screencopy or
// ext-image-copy-capture, single windows through the foreign
// toplevel capture), and follows one output continuously with the
// damage-driven Stream, writing a numbered PNG per changed frame.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/capture"
)

func main() {
	list := flag.Bool("list", false, "list outputs and toplevels, then exit")
	output := flag.String("output", "", "capture this output by connector name")
	window := flag.String("window", "", "capture the toplevel whose title or app id contains this")
	monitor := flag.Bool("monitor", false, "follow the output continuously, writing a PNG per changed frame")
	cursor := flag.Bool("cursor", false, "composite the pointer into the frames")
	frames := flag.Int("n", 0, "stop after this many monitored frames (0: until interrupted)")
	out := flag.String("out", "", "output PNG file (screenshots) or pattern like frame%03d.png (monitor)")
	flag.Parse()
	if err := run(*list, *output, *window, *monitor, *cursor, *frames, *out); err != nil {
		log.Fatal(err)
	}
}

func run(list bool, outputName, window string, monitor, cursor bool, frames int, out string) error {
	client, err := capture.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err := client.Refresh(); err != nil {
		return err
	}
	switch {
	case list:
		return listAll(client)
	case monitor:
		if outputName == "" {
			return errors.New("lys-ui-capture: -monitor needs -output")
		}
		if out == "" {
			out = "frame%03d.png"
		}
		return monitorOutput(outputName, cursor, frames, out)
	case outputName != "":
		if out == "" {
			return errors.New("lys-ui-capture: -output needs -out")
		}
		return shootOutput(client, outputName, cursor, out)
	case window != "":
		if out == "" {
			return errors.New("lys-ui-capture: -window needs -out")
		}
		return shootWindow(client, window, cursor, out)
	}
	flag.Usage()
	return nil
}

func listAll(client *capture.Client) error {
	for _, o := range client.Outputs() {
		fmt.Printf("output %s %dx%d %s\n", o.Name, o.Width, o.Height, o.Description)
	}
	if !client.HasToplevelCapture() {
		return nil
	}
	toplevels, err := client.Toplevels()
	if err != nil {
		return err
	}
	for _, t := range toplevels {
		fmt.Printf("window %q app %q id %s\n", t.Title, t.AppID, t.Identifier)
	}
	return nil
}

func shootOutput(client *capture.Client, name string, cursor bool, path string) error {
	out, ok := client.OutputByName(name)
	if !ok {
		return fmt.Errorf("lys-ui-capture: no output %q", name)
	}
	frame, err := client.CaptureOutputOnce(out, capture.Options{Cursor: cursor})
	if err != nil {
		return err
	}
	return writePNG(frame, path)
}

func shootWindow(client *capture.Client, needle string, cursor bool, path string) error {
	if !client.HasToplevelCapture() {
		return capture.ErrUnsupported
	}
	toplevels, err := client.Toplevels()
	if err != nil {
		return err
	}
	for _, t := range toplevels {
		if strings.Contains(t.Title, needle) || strings.Contains(t.AppID, needle) {
			frame, err := client.CaptureToplevel(t, capture.Options{Cursor: cursor})
			if err != nil {
				return err
			}
			return writePNG(frame, path)
		}
	}
	return fmt.Errorf("lys-ui-capture: no window matches %q", needle)
}

func monitorOutput(name string, cursor bool, frames int, pattern string) error {
	stream, err := capture.OpenOutputStream(name, cursor)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	info := stream.Info()
	log.Printf("lys-ui-capture: streaming %dx%d", info.Width, info.Height)
	n := 0
	var last *capture.StreamFrame
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stream.Done():
			return stream.Err()
		case <-tick.C:
			latest := stream.Latest()
			if latest == nil || latest == last {
				continue
			}
			last = latest
			frame, err := latest.Frame()
			if err != nil {
				return err
			}
			if err := writePNG(frame, fmt.Sprintf(pattern, n)); err != nil {
				return err
			}
			n++
			if frames > 0 && n >= frames {
				return nil
			}
		}
	}
}

func writePNG(frame *capture.Frame, path string) error {
	img, err := frame.Image()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	f, err := os.Create(path) //nolint:gosec // path is the user's -out argument
	if err != nil {
		return err
	}
	defer f.Close()
	log.Printf("lys-ui-capture: %s %dx%d", path, frame.Width, frame.Height)
	return png.Encode(f, img)
}
