// The harness clicker: pointer clicks, wheel, keys, and text on
// $WAYLAND_DISPLAY through the headless suite's virtual pointer — the
// manual companion to internal/headlesstest's in-process driving,
// used to walk a live surface (the settings app on a nested sway)
// while its trace log runs.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/stubbedev/lys-lib/intconv"

	"github.com/stubbedev/lys-ui/internal/headlesstest"
)

const usage = "usage: zz-vpclick X Y [click | dclick | scroll DY | key CODE | type TEXT]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	act, err := parse(args)
	if err != nil {
		return err
	}
	in, err := headlesstest.Dial(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		return err
	}
	return act(in)
}

// parse validates the whole command line before anything reaches the
// compositor, returning the input to drive.
func parse(args []string) (func(*headlesstest.VirtualInput) error, error) {
	if len(args) < 2 {
		return nil, errors.New(usage)
	}
	x, err := coord(args[0])
	if err != nil {
		return nil, err
	}
	y, err := coord(args[1])
	if err != nil {
		return nil, err
	}
	action := "click"
	if len(args) > 2 {
		action = args[2]
	}
	operand := func() (string, error) {
		if len(args) != 4 {
			return "", fmt.Errorf("%s needs one operand; %s", action, usage)
		}
		return args[3], nil
	}
	switch action {
	case "click", "dclick":
		if len(args) > 3 {
			return nil, errors.New(usage)
		}
		clicks := 1
		if action == "dclick" {
			clicks = 2
		}
		return func(in *headlesstest.VirtualInput) error {
			for range clicks {
				if err := in.ClickAt(x, y, headlesstest.BTNLeft); err != nil {
					return err
				}
			}
			return nil
		}, nil
	case "scroll":
		op, err := operand()
		if err != nil {
			return nil, err
		}
		dy, err := strconv.ParseFloat(op, 64)
		if err != nil {
			return nil, fmt.Errorf("scroll delta %q: %w", op, err)
		}
		return func(in *headlesstest.VirtualInput) error { return in.ScrollAt(x, y, dy) }, nil
	case "key":
		op, err := operand()
		if err != nil {
			return nil, err
		}
		// evdev key codes fit in 16 bits (KEY_MAX is 0x2ff).
		code, err := strconv.ParseUint(op, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("key code %q: %w", op, err)
		}
		return func(in *headlesstest.VirtualInput) error { return in.Tap(intconv.Clamp[uint32](code)) }, nil
	case "type":
		text, err := operand()
		if err != nil {
			return nil, err
		}
		return func(in *headlesstest.VirtualInput) error { return in.TypeText(text) }, nil
	}
	return nil, fmt.Errorf("unknown action %q; %s", action, usage)
}

// coord parses one non-negative compositor coordinate.
func coord(s string) (int, error) {
	v, err := strconv.ParseUint(s, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("coordinate %q: %w", s, err)
	}
	return int(v), nil
}
