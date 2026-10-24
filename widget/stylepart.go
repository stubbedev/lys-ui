package widget

import "github.com/stubbedev/lys-ui/render"

// stylePart is a style-only node of a widget: matched and painted by
// its owning widget, never laid out or hit on its own. Every sub-node
// a GTK stylesheet names embeds it — the entry's `text`, the scale's
// `trough`, the switch's `slider`, the checkbutton's `check` — so one
// type carries the tree shape and the cascade resolves per part.
type stylePart struct{ node }

func (*stylePart) Measure(con Constraints) Size { return clampSize(Size{}, con) }
func (*stylePart) Paint(*render.Canvas)         {}
func (*stylePart) HitTest(Point) Widget         { return nil }

// styleParter is a style part: a node of its owner's own tree, which
// restyles with the owner rather than as a child of its own.
type styleParter interface{ partNode() *node }

func (p *stylePart) partNode() *node { return &p.node }
