// Command lys-ui-gallery is the component gallery: every component
// lys-ui implements, one titled section each, declared with the typed
// ui builders in the dark and light palettes. Ctrl+K
// opens the command palette as a dialog, Ctrl+T or the header button
// toggles the palette, Escape closes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

func main() {
	shotsDir := flag.String("shots", "", "write a PNG of every demo, in both palettes, to this directory and exit")
	flag.Parse()
	cmd := run
	if *shotsDir != "" {
		cmd = func() error { return runShots(*shotsDir) }
	}
	switch err := cmd(); {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
		os.Exit(app.DisconnectExitCode)
	default:
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	application := app.NewApplication(sess)
	widget.LoadStylesheet(galleryCSS)
	var ctrl *component.Controller[galleryMsg, struct{}]
	gal := &gallery{env: ui.Env{Face: app.FontFallback(face), Size: 14}, app: application}
	var win *app.Window
	ctrl, win, err = component.Window(application, app.WindowConfig{
		Title:      "lys-ui gallery",
		AppID:      "dev.stubbe.lys.ui.gallery",
		Width:      760,
		Height:     820,
		Background: widget.Current().Bg,
	}, gal)
	if err != nil {
		return err
	}
	gal.win = win
	for keys, action := range map[string]widget.Activatable{
		"Escape": widget.NewAction("quit", application.Quit),
		"Ctrl+T": widget.NewAction("palette", func() { ctrl.Send(togglePalette{}) }),
		"Ctrl+B": widget.NewAction("sidebar", func() { gal.sidebar.Toggle() }),
		"Ctrl+K": widget.NewAction("command", func() {
			if err := openCommandDialog(application, win, app.FontFallback(face)); err != nil {
				log.Printf("lys-ui-gallery: command dialog: %v", err)
			}
		}),
	} {
		if err := application.AddAccel(keys, action); err != nil {
			return err
		}
	}
	return application.Run()
}

// commandGroups are the command palette's items; pick runs after any
// of them is picked.
func commandGroups(pick func()) []widget.CommandGroup {
	item := func(label, shortcut string, keywords ...string) widget.CommandItem {
		return widget.CommandItem{Label: label, Shortcut: shortcut, Keywords: keywords, OnSelect: pick}
	}
	return []widget.CommandGroup{
		{Heading: "Suggestions", Items: []widget.CommandItem{
			item("Calendar", ""), item("Search Emoji", "", "smiley"),
			{Label: "Calculator", Disabled: true},
		}},
		{Heading: "Settings", Items: []widget.CommandItem{
			item("Profile", "Ctrl P"), item("Billing", "Ctrl B"), item("Settings", "Ctrl S", "preferences"),
		}},
	}
}

// openCommandDialog shows the command palette in a dialog over parent
// that closes when an item is picked.
func openCommandDialog(a *app.Application, parent *app.Window, face render.Font) error {
	var d *app.Dialog
	cmd := widget.NewCommand(face, "Type a command or search...", commandGroups(func() { d.Close() })...)
	var err error
	d, err = a.NewDialog(parent, app.DialogConfig{Title: "Command", Width: 460, Height: 340, Content: cmd, Focus: cmd.FocusTarget(), Bare: true})
	return err
}

type galleryMsg interface{ galleryMsg() }

type togglePalette struct{}

type submitted struct{ name string }

func (togglePalette) galleryMsg() {}
func (submitted) galleryMsg()     {}

// demo is one gallery section: a title over its body. A component
// that lives in an overlay names the action that opens it, and a sheet
// hosts the content it slides over.
type demo struct {
	title string
	body  []ui.Node
	wrap  func(ui.Node) ui.Node
	// gesture at the point target names opens the overlay; minSize
	// gives the overlay room in a shot window.
	gesture gesture
	target  func() image.Point
	minSize widget.Size
}

// section is the demo titled title showing body.
func section(title string, body ...ui.Node) demo { return demo{title: title, body: body} }

// opens names the gesture and the point that open the demo's overlay.
func (d demo) opens(g gesture, target func() image.Point) demo {
	d.gesture, d.target = g, target
	return d
}

// room gives the demo's shot window at least w by h for its overlay.
func (d demo) room(w, h int) demo { d.minSize = widget.Size{W: w, H: h}; return d }

// windowSize is the shot window for the demo built as root: its
// natural size, and the room its overlay asks for.
func (d demo) windowSize(root widget.Widget) widget.Size {
	n := root.Measure(widget.Constraints{Max: widget.Size{W: shotMaxWidth, H: 4096}})
	return widget.Size{W: max(n.W, d.minSize.W), H: max(n.H, d.minSize.H)}
}

// center is the point at the middle of *w, read when the shot needs it.
func center[W widget.Boundser](w *W) func() image.Point {
	return func() image.Point {
		b := (*w).Bounds()
		return image.Pt(b.X+b.W/2, b.Y+b.H/2)
	}
}

// leading is the point inset px into *w's leading edge, vertically
// centered: the first trigger of a row of them.
func leading[W widget.Boundser](w *W, inset int) func() image.Point {
	return func() image.Point {
		b := (*w).Bounds()
		return image.Pt(b.X+inset, b.Y+b.H/2)
	}
}

// hosts names how the demo's overlay wraps the content under it.
func (d demo) hosts(fn func(ui.Node) ui.Node) demo { d.wrap = fn; return d }

// host wraps content in the demo's overlay, if it has one.
func (d demo) host(content ui.Node) ui.Node {
	if d.wrap == nil {
		return content
	}
	return d.wrap(content)
}

// node is the demo as a gallery section: its title over its body.
func (d demo) node() ui.Node {
	return ui.Column(ui.Label(d.title).Classes(css.H4), d.bodyNode()).Spacing(12)
}

// bodyNode is the demo's body, its parts 16px apart: the section's
// content, and all a shot shows.
func (d demo) bodyNode() *ui.BoxBuilder { return ui.Column(d.body...).Spacing(16) }

type gallery struct {
	env           ui.Env
	status        string
	sidebar       *widget.Sidebar
	app           *app.Application
	win           *app.Window
	menuButton    *widget.Button
	popoverButton *widget.Button
	menubar       *widget.MenuBar
	// shown is the dialog a demo opened last.
	shown *app.Dialog
}

// demoMenu is the dropdown and menubar demos' menu.
func demoMenu() []widget.MenuItem {
	return []widget.MenuItem{
		{Label: "Profile", Accel: "Shift+Ctrl+P", OnClick: func() {}},
		{Label: "Billing", Accel: "Ctrl+B", OnClick: func() {}},
		{Label: "Settings", Accel: "Ctrl+S", OnClick: func() {}},
		{Kind: widget.ItemSeparator},
		{Label: "Log out", Accel: "Shift+Ctrl+Q", OnClick: func() {}},
	}
}

// menubarMenus are the Menubar demo's menus.
func menubarMenus() []widget.MenuBarMenu {
	item := func(label, accel string) widget.MenuItem {
		return widget.MenuItem{Label: label, Accel: accel, OnClick: func() {}}
	}
	return []widget.MenuBarMenu{
		{Label: "File", Items: []widget.MenuItem{
			item("New Tab", "Ctrl+T"), item("New Window", "Ctrl+N"),
			{Label: "New Incognito Window", Disabled: true},
			widget.MenuSeparator(),
			{Label: "Share", Items: []widget.MenuItem{item("Email link", ""), item("Messages", ""), item("Notes", "")}},
			widget.MenuSeparator(),
			item("Print...", "Ctrl+P"),
		}},
		{Label: "Edit", Items: []widget.MenuItem{
			item("Undo", "Ctrl+Z"), item("Redo", "Shift+Ctrl+Z"), widget.MenuSeparator(),
			item("Cut", ""), item("Copy", ""), item("Paste", ""),
		}},
		{Label: "View", Items: []widget.MenuItem{
			{Label: "Always Show Bookmarks Bar", Kind: widget.ItemCheck, OnClick: func() {}},
			{Label: "Always Show Full URLs", Kind: widget.ItemCheck, Checked: true, OnClick: func() {}},
			widget.MenuSeparator(),
			item("Reload", "Ctrl+R"),
			{Label: "Force Reload", Accel: "Shift+Ctrl+R", Disabled: true},
		}},
		{Label: "Profiles", Items: []widget.MenuItem{
			{Label: "Andy", Kind: widget.ItemRadio, Group: "profile", OnClick: func() {}},
			{Label: "Benoit", Kind: widget.ItemRadio, Group: "profile", Checked: true, OnClick: func() {}},
			{Label: "Luis", Kind: widget.ItemRadio, Group: "profile", OnClick: func() {}},
			widget.MenuSeparator(),
			item("Edit...", ""), item("Add Profile...", ""),
		}},
	}
}

// dropdownMenu opens the Dropdown Menu demo under its button.
func (g *gallery) dropdownMenu() {
	widget.OpenMenu(g.menuButton, g.menuButton, g.env.Face, 14, demoMenu(), nil)
}

// popover opens the Popover demo under its button.
func (g *gallery) popover() {
	field := func(label, value string) ui.Node {
		return ui.Field(label, ui.Entry().Text(value)).Orientation(widget.FieldHorizontal)
	}
	content, view := ui.Build(g.env, ui.Column(
		ui.Column(
			ui.Label("Dimensions").Classes(css.Heading),
			ui.Label("Set the dimensions for the layer.").Classes(css.Muted),
		).Spacing(8),
		ui.Column(field("Width", "100%"), field("Max. width", "300px"), field("Height", "25px"), field("Max. height", "none")).Spacing(8),
	).Spacing(16).Padding(render.UniformInsets(16)).Classes("popover-demo"))
	widget.OpenPopover(g.popoverButton, g.popoverButton, content, widget.PopoverOptions{OnClosed: view.Close})
}

// alertDialog opens the Alert Dialog demo.
func (g *gallery) alertDialog() {
	var err error
	if g.shown, err = g.app.MessageBox(g.win, "Are you absolutely sure?",
		"This action cannot be undone. This will permanently delete your account and remove your data from our servers.",
		[]app.DialogButton{{Label: "Cancel", Response: "cancel", Role: app.ButtonRoleCancel}, {Label: "Continue", Response: "continue", Role: app.ButtonRoleDefault}}); err != nil {
		log.Printf("lys-ui-gallery: alert dialog: %v", err)
	}
}

// dialog opens the Dialog demo.
func (g *gallery) dialog() {
	content, view := ui.Build(g.env, ui.Column(
		ui.Field("Name", ui.Entry().Text("Ada Lovelace")),
		ui.Field("Username", ui.Entry().Text("@ada")),
	).Spacing(16))
	var err error
	if g.shown, err = g.app.NewDialog(g.win, app.DialogConfig{
		Title: "Edit profile", Description: "Make changes to your profile here. Click save when you're done.",
		Width: 425, Content: content, Modal: true, OnClosed: view.Close,
		Buttons: []app.DialogButton{{Label: "Cancel", Response: "cancel", Role: app.ButtonRoleCancel}, {Label: "Save changes", Response: "save", Role: app.ButtonRoleDefault}},
	}); err != nil {
		log.Printf("lys-ui-gallery: dialog: %v", err)
	}
}

// toast shows the Sonner demo.
func (g *gallery) toast() {
	if _, err := g.app.ShowToast("Event has been created", 4*time.Second, nil); err != nil {
		log.Printf("lys-ui-gallery: toast: %v", err)
	}
}

// radioGroup is the Radio Group demo.
func (g *gallery) radioGroup() ui.Node {
	first := ui.CheckButton(true).Child(ui.Label("Default"))
	return ui.Column(first,
		ui.CheckButton(false).Child(ui.Label("Comfortable")).Group(first),
		ui.CheckButton(false).Child(ui.Label("Compact")).Group(first),
	).Spacing(12)
}

// scrollArea is the Scroll Area demo: a fixed box of tags.
func (g *gallery) scrollArea() ui.Node {
	tags := []ui.Node{ui.Label("Tags").Classes(css.Heading)}
	for i := 50; i > 0; i-- {
		tags = append(tags, ui.Label("v1.2.0-beta."+strconv.Itoa(i)), ui.Separator(widget.Horizontal))
	}
	return ui.Scroll(ui.Column(tags...).Spacing(8).Padding(render.UniformInsets(16))).VerticalOnly(true).MaxContentHeight(288).SizeRequest(192, 0).Classes("bordered")
}

// sidebarDemo is the Sidebar section: a sidebar collapsing to an icon
// rail beside a page, at a fixed height; Ctrl+B toggles it.
func (g *gallery) sidebarDemo() ui.Node {
	icon := func(k widget.SymbolKind) widget.Widget { return widget.NewSymbol(k, 16) }
	return ui.Sidebar(ui.Column(
		ui.Label("Dashboard").Classes(css.H4),
		ui.Label("Toggle the sidebar with Ctrl+B.").Classes(css.Muted),
	).Spacing(8).Padding(render.UniformInsets(16))).
		Collapse(widget.SidebarIcon).NarrowWidth(480).
		Header(ui.Label("Acme Inc").Classes(css.H4)).
		AddGroup("Platform",
			widget.SidebarItem{Label: "Home", Icon: icon(widget.SymbolMaximize), Active: true},
			widget.SidebarItem{Label: "Inbox", Icon: icon(widget.SymbolSearch), Badge: "24"},
			widget.SidebarItem{Label: "Calendar", Icon: icon(widget.SymbolCalendar)},
			widget.SidebarItem{Label: "Settings", Icon: icon(widget.SymbolCrosshair), Items: []widget.SidebarItem{
				{Label: "General"}, {Label: "Team"}, {Label: "Billing"},
			}},
		).
		Footer(ui.Label("lys-ui").Classes(css.Muted)).
		Ref(&g.sidebar).Classes("sidebar-demo")
}

// datePickers is the Date Picker section: a date, a date with a time,
// and a range with presets relative to today.
func (g *gallery) datePickers(first **widget.DatePicker) ui.Node {
	today := widget.Today()
	start := func(n ui.Node) ui.Node { return ui.Aligned(n, widget.AlignStart) }
	return ui.Column(
		start(ui.DatePicker("Pick a date").Ref(first)),
		start(ui.DatePicker("Pick a date and time").TimeInput(true)),
		start(ui.DatePicker("Pick a range").Mode(widget.CalendarRange).Presets(
			widget.DatePreset{Label: "Today", From: today, To: today},
			widget.DatePreset{Label: "Last 7 days", From: today.AddDays(-6), To: today},
			widget.DatePreset{Label: "Last 30 days", From: today.AddDays(-29), To: today},
		)),
	).Spacing(12)
}

// galleryCSS styles the gallery's own pieces: the context menu's
// dashed drop zone, the bordered demo frames, the hover card's and the
// popover's widths.
const galleryCSS = `
.drop-zone { border: 1px dashed @borders; border-radius: 8px; padding: 48px; }
.bordered { border: 1px solid @borders; border-radius: 8px; }
.hovercard-content { width: 320px; padding: 16px; }
.popover-demo { width: 320px; }
.sidebar-demo { min-height: 360px; border: 1px solid @borders; border-radius: 8px; }
.datatable-demo { min-height: 520px; }
`

// payment is one row of the data table section.
type payment struct {
	status, email string
	amount        int
}

// dataTable is the Data Table section: a thousand payments, filtered by
// email, sorted by amount, selectable, paged.
// invoice is a row of the Table demo.
type invoice struct{ id, status, method, amount string }

// table is the plain table demo: a few invoices, no paging,
// sorting or selection.
func (g *gallery) table() ui.Node {
	rows := widget.SliceModel[invoice]{
		{"INV001", "Paid", "Credit Card", "$250.00"},
		{"INV002", "Pending", "PayPal", "$150.00"},
		{"INV003", "Unpaid", "Bank Transfer", "$350.00"},
		{"INV004", "Paid", "Credit Card", "$450.00"},
		{"INV005", "Paid", "PayPal", "$550.00"},
	}
	text := func(s string) widget.Widget { return widget.NewLabel(g.env.Face, 14, s, 0) }
	cols := []widget.TableColumn[invoice]{
		{Title: "Invoice", Width: 100, Cell: func(r invoice) widget.Widget { return text(r.id) }},
		{Title: "Status", Width: 100, Cell: func(r invoice) widget.Widget { return text(r.status) }},
		{Title: "Method", Width: 160, Expand: true, Cell: func(r invoice) widget.Widget { return text(r.method) }},
		{Title: "Amount", Width: 100, Cell: func(r invoice) widget.Widget { return text(r.amount) }},
	}
	return ui.ColumnView[invoice](rows, cols).SizeRequest(0, 240)
}

func (g *gallery) dataTable() ui.Node {
	statuses := []string{"success", "processing", "failed", "pending"}
	rows := make(widget.SliceModel[payment], 1000)
	for i := range rows {
		rows[i] = payment{status: statuses[i%len(statuses)], email: fmt.Sprintf("user%d@example.com", i), amount: (i*7919)%1000 + 10}
	}
	face := g.env.Face
	text := func(s string) widget.Widget { return widget.NewLabel(face, 14, s, 0) }
	cols := []widget.TableColumn[payment]{
		{Title: "Status", Width: 110, Cell: func(p payment) widget.Widget { return text(p.status) }},
		{
			Title: "Email", Width: 220, Expand: true, Cell: func(p payment) widget.Widget { return text(p.email) },
			Sort: func(a, b payment) int { return strings.Compare(a.email, b.email) },
		},
		{
			Title: "Amount", Width: 100, Cell: func(p payment) widget.Widget { return text(fmt.Sprintf("$%d.00", p.amount)) },
			Sort: func(a, b payment) int { return a.amount - b.amount },
		},
	}
	return ui.DataTable[payment](rows, cols, func(p payment, q string) bool { return strings.Contains(p.email, q) }).
		Classes("datatable-demo")
}

// form is the Field and Form section: two required entries and a
// switch, validated on blur and on submit.
func (g *gallery) form(send func(galleryMsg)) ui.Node {
	var name, email *widget.Entry
	var form *widget.Form
	required := func(e **widget.Entry, what string) func() string {
		return func() string {
			if strings.TrimSpace((*e).Text()) == "" {
				return what + " is required."
			}
			return ""
		}
	}
	return ui.Column(
		ui.Form().Ref(&form).
			Append(ui.Label("Profile").Classes(css.H4)).
			Add(ui.Field("Name", ui.Entry().Ref(&name)).Description("Your public display name."), required(&name, "Your name")).
			Add(ui.Field("Email", ui.Entry().Ref(&email)), func() string {
				if !strings.Contains(email.Text(), "@") {
					return "Enter a valid email address."
				}
				return ""
			}).
			Add(ui.Field("Newsletter", ui.Switch(true)).Description("Monthly product updates.").Orientation(widget.FieldHorizontal), nil).
			OnSubmit(func() { send(submitted{name: name.Text()}) }),
		ui.Row(
			ui.Button(ui.Label("Submit"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { form.Submit() }),
			ui.Label("").Classes(css.Muted).WatchText(func() string { return g.status }),
		).Spacing(16),
	).Spacing(24)
}

// pane is a Resizable demo pane: its name centered in it.
func pane(name string) ui.Node {
	return ui.Label(name).Classes(css.Heading).HAlign(widget.AlignCenter).VAlign(widget.AlignCenter)
}

// carousel is the carousel demo: five numbered slides.
func (g *gallery) carousel() ui.Node {
	c := ui.Carousel().SizeRequest(0, 160)
	for i := range 5 {
		c.Append(ui.Label(strconv.Itoa(i + 1)).Classes(css.H1).HAlign(widget.AlignCenter).VAlign(widget.AlignCenter))
	}
	return c
}

func (g *gallery) Init(cx *component.Context[galleryMsg, struct{}]) widget.Widget {
	header := ui.Row(
		ui.Expand(ui.Label("lys-ui components").Classes(css.H2)),
		ui.Button(ui.Label("Toggle palette"), 0, 0).Classes(css.Outline).
			OnClick(func() { cx.Input(togglePalette{}) }),
	).Spacing(16)
	demos := g.demos(func(m galleryMsg) { cx.Input(m) })
	sections := []ui.Node{header}
	for _, d := range demos {
		sections = append(sections, d.node())
	}
	page := ui.Node(ui.Scroll(ui.Clamp(680, ui.Column(sections...).Spacing(40).Padding(render.UniformInsets(32)))).VerticalOnly(true).Classes(css.Background))
	for _, d := range demos {
		page = d.host(page)
	}
	return ui.Mount(cx, g.env, page)
}

// demos is every section of the gallery, in order. send delivers the
// form's submission to the gallery component.
func (g *gallery) demos(send func(galleryMsg)) []demo {
	text := func(s string) ui.Node { return ui.Label(s).Wrap(true) }
	var sheet, drawer *widget.Sheet
	var (
		alertButton, dialogButton, drawerButton, sheetButton, toastButton, hoverTrigger, tooltipButton *widget.Button
		contextMenu                                                                                    *widget.ContextMenu
		combobox                                                                                       *widget.Combobox[string]
		datePicker                                                                                     *widget.DatePicker
		navMenu                                                                                        *widget.NavigationMenu
		selectMenu                                                                                     *widget.Dropdown
	)
	return []demo{
		section("Accordion", ui.Accordion(widget.AccordionSingle).Collapsible(true).
			Append(ui.AccordionItem("Is it accessible?", text("Yes. The triggers take focus, and Up, Down, Home and End move between them."))).
			Append(ui.AccordionItem("Is it styled?", text("Yes. It comes with default styles that match the other components' aesthetic.")).Open(true)).
			Append(ui.AccordionItem("Is it animated?", text("Yes. The content height and the chevron animate unless reduced motion is on.")))),
		section("Alert",
			ui.Alert("Heads up!", "You can add components to your app using the ui builders.").
				Icon(ui.Symbol(widget.SymbolCrosshair, 16)),
			ui.Alert("Unable to process your payment.", "Please verify your billing information and try again.").
				Icon(ui.Symbol(widget.SymbolClose, 16)).Variant(widget.AlertDestructive)),
		section("Alert Dialog", ui.Aligned(ui.Button(ui.Label("Show Dialog"), 0, 0).Classes(css.Outline).Ref(&alertButton).OnClick(g.alertDialog), widget.AlignStart)).opens(click, center(&alertButton)).room(560, 260),
		section("Aspect Ratio", ui.Aligned(ui.AspectFrame(ui.Column(
			ui.Expand(ui.Spacer(0, 0)),
			ui.Aligned(ui.Label("16 : 9").Classes(css.DimLabel), widget.AlignCenter),
			ui.Expand(ui.Spacer(0, 0)),
		).SizeRequest(360, 0).Classes(css.Card), 16.0/9), widget.AlignStart)),
		section("Avatar", ui.Row(ui.Avatar(40, "Ada Lovelace"), ui.Avatar(40, "Grace Hopper"), ui.Avatar(40, "")).Spacing(12)),
		section("Badge", ui.Row(
			ui.Badge("Default"),
			ui.Badge("Secondary").Variant(widget.BadgeSecondary),
			ui.Badge("Destructive").Variant(widget.BadgeDestructive),
			ui.Badge("Outline").Variant(widget.BadgeOutline),
		).Spacing(8)),
		section("Button", ui.Row(
			ui.Button(ui.Label("Button"), 0, 0).Classes(css.SuggestedAction),
			ui.Button(ui.Label("Secondary"), 0, 0).Classes(css.Secondary),
			ui.Button(ui.Label("Destructive"), 0, 0).Classes(css.DestructiveAction),
			ui.Button(ui.Label("Outline"), 0, 0).Classes(css.Outline),
			ui.Button(ui.Label("Ghost"), 0, 0).Classes(css.Ghost),
			ui.Button(ui.Label("Link"), 0, 0).Classes(css.Link),
		).Spacing(8)),
		section("Button Group", ui.Aligned(ui.ButtonGroup(
			ui.Button(ui.Label("Archive"), 0, 0).Classes(css.Outline),
			ui.Button(ui.Label("Report"), 0, 0).Classes(css.Outline),
			ui.Button(ui.Label("Snooze"), 0, 0).Classes(css.Outline),
		), widget.AlignStart)),
		section("Breadcrumb",
			ui.Breadcrumb("Home", "Components", "Breadcrumb"),
			ui.Breadcrumb("Home", "Docs", "Components", "Data", "Breadcrumb").MaxItems(3)),
		section("Calendar", ui.Aligned(ui.Column(ui.Calendar(widget.Today())).Padding(render.UniformInsets(8)).Classes(css.Card), widget.AlignStart)),
		section("Card", ui.Aligned(ui.Column(
			ui.Column(
				ui.Label("Create project").Classes(css.Heading),
				ui.Label("Deploy your new project in one click.").Classes(css.Muted),
			).Spacing(6),
			ui.Field("Name", ui.Entry().Placeholder("Name of your project")),
			ui.Row(ui.Expand(ui.Spacer(0, 0)), ui.Button(ui.Label("Cancel"), 0, 0).Classes(css.Outline), ui.Button(ui.Label("Deploy"), 0, 0).Classes(css.SuggestedAction)).Spacing(8),
		).Spacing(24).Padding(render.UniformInsets(24)).SizeRequest(360, 0).Classes(css.Card), widget.AlignStart)),
		section("Carousel", g.carousel()),
		section("Chart", ui.Row(
			ui.Expand(ui.Chart(widget.ChartBar, []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"},
				widget.ChartSeries{Name: "Desktop", Values: []float64{186, 305, 237, 73, 209, 214}},
				widget.ChartSeries{Name: "Mobile", Values: []float64{80, 200, 120, 190, 130, 140}},
			)),
			ui.Expand(ui.Chart(widget.ChartPie, []string{"Chrome", "Safari", "Firefox", "Edge", "Other"},
				widget.ChartSeries{Name: "Visitors", Values: []float64{275, 200, 187, 173, 90}},
			).Donut(true)),
		).Spacing(16)),
		section("Checkbox", ui.Aligned(ui.CheckButton(true).Child(ui.Label("Accept terms and conditions")), widget.AlignStart)),
		section("Collapsible", ui.Expander("@stubbedev starred 3 repositories", ui.Column(
			ui.Label("stubbedev/lys-ui"), ui.Label("stubbedev/lys-shell"), ui.Label("stubbedev/lys-wm"),
		).Spacing(8))),
		section("Combobox", ui.Aligned(ui.Combobox[string]("Select compositor...",
			widget.ComboboxOption[string]{Value: "lys-wm", Label: "lys-wm"},
			widget.ComboboxOption[string]{Value: "sway", Label: "sway"},
			widget.ComboboxOption[string]{Value: "hyprland", Label: "Hyprland"},
			widget.ComboboxOption[string]{Value: "niri", Label: "niri"},
			widget.ComboboxOption[string]{Value: "river", Label: "river"},
		).Ref(&combobox), widget.AlignStart)).opens(click, center(&combobox)).room(300, 300),
		section("Command", ui.Command("Type a command or search...", commandGroups(func() {})...).Classes(css.Card)),
		section("Context Menu", ui.ContextMenu(
			ui.Label("Right click here").Classes(css.Muted),
			widget.MenuItem{Label: "Back", Accel: "Alt+Left", OnClick: func() {}},
			widget.MenuItem{Label: "Forward", Accel: "Alt+Right", Disabled: true},
			widget.MenuItem{Label: "Reload", Accel: "Ctrl+R", OnClick: func() {}},
			widget.MenuSeparator(),
			widget.MenuItem{Label: "Show Bookmarks", Kind: widget.ItemCheck, Checked: true, OnClick: func() {}},
		).Classes("drop-zone").Ref(&contextMenu)).opens(secondaryClick, center(&contextMenu)),
		section("Data Table", g.dataTable()),
		section("Date Picker", g.datePickers(&datePicker)).opens(click, center(&datePicker)).room(360, 420),
		section("Dialog", ui.Aligned(ui.Button(ui.Label("Edit Profile"), 0, 0).Classes(css.Outline).Ref(&dialogButton).OnClick(g.dialog), widget.AlignStart)).opens(click, center(&dialogButton)).room(560, 340),
		section("Drawer", ui.Aligned(ui.Button(ui.Label("Open Drawer"), 0, 0).Classes(css.Outline).Ref(&drawerButton).OnClick(func() { drawer.SetOpen(true) }), widget.AlignStart)).
			opens(click, center(&drawerButton)).room(640, 420).
			hosts(func(page ui.Node) ui.Node {
				return ui.Sheet(page, ui.Aligned(ui.Label("350 calories per day").Classes(css.H3), widget.AlignCenter), widget.SheetBottom).
					Header(g.env.Face, "Move Goal", "Set your daily activity goal.").
					Footer(ui.Button(ui.Label("Submit"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { drawer.SetOpen(false) })).
					Ref(&drawer)
			}),
		section("Dropdown Menu", ui.Aligned(ui.Button(ui.Label("Open"), 0, 0).Classes(css.Outline).Ref(&g.menuButton).OnClick(g.dropdownMenu), widget.AlignStart)).opens(click, center(&g.menuButton)),
		section("Empty", ui.StatusPage("", "No projects yet", "You haven't created any projects yet. Get started by creating your first project.").
			Icon(ui.Symbol(widget.SymbolMaximize, 24)).
			Child(ui.Button(ui.Label("Create project"), 0, 0).Classes(css.SuggestedAction))),
		section("Field and Form", g.form(send)),
		section("Hover Card", ui.Aligned(ui.HoverCard(
			ui.Button(ui.Label("@lys-ui"), 0, 0).Classes(css.Link).Ref(&hoverTrigger),
			ui.Column(
				ui.Label("lys-ui").Classes(css.Heading),
				ui.Label("A pure-Go widget toolkit for Wayland desktops, no cgo.").Wrap(true),
				ui.Label("Part of the lys desktop since 2026").Classes(css.Caption, css.DimLabel),
			).Spacing(4),
		), widget.AlignStart)).opens(hover, center(&hoverTrigger)).room(360, 240),
		section("Input", ui.Aligned(ui.Entry().Placeholder("Email").SizeRequest(320, 0), widget.AlignStart)),
		section("Input Group",
			ui.InputGroup(ui.Entry().Placeholder("Search...")).
				Prepend(ui.Symbol(widget.SymbolSearch, 16)).Append(ui.Label("12 results")),
			ui.InputGroup(ui.Entry().Text("example.com")).
				Prepend(ui.Label("https://")).
				Append(ui.Button(ui.Label("Copy"), 0, 0).Classes(css.Ghost, css.Small))),
		section("Input OTP", ui.InputOTP(widget.OTPDigits, 3, 3)),
		section("Item",
			ui.Item("Basic Item", "A simple item with title and description.").Variant(widget.ItemOutline).
				AddAction(ui.Button(ui.Label("Action"), 0, 0).Classes(css.Outline, css.Small)),
			ui.Item("Your profile has been verified.", "").Variant(widget.ItemMuted).Small(true),
			ui.ItemGroup(
				ui.Item("ada", "ada@example.com"),
				ui.Item("grace", "grace@example.com"),
				ui.Item("linus", "linus@example.com"),
			).Classes(css.Card),
		),
		section("Kbd", ui.Row(ui.KbdGroup("Ctrl", "Shift", "P"), ui.Label("opens the command palette")).Spacing(12)),
		section("Menubar", ui.Aligned(ui.MenuBar(menubarMenus()...).Ref(&g.menubar), widget.AlignStart)).
			opens(click, func() image.Point { b := g.menubar.Root(0).Bounds(); return image.Pt(b.X+b.W/2, b.Y+b.H/2) }),
		section("Navigation Menu", ui.Aligned(ui.NavigationMenu(
			widget.NavigationMenuItem{Label: "Getting started", Content: widget.NewNavigationLinks(g.env.Face, 2,
				widget.NavigationLink{Title: "Introduction", Description: "Re-usable components built on lys-ui's widgets."},
				widget.NavigationLink{Title: "Installation", Description: "How to add lys-ui to a Go module."},
				widget.NavigationLink{Title: "Typography", Description: "Styles for headings, paragraphs and lists."},
				widget.NavigationLink{Title: "Theming", Description: "The dark and light palettes and their tokens."},
			)},
			widget.NavigationMenuItem{Label: "Components", Content: widget.NewNavigationLinks(g.env.Face, 1,
				widget.NavigationLink{Title: "Alert", Description: "An inline callout for the user's attention."},
				widget.NavigationLink{Title: "Hover Card", Description: "Preview content behind a link."},
			)},
			widget.NavigationMenuItem{Label: "Docs", OnActivate: func() {}},
		).Ref(&navMenu), widget.AlignStart)).opens(click, leading(&navMenu, 40)).room(640, 300),
		section("Pagination", ui.Pagination(10, 5)),
		section("Popover", ui.Aligned(ui.Button(ui.Label("Open popover"), 0, 0).Classes(css.Outline).Ref(&g.popoverButton).OnClick(g.popover), widget.AlignStart)).opens(click, center(&g.popoverButton)),
		section("Progress", ui.Aligned(ui.ProgressBar(0.66).SizeRequest(360, 0), widget.AlignStart)),
		section("Radio Group", g.radioGroup()),
		section("Resizable", ui.Aligned(ui.Column(ui.Expand(ui.Paned(widget.Row,
			pane("One"),
			ui.Paned(widget.Column, pane("Two"), pane("Three")).Position(56),
		).Position(224))).SizeRequest(448, 200).Classes("bordered"), widget.AlignStart)),
		section("Scroll Area", ui.Aligned(g.scrollArea(), widget.AlignStart)),
		section("Select", ui.Aligned(ui.Dropdown([]string{"Apple", "Banana", "Blueberry", "Grapes", "Pineapple"}, 0).Ref(&selectMenu), widget.AlignStart)).opens(click, center(&selectMenu)).room(320, 320),
		section("Separator", ui.Column(
			ui.Label("lys-ui").Classes(css.Heading),
			ui.Label("A pure-Go widget toolkit for Wayland.").Classes(css.Muted),
			ui.Separator(widget.Horizontal),
			ui.Row(ui.Label("Blog"), ui.Label("Docs"), ui.Label("Source")).Spacing(16),
		).Spacing(8)),
		section("Sheet", ui.Aligned(ui.Button(ui.Label("Open"), 0, 0).Classes(css.Outline).Ref(&sheetButton).OnClick(func() { sheet.SetOpen(true) }), widget.AlignStart)).
			opens(click, center(&sheetButton)).room(720, 480).
			hosts(func(page ui.Node) ui.Node {
				return ui.Sheet(page, ui.Field("Name", ui.Entry().Text("Ada Lovelace")), widget.SheetRight).
					Header(g.env.Face, "Edit profile", "Make changes to your profile here. Click save when you're done.").
					Footer(ui.Button(ui.Label("Save changes"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { sheet.SetOpen(false) })).
					Ref(&sheet)
			}),
		section("Sidebar", g.sidebarDemo()),
		section("Skeleton", ui.Row(
			ui.Skeleton(48, 48),
			ui.Column(ui.Skeleton(240, 14), ui.Skeleton(180, 14)).Spacing(8),
		).Spacing(16)),
		section("Slider", ui.Aligned(ui.Slider(0, 100, 1, 50).SizeRequest(360, 0), widget.AlignStart)),
		section("Sonner", ui.Aligned(ui.Button(ui.Label("Show Toast"), 0, 0).Classes(css.Outline).Ref(&toastButton).OnClick(g.toast), widget.AlignStart)).opens(click, center(&toastButton)).room(480, 200),
		section("Spinner", ui.Row(ui.Spinner(16), ui.Label("Processing payment...")).Spacing(8)),
		section("Switch", ui.Row(ui.Switch(false), ui.Label("Airplane Mode")).Spacing(8)),
		section("Table", g.table()),
		section("Tabs", ui.Aligned(ui.Notebook().SizeRequest(400, 0).
			AppendTab("Account", text("Make changes to your account here.")).
			AppendTab("Password", text("Change your password here.")), widget.AlignStart)),
		section("Textarea", ui.Aligned(ui.TextArea().Placeholder("Type your message here.").SizeRequest(400, 80), widget.AlignStart)),
		section("Toggle", ui.Row(
			ui.ToggleButton(ui.Label("Bold"), 0, 0).Classes(css.Outline),
			ui.ToggleButton(ui.Label("Italic"), 0, 0).Classes(css.Outline).Active(true),
			ui.ToggleButton(ui.Label("Underline"), 0, 0).Classes(css.Ghost),
		).Spacing(8)),
		section("Toggle Group", ui.Aligned(ui.ButtonGroup(
			ui.ToggleButton(ui.Label("Bold"), 0, 0).Classes(css.Outline).Active(true),
			ui.ToggleButton(ui.Label("Italic"), 0, 0).Classes(css.Outline),
			ui.ToggleButton(ui.Label("Underline"), 0, 0).Classes(css.Outline),
		), widget.AlignStart)),
		section("Tooltip", ui.Aligned(ui.Button(ui.Label("Hover"), 0, 0).Classes(css.Outline).Tooltip("Add to library").Ref(&tooltipButton), widget.AlignStart)).opens(hover, center(&tooltipButton)),
		section("Typography",
			ui.Label("The Joke Tax Chronicles").Classes(css.H1),
			ui.Label("Once upon a time, in a far-off land, there was a very lazy king who spent all day lounging on his throne.").Classes(css.Lead).Wrap(true),
			ui.Label("The King's Plan").Classes(css.H2),
			ui.Label("\"After all,\" he said, \"everyone enjoys a good joke, so it's only fair that they should pay for the privilege.\"").Classes(css.Blockquote).Wrap(true),
			ui.BulletList("1st level of puns: 5 gold coins", "2nd level of jokes: 10 gold coins", "3rd level of one-liners: 20 gold coins"),
			ui.Aligned(ui.Label("ui.Label(text).Classes(css.InlineCode)").Classes(css.InlineCode), widget.AlignStart),
			ui.Label("Enter your email address.").Classes(css.Muted),
		),
	}
}

func (g *gallery) Update(_ *component.Context[galleryMsg, struct{}], msg galleryMsg) {
	switch m := msg.(type) {
	case submitted:
		g.status = "Saved " + m.name + "."
	case togglePalette:
		if widget.Current().IsDark() {
			widget.SetTheme(widget.LightTheme())
		} else {
			widget.SetTheme(widget.DarkTheme())
		}
	}
}
