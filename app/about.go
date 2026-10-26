package app

import (
	"image"
	"strings"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

// AboutConfig declares an about dialog, the AdwAboutWindow content:
// who the app is, where it lives, who made it, and under what terms.
// Zero fields drop their sections - an about with only a name and
// version is a title card, one with credits and license is the full
// GNOME-class page.
type AboutConfig struct {
	// Name and Version head the dialog; Comments is the one-line
	// tagline under them.
	Name     string
	Version  string
	Comments string
	// Logo paints above the name (image.Image; nil skips it). LogoPath
	// loads asynchronously in its place, the widget.Image file source.
	Logo     image.Image
	LogoPath string
	// Website and IssueURL render as their URLs; opening them is the
	// launcher ticket's (#86) job once it lands.
	Website  string
	IssueURL string
	// ReleaseNotes shows under a "What's New" heading; License under
	// "Legal", both scrolled with the rest.
	ReleaseNotes string
	License      string
	// Credits group contributors by role ("Authors", "Artists", ...),
	// in order, each group a heading and its names.
	Credits []AboutCredit
}

// AboutCredit is one contributor group.
type AboutCredit struct {
	Role  string
	Names []string
}

// aboutSection is one display block of the about page.
type aboutSection struct {
	Heading string
	Lines   []string
}

// aboutSections flattens the config into display blocks in about-page
// order, dropping empty ones: release notes, then credit groups, then
// the license. The identity block (name, version, links) is separate;
// every section here lands inside the scrolled area.
func aboutSections(cfg AboutConfig) []aboutSection {
	var sections []aboutSection
	if cfg.ReleaseNotes != "" {
		sections = append(sections, aboutSection{Heading: widget.Tr("What's New"), Lines: strings.Split(cfg.ReleaseNotes, "\n")})
	}
	for _, c := range cfg.Credits {
		if len(c.Names) == 0 {
			continue
		}
		sections = append(sections, aboutSection{Heading: c.Role, Lines: c.Names})
	}
	if cfg.License != "" {
		sections = append(sections, aboutSection{Heading: widget.Tr("Legal"), Lines: strings.Split(cfg.License, "\n")})
	}
	return sections
}

// AboutDialog opens the application's about page: identity at the top,
// release notes, credits, and license scrolled beneath, one Close
// button, Escape closes. It rides the standard Dialog machinery like
// every other dialog.
func (a *Application) AboutDialog(parent *Window, cfg AboutConfig) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	head := widget.NewBox(widget.Column, 4, 0)
	if cfg.Logo != nil {
		logo := widget.NewImage(cfg.Logo)
		head.Append(widget.NewBox(widget.Row, 0, 8).Append(logo, false), false)
	} else if cfg.LogoPath != "" {
		logo := widget.NewFileImage(cfg.LogoPath)
		head.Append(widget.NewBox(widget.Row, 0, 8).Append(logo, false), false)
	}
	if cfg.Name != "" {
		head.Append(widget.NewLabel(face, 20, cfg.Name, 0), false)
	}
	if cfg.Version != "" {
		head.Append(aboutLabel(face, 13, widget.Tr("Version")+" "+cfg.Version, css.DimLabel), false)
	}
	if cfg.Comments != "" {
		head.Append(aboutLabel(face, 13, cfg.Comments, css.DimLabel), false)
	}
	for _, link := range []string{cfg.Website, cfg.IssueURL} {
		if link != "" {
			head.Append(aboutLabel(face, 13, link, css.Accent), false)
		}
	}

	body := widget.NewBox(widget.Column, 14, 0)
	for _, s := range aboutSections(cfg) {
		body.Append(aboutLabel(face, 12, strings.ToUpper(s.Heading), css.DimLabel), false)
		for _, line := range s.Lines {
			body.Append(widget.NewLabel(face, 13, line, 0), false)
		}
	}

	content := widget.NewBox(widget.Column, 16, 16)
	content.Append(head, false)
	if len(body.Children()) > 0 {
		scroll := widget.NewScroll(body)
		scroll.SetMaxContentHeight(220)
		content.Append(scroll, true)
	}
	return a.NewDialog(parent, DialogConfig{
		Title: widget.Tr("About") + " " + cfg.Name,
		Width: 380, Height: 320,
		Content:         content,
		Buttons:         []DialogButton{{Label: widget.Tr("Close"), Response: "close"}},
		DefaultResponse: "close",
		CancelResponse:  "close",
	})
}

// aboutLabel is a line of the about dialog in the class's color.
func aboutLabel(face *render.Typeface, sizePx float64, text, class string) *widget.Label {
	l := widget.NewLabel(face, sizePx, text, 0)
	l.AddClass(class)
	return l
}
