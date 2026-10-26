package app

import (
	"errors"
	"image"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// The eyedropper half of #84: ColorChooserDialog wires a
// pick-from-screen button when the compositor offers wlr-screencopy
// (checked once per application, on the capture package's private
// connection). The click captures the first output off the loop, then
// opens a dialog showing the frame; a click on it sets the chooser's
// color. Without screencopy the button never appears - the chooser
// degrades to swatches-only.

// eyedropperAvailable reports and caches whether a screen capture is
// possible; the connect and refresh round-trips run once.
func (a *Application) eyedropperAvailable() bool {
	if a.eyedropChecked {
		return a.eyedropOK
	}
	a.eyedropChecked = true
	client, err := capture.Connect()
	if err != nil {
		debug.Log("shell", "eyedropper: capture connect: %v", err)
		return false
	}
	defer func() { _ = client.Close() }()
	if err := client.Refresh(); err != nil {
		debug.Log("shell", "eyedropper: capture refresh: %v", err)
		return false
	}
	a.eyedropOK = client.HasScreencopy() && len(client.Outputs()) > 0
	return a.eyedropOK
}

// startEyedrop captures the first output off the loop and opens the
// pick dialog on it, setting the chooser's color on the picked pixel.
func (a *Application) startEyedrop(chooser *widget.ColorChooser) {
	go func() {
		var img image.Image
		err := func() error {
			client, err := capture.Connect()
			if err != nil {
				return err
			}
			defer func() { _ = client.Close() }()
			if err := client.Refresh(); err != nil {
				return err
			}
			outputs := client.Outputs()
			if len(outputs) == 0 {
				return errors.New("no output to capture")
			}
			frame, err := client.CaptureOutputOnce(outputs[0], capture.Options{})
			if err != nil {
				return err
			}
			img, err = frame.Image()
			return err
		}()
		a.Invoke(func() {
			if err != nil {
				debug.Log("shell", "eyedropper: capture: %v", err)
				_, _ = a.MessageBox(nil, widget.Tr("Pick from screen"), widget.Tr("The screen could not be captured on this compositor."), []DialogButton{{Label: widget.Tr("Close"), Response: "close", Role: ButtonRoleCancel}})
				return
			}
			a.openEyedrop(chooser, img)
		})
	}()
}

// openEyedrop shows the captured frame and applies the clicked pixel;
// the picked color lands in the chooser and the dialog closes.
func (a *Application) openEyedrop(chooser *widget.ColorChooser, img image.Image) {
	pick := widget.NewScreenPick(img)
	var d *Dialog
	pick.OnPick = func(col render.Color) {
		chooser.SetColor(col)
		chooser.Invalidate()
		if d != nil {
			d.Respond("cancel")
		}
	}
	d, _ = a.NewDialog(nil, DialogConfig{
		Title:          widget.Tr("Pick from screen"),
		Width:          520,
		Height:         400,
		Content:        pick,
		Modal:          true,
		Buttons:        []DialogButton{{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel}},
		CancelResponse: "cancel",
	})
}
