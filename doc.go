// Package lysui is a pure-Go (no cgo) widget toolkit for Wayland and a
// counterpart of relm4: a component framework with typed messages, a
// retained widget tree with GTK4/libadwaita-class widgets, xdg-shell
// windows and layer-shell surfaces, a CSS engine over a typed palette,
// the desktop's portals and protocols, and one event loop that idles
// at 0% CPU.
//
// Applications import the public packages:
//
//	component  the component framework: components, controllers,
//	           factories, workers, commands (headless tests in
//	           component/componenttest)
//	ui         typed view builders, one per widget
//	app        the session, windows and layer surfaces, the loop,
//	           dialogs, popovers, desktop services
//	widget     the widget tree, theme, CSS, actions, bindings
//	           (libadwaita style classes in widget/css)
//	render     the premultiplied canvas, shaped text, icons
//	anim       tweens, easing, springs and timelines
//	appearance the desktop's color scheme, accent and contrast
//	i18n       gettext catalogs
//	highlight  syntax highlighters for the code view
//	media      video and sound playback
//	media/mux  MP4 and Matroska muxing of H.264, H.265 and Opus
//	pdf        PDF writing
//	transfer   clipboard and drag-and-drop content
//	dmabuf     dma-buf frame descriptions
//	capture    screen and window capture
//	vinput     virtual pointer and keyboard injection
//	wlr        generated Wayland protocol bindings
//
// Everything under internal/ is implementation. The docs/ directory
// holds the reference: architecture, components, the application,
// input and threading models, CSS, appearance, icons, accessibility,
// the inspector, and the capability map in docs/completeness.md.
package lysui
