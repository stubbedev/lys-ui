package app

import "github.com/stubbedev/lys-ui/widget"

func (a *Application) PasteInto(router *widget.Router) { a.keyClip.paste(router) }
