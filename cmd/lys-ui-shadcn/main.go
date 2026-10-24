// Command lys-ui-shadcn is the shadcn/ui gallery: every shadcn
// component lys-ui implements, one titled section each, declared with
// the typed ui builders in the shadcn dark and light palettes. Ctrl+K
// opens the command palette as a dialog, Ctrl+T or the header button
// toggles the palette, Escape closes.
package main

import (
	"errors"
	"fmt"
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
	switch err := run(); {
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
		Title:      "lys-ui shadcn gallery",
		AppID:      "dev.stubbe.lys.ui.shadcn",
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
				log.Printf("lys-ui-shadcn: command dialog: %v", err)
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
	d, err = a.NewDialog(parent, app.DialogConfig{Title: "Command", Width: 460, Height: 340, Content: cmd, Focus: cmd.Entry(), Bare: true})
	return err
}

type galleryMsg interface{ galleryMsg() }

type togglePalette struct{}

type submitted struct{ name string }

func (togglePalette) galleryMsg() {}
func (submitted) galleryMsg()     {}

type gallery struct {
	env           ui.Env
	status        string
	openSheet     func()
	openDrawer    func()
	sidebar       *widget.Sidebar
	app           *app.Application
	win           *app.Window
	menuButton    *widget.Button
	popoverButton *widget.Button
	menubar       *widget.MenuBar
}

// demoMenu is the dropdown and menubar demos' menu.
func demoMenu() []widget.MenuItem {
	return []widget.MenuItem{
		{Label: "Profile", Accel: "Shift+Ctrl+P"},
		{Label: "Billing", Accel: "Ctrl+B"},
		{Label: "Settings", Accel: "Ctrl+S"},
		{Kind: widget.ItemSeparator},
		{Label: "Log out", Accel: "Shift+Ctrl+Q"},
	}
}

// dropdownMenu opens the Dropdown Menu demo under its button.
func (g *gallery) dropdownMenu() {
	widget.OpenMenu(g.menuButton, g.menuButton, g.env.Face, 14, demoMenu())
}

// menubarRoot opens a menubar root's menu under it.
func (g *gallery) menubarRoot(_ int, anchor widget.Boundser) {
	widget.OpenMenu(g.menubar, anchor, g.env.Face, 14, demoMenu())
}

// popover opens the Popover demo under its button.
func (g *gallery) popover() {
	content, view := ui.Build(g.env, ui.Column(
		ui.Label("Dimensions").Classes(css.Heading),
		ui.Label("Set the dimensions for the layer.").Classes(css.Muted),
		ui.Field("Width", ui.Entry().Text("100%")),
		ui.Field("Height", ui.Entry().Text("25px")),
	).Spacing(8).Padding(render.UniformInsets(16)))
	widget.OpenPopover(g.popoverButton, g.popoverButton, content, widget.PopoverOptions{OnClosed: view.Close})
}

// alertDialog opens the Alert Dialog demo.
func (g *gallery) alertDialog() {
	if _, err := g.app.MessageBox(g.win, "Are you absolutely sure?",
		"This action cannot be undone. This will permanently delete your account and remove your data from our servers.",
		[]app.DialogButton{{Label: "Cancel", Response: "cancel", Role: app.ButtonRoleCancel}, {Label: "Continue", Response: "continue", Role: app.ButtonRoleDefault}}); err != nil {
		log.Printf("lys-ui-shadcn: alert dialog: %v", err)
	}
}

// dialog opens the Dialog demo.
func (g *gallery) dialog() {
	content, view := ui.Build(g.env, ui.Column(
		ui.Field("Name", ui.Entry().Text("Pedro Duarte")),
		ui.Field("Username", ui.Entry().Text("@peduarte")),
	).Spacing(16))
	if _, err := g.app.NewDialog(g.win, app.DialogConfig{
		Title: "Edit profile", Width: 425, Height: 260, Content: content, Modal: true, OnClosed: view.Close,
		Buttons: []app.DialogButton{{Label: "Cancel", Response: "cancel", Role: app.ButtonRoleCancel}, {Label: "Save changes", Response: "save", Role: app.ButtonRoleDefault}},
	}); err != nil {
		log.Printf("lys-ui-shadcn: dialog: %v", err)
	}
}

// toast shows the Sonner demo.
func (g *gallery) toast() {
	if _, err := g.app.ShowToast("Event has been created", 4*time.Second, nil); err != nil {
		log.Printf("lys-ui-shadcn: toast: %v", err)
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
	tags := make([]ui.Node, 0, 50)
	for i := 50; i > 0; i-- {
		tags = append(tags, ui.Label("v1.2.0-beta."+strconv.Itoa(i)))
	}
	return ui.Scroll(ui.Column(tags...).Spacing(8).Padding(render.UniformInsets(16))).VerticalOnly(true).MaxContentHeight(240).SizeRequest(200, 0).Classes(css.Card)
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
		Footer(ui.Label("shadcn").Classes(css.Muted)).
		Ref(&g.sidebar).Classes("sidebar-demo")
}

// datePickers is the Date Picker section: a date, a date with a time,
// and a range with presets relative to today.
func (g *gallery) datePickers() ui.Node {
	today := widget.Today()
	start := func(n ui.Node) ui.Node { return ui.Aligned(n, widget.AlignStart) }
	return ui.Column(
		start(ui.DatePicker("Pick a date")),
		start(ui.DatePicker("Pick a date and time").TimeInput(true)),
		start(ui.DatePicker("Pick a range").Mode(widget.CalendarRange).Presets(
			widget.DatePreset{Label: "Today", From: today, To: today},
			widget.DatePreset{Label: "Last 7 days", From: today.AddDays(-6), To: today},
			widget.DatePreset{Label: "Last 30 days", From: today.AddDays(-29), To: today},
		)),
	).Spacing(12)
}

// galleryCSS styles the gallery's own pieces: the context menu's
// dashed drop zone, the hover card's width.
const galleryCSS = `
.drop-zone { border: 1px dashed @borders; border-radius: 8px; padding: 48px; }
.hovercard-content { min-width: 280px; padding: 16px; }
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

// table is shadcn's plain table demo: a few invoices, no paging,
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
func (g *gallery) form(cx *component.Context[galleryMsg, struct{}]) ui.Node {
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
			OnSubmit(func() { cx.Input(submitted{name: name.Text()}) }),
		ui.Row(
			ui.Button(ui.Label("Submit"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { form.Submit() }),
			ui.Label("").Classes(css.Muted).WatchText(func() string { return g.status }),
		).Spacing(16),
	).Spacing(24)
}

// carousel is shadcn's carousel demo: five numbered slides.
func (g *gallery) carousel() ui.Node {
	c := ui.Carousel().SizeRequest(0, 160)
	for i := range 5 {
		c.Append(ui.Label(strconv.Itoa(i + 1)).Classes(css.H1).HAlign(widget.AlignCenter).VAlign(widget.AlignCenter))
	}
	return c
}

func (g *gallery) Init(cx *component.Context[galleryMsg, struct{}]) widget.Widget {
	section := func(title string, body ...ui.Node) ui.Node {
		return ui.Column(append([]ui.Node{ui.Label(title).Classes(css.H4)}, body...)...).Spacing(12)
	}
	text := func(s string) ui.Node { return ui.Label(s).Wrap(true) }
	header := ui.Row(
		ui.Expand(ui.Label("shadcn/ui in lys-ui").Classes(css.H2)),
		ui.Button(ui.Label("Toggle palette"), 0, 0).Classes(css.Outline).
			OnClick(func() { cx.Input(togglePalette{}) }),
	).Spacing(16)
	var sheet *widget.Sheet
	page := ui.Scroll(ui.Clamp(680, ui.Column(
		header,
		section("Accordion", ui.Accordion(widget.AccordionSingle).Collapsible(true).
			Append(ui.AccordionItem("Is it accessible?", text("Yes. The triggers take focus, and Up, Down, Home and End move between them."))).
			Append(ui.AccordionItem("Is it styled?", text("Yes. It comes with default styles that match the other components' aesthetic.")).Open(true)).
			Append(ui.AccordionItem("Is it animated?", text("Yes. The content height and the chevron animate unless reduced motion is on.")))),
		section("Alert",
			ui.Alert("Heads up!", "You can add components to your app using the ui builders.").
				Icon(ui.Symbol(widget.SymbolCrosshair, 16)),
			ui.Alert("Unable to process your payment.", "Please verify your billing information and try again.").
				Icon(ui.Symbol(widget.SymbolClose, 16)).Variant(widget.AlertDestructive)),
		section("Alert Dialog", ui.Aligned(ui.Button(ui.Label("Show Dialog"), 0, 0).Classes(css.Outline).OnClick(g.alertDialog), widget.AlignStart)),
		section("Aspect Ratio", ui.Aligned(ui.AspectFrame(ui.Column().SizeRequest(360, 0).Classes(css.Card), 16.0/9), widget.AlignStart)),
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
			ui.Label("Create project").Classes(css.Heading),
			ui.Label("Deploy your new project in one click.").Classes(css.Muted),
			ui.Field("Name", ui.Entry().Placeholder("Name of your project")),
			ui.Row(ui.Expand(ui.Spacer(0, 0)), ui.Button(ui.Label("Cancel"), 0, 0).Classes(css.Outline), ui.Button(ui.Label("Deploy"), 0, 0).Classes(css.SuggestedAction)).Spacing(8),
		).Spacing(16).Padding(render.UniformInsets(24)).SizeRequest(360, 0).Classes(css.Card), widget.AlignStart)),
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
		section("Collapsible", ui.Expander("@peduarte starred 3 repositories", ui.Column(
			ui.Label("@radix-ui/primitives"), ui.Label("@radix-ui/colors"), ui.Label("@stitches/react"),
		).Spacing(8))),
		section("Combobox", ui.Aligned(ui.Combobox("Select framework...",
			widget.ComboboxOption[string]{Value: "next", Label: "Next.js"},
			widget.ComboboxOption[string]{Value: "svelte", Label: "SvelteKit"},
			widget.ComboboxOption[string]{Value: "nuxt", Label: "Nuxt.js"},
			widget.ComboboxOption[string]{Value: "remix", Label: "Remix"},
			widget.ComboboxOption[string]{Value: "astro", Label: "Astro"},
		), widget.AlignStart)),
		section("Command", ui.Command("Type a command or search...", commandGroups(func() {})...).Classes(css.Card)),
		section("Context Menu", ui.ContextMenu(
			ui.Label("Right click here").Classes(css.Muted),
			widget.MenuItem{Label: "Back", Accel: "Alt+Left", OnClick: func() {}},
			widget.MenuItem{Label: "Forward", Accel: "Alt+Right", Disabled: true},
			widget.MenuItem{Label: "Reload", Accel: "Ctrl+R", OnClick: func() {}},
			widget.MenuSeparator(),
			widget.MenuItem{Label: "Show Bookmarks", Kind: widget.ItemCheck, Checked: true, OnClick: func() {}},
		).Classes("drop-zone")),
		section("Data Table", g.dataTable()),
		section("Date Picker", g.datePickers()),
		section("Dialog", ui.Aligned(ui.Button(ui.Label("Edit Profile"), 0, 0).Classes(css.Outline).OnClick(g.dialog), widget.AlignStart)),
		section("Drawer", ui.Aligned(ui.Button(ui.Label("Open Drawer"), 0, 0).Classes(css.Outline).OnClick(func() { g.openDrawer() }), widget.AlignStart)),
		section("Dropdown Menu", ui.Aligned(ui.Button(ui.Label("Open"), 0, 0).Classes(css.Outline).Ref(&g.menuButton).OnClick(g.dropdownMenu), widget.AlignStart)),
		section("Empty", ui.StatusPage("", "No projects yet", "You haven't created any projects yet. Get started by creating your first project.").
			Icon(ui.Symbol(widget.SymbolMaximize, 24)).
			Child(ui.Button(ui.Label("Create project"), 0, 0).Classes(css.SuggestedAction))),
		section("Field and Form", g.form(cx)),
		section("Hover Card", ui.Aligned(ui.HoverCard(
			ui.Button(ui.Label("@nextjs"), 0, 0).Classes(css.Link),
			ui.Column(
				ui.Label("@nextjs").Classes(css.H4),
				ui.Label("The React Framework - created and maintained by @vercel.").Wrap(true),
				ui.Label("Joined December 2021").Classes(css.Muted),
			).Spacing(8),
		), widget.AlignStart)),
		section("Input", ui.Aligned(ui.Entry().Placeholder("Email").SizeRequest(320, 0), widget.AlignStart)),
		section("Input Group",
			ui.InputGroup(ui.Entry().Placeholder("Search...")).
				Prepend(ui.Symbol(widget.SymbolCrosshair, 16)).Append(ui.Label("12 results")),
			ui.InputGroup(ui.Entry().Text("example.com")).
				Prepend(ui.Label("https://")).
				Append(ui.Button(ui.Label("Copy"), 0, 0).Classes(css.Ghost, css.Small))),
		section("Input OTP", ui.InputOTP(widget.OTPDigits, 3, 3)),
		section("Item",
			ui.Item("Basic Item", "A simple item with title and description.").Variant(widget.ItemOutline).
				AddAction(ui.Button(ui.Label("Action"), 0, 0).Classes(css.Outline, css.Small)),
			ui.Item("Your profile has been verified.", "").Variant(widget.ItemMuted).Small(true),
			ui.ItemGroup(
				ui.Item("shadcn", "shadcn@vercel.com"),
				ui.Item("maxleiter", "maxleiter@vercel.com"),
				ui.Item("evilrabbit", "evilrabbit@vercel.com"),
			).Classes(css.Card),
		),
		section("Kbd", ui.Row(ui.KbdGroup("Ctrl", "Shift", "P"), ui.Label("opens the command palette")).Spacing(12)),
		section("Menubar", ui.Aligned(ui.MenuBar("File", "Edit", "View", "Profiles").Ref(&g.menubar).OnRoot(g.menubarRoot), widget.AlignStart)),
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
		), widget.AlignStart)),
		section("Pagination", ui.Pagination(10, 5)),
		section("Popover", ui.Aligned(ui.Button(ui.Label("Open popover"), 0, 0).Classes(css.Outline).Ref(&g.popoverButton).OnClick(g.popover), widget.AlignStart)),
		section("Progress", ui.Aligned(ui.ProgressBar(0.66).SizeRequest(360, 0), widget.AlignStart)),
		section("Radio Group", g.radioGroup()),
		section("Resizable", ui.Column(ui.Expand(ui.Paned(widget.Row, ui.Label("One").HAlign(widget.AlignCenter), ui.Label("Two").HAlign(widget.AlignCenter)).Position(200))).SizeRequest(0, 160).Classes(css.Card)),
		section("Scroll Area", ui.Aligned(g.scrollArea(), widget.AlignStart)),
		section("Select", ui.Aligned(ui.Dropdown([]string{"Apple", "Banana", "Blueberry", "Grapes", "Pineapple"}, 0), widget.AlignStart)),
		section("Separator", ui.Column(
			ui.Label("Radix Primitives").Classes(css.Heading),
			ui.Label("An open-source UI component library.").Classes(css.Muted),
			ui.Separator(widget.Horizontal),
			ui.Row(ui.Label("Blog"), ui.Label("Docs"), ui.Label("Source")).Spacing(16),
		).Spacing(8)),
		section("Sheet", ui.Aligned(ui.Button(ui.Label("Open"), 0, 0).Classes(css.Outline).OnClick(func() { g.openSheet() }), widget.AlignStart)),
		section("Sidebar", g.sidebarDemo()),
		section("Skeleton", ui.Row(
			ui.Skeleton(48, 48),
			ui.Column(ui.Skeleton(240, 14), ui.Skeleton(180, 14)).Spacing(8),
		).Spacing(16)),
		section("Slider", ui.Aligned(ui.Slider(0, 100, 1, 50).SizeRequest(360, 0), widget.AlignStart)),
		section("Sonner", ui.Aligned(ui.Button(ui.Label("Show Toast"), 0, 0).Classes(css.Outline).OnClick(g.toast), widget.AlignStart)),
		section("Spinner", ui.Row(ui.Spinner(16), ui.Label("Processing payment...")).Spacing(8)),
		section("Switch", ui.Row(ui.Switch(false), ui.Label("Airplane Mode")).Spacing(8)),
		section("Table", g.table()),
		section("Tabs", ui.Aligned(ui.Notebook().SizeRequest(400, 0).
			AppendTab("Account", text("Make changes to your account here.")).
			AppendTab("Password", text("Change your password here.")), widget.AlignStart)),
		section("Textarea", ui.Aligned(ui.TextArea().Placeholder("Type your message here.").SizeRequest(400, 80), widget.AlignStart)),
		section("Toggle", ui.Aligned(ui.ToggleButton(ui.Label("Bold"), 0, 0).Classes(css.Ghost), widget.AlignStart)),
		section("Toggle Group", ui.Aligned(ui.ButtonGroup(
			ui.ToggleButton(ui.Label("Bold"), 0, 0).Classes(css.Outline).Active(true),
			ui.ToggleButton(ui.Label("Italic"), 0, 0).Classes(css.Outline),
			ui.ToggleButton(ui.Label("Underline"), 0, 0).Classes(css.Outline),
		), widget.AlignStart)),
		section("Tooltip", ui.Aligned(ui.Button(ui.Label("Hover"), 0, 0).Classes(css.Outline).Tooltip("Add to library"), widget.AlignStart)),
		section("Typography",
			ui.Label("The Joke Tax Chronicles").Classes(css.H1),
			ui.Label("Once upon a time, in a far-off land, there was a very lazy king who spent all day lounging on his throne.").Classes(css.Lead).Wrap(true),
			ui.Label("The King's Plan").Classes(css.H2),
			ui.Label("\"After all,\" he said, \"everyone enjoys a good joke, so it's only fair that they should pay for the privilege.\"").Classes(css.Blockquote).Wrap(true),
			ui.BulletList("1st level of puns: 5 gold coins", "2nd level of jokes: 10 gold coins", "3rd level of one-liners: 20 gold coins"),
			ui.Aligned(ui.Label("ui.Label(text).Classes(css.InlineCode)").Classes(css.InlineCode), widget.AlignStart),
			ui.Label("Enter your email address.").Classes(css.Muted),
		),
	).Spacing(40).Padding(render.UniformInsets(32)))).VerticalOnly(true).Classes(css.Background)
	var drawer *widget.Sheet
	g.openSheet = func() { sheet.SetOpen(true) }
	g.openDrawer = func() { drawer.SetOpen(true) }
	withDrawer := ui.Sheet(page, ui.Aligned(ui.Label("350 calories per day").Classes(css.H3), widget.AlignCenter), widget.SheetBottom).
		Header(g.env.Face, "Move Goal", "Set your daily activity goal.").
		Footer(ui.Button(ui.Label("Submit"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { drawer.SetOpen(false) })).
		Ref(&drawer)
	return ui.Mount(cx, g.env, ui.Sheet(withDrawer, ui.Field("Name", ui.Entry().Text("Pedro Duarte")), widget.SheetRight).
		Header(g.env.Face, "Edit profile", "Make changes to your profile here. Click save when you're done.").
		Footer(ui.Button(ui.Label("Save changes"), 0, 0).Classes(css.SuggestedAction).OnClick(func() { sheet.SetOpen(false) })).
		Ref(&sheet))
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
