package app

import "github.com/stubbedev/lys-ui/widget"

// PasteInto is the ctrl+v path into router's focused widget.
func (a *Application) PasteInto(router *widget.Router) { a.keyClip.paste(router) }
