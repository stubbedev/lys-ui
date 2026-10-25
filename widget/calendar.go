package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// calendarCols and calendarRows are a month's fixed grid: seven
// weekday columns and six week rows (a month whose first day lands
// late spans six weeks), so the calendar never changes height.
const (
	calendarCols = 7
	calendarRows = 6
	// calendarRowGap is the space between the week rows.
	calendarRowGap = 8
	// calendarMonthGap separates months shown side by side.
	calendarMonthGap = 16
)

// CalendarNames supplies a Calendar's localized pieces. A nil
// interface (the default) reads them through the message catalog
// (CalendarCatalog); supply your own to localize otherwise.
type CalendarNames interface {
	// MonthName is the full month name for the title.
	MonthName(time.Month) string
	// WeekdayName is the short weekday label for the header.
	WeekdayName(time.Weekday) string
	// FirstWeekday is the week's first column (Sunday for en-US).
	FirstWeekday() time.Weekday
	// FormatDate writes a day the way the locale does: "October 24,
	// 2026" in en-US.
	FormatDate(Date) string
}

// CalendarENUS is the en-US name set: full English month names,
// two-letter weekday abbreviations, weeks starting Sunday, dates as
// "October 24, 2026".
type CalendarENUS struct{}

// FormatDate implements CalendarNames.
func (n CalendarENUS) FormatDate(d Date) string { return formatDate(n, enUSDateLayout, d) }

// enUSDateLayout is the en-US date layout (see formatDate).
const enUSDateLayout = "%B %-d, %Y"

// CalendarCatalog is the default name set, read through the installed
// message catalog (SetCatalog) the GTK way, English without one: the
// month and weekday names translate as themselves, the week's first
// day is the translation of "calendar:week_start:0" (a translator
// writes calendar:week_start:1 for Monday), and dates follow the
// translation of the layout "%B %-d, %Y" (see formatDate).
type CalendarCatalog struct{}

// MonthName implements CalendarNames.
func (CalendarCatalog) MonthName(m time.Month) string { return TrCtx("month", m.String()) }

// WeekdayName implements CalendarNames.
func (CalendarCatalog) WeekdayName(w time.Weekday) string {
	return TrCtx("weekday", w.String()[:2])
}

// FirstWeekday implements CalendarNames.
func (CalendarCatalog) FirstWeekday() time.Weekday {
	const key = "calendar:week_start:"
	v := Tr(key + "0")
	if len(v) == len(key)+1 && v[:len(key)] == key && v[len(key)] >= '0' && v[len(key)] <= '6' {
		return time.Weekday(v[len(key)] - '0')
	}
	return time.Sunday
}

// FormatDate implements CalendarNames.
func (n CalendarCatalog) FormatDate(d Date) string {
	return formatDate(n, TrCtx("date format", enUSDateLayout), d)
}

// formatDate writes d by layout: %B the month name, %Y the year, %d
// the day of the month (%-d unpadded), %m the month number; other
// text stays as it is.
func formatDate(names CalendarNames, layout string, d Date) string {
	var b []byte
	for i := 0; i < len(layout); i++ {
		if layout[i] != '%' || i+1 >= len(layout) {
			b = append(b, layout[i])
			continue
		}
		i++
		pad := true
		if layout[i] == '-' && i+1 < len(layout) {
			pad = false
			i++
		}
		switch layout[i] {
		case 'B':
			b = append(b, names.MonthName(d.Month)...)
		case 'Y':
			b = append(b, itoa(d.Year)...)
		case 'd':
			b = appendTwo(b, d.Day, pad)
		case 'm':
			b = appendTwo(b, int(d.Month), pad)
		default:
			b = append(b, '%', layout[i])
		}
	}
	return string(b)
}

// appendTwo appends v, zero-padded to two digits when pad.
func appendTwo(b []byte, v int, pad bool) []byte {
	if pad && v < 10 {
		b = append(b, '0')
	}
	return append(b, itoa(v)...)
}

// MonthName implements CalendarNames.
func (CalendarENUS) MonthName(m time.Month) string { return m.String() }

// WeekdayName implements CalendarNames.
func (CalendarENUS) WeekdayName(w time.Weekday) string { return w.String()[:2] }

// FirstWeekday implements CalendarNames.
func (CalendarENUS) FirstWeekday() time.Weekday { return time.Sunday }

// CalendarMode is what a Calendar's clicks pick.
type CalendarMode uint8

// The calendar modes.
const (
	// CalendarSingle picks one day.
	CalendarSingle CalendarMode = iota
	// CalendarRange picks a span: the first pick starts it, the second
	// ends it (the earlier of the two starting), a third starts anew.
	CalendarRange
)

// Calendar is a month grid: the month title
// between previous and next buttons, the weekday header, and the days
// with the adjacent months' leading and trailing days muted. Today
// takes the muted wash, a picked day the primary fill, and a range's
// days between its ends a band. SetMonths shows several months side by
// side, the date range picker's two. Colors come from the typed theme.
//
// Pointer: a click picks its day; the buttons step the shown months.
// Keyboard: in single mode the arrows move the selection a day (a week
// vertically) and PageUp/PageDown a month (Shift: a year), the view
// following; in range mode they move a cursor and Enter or Space picks
// it.
type Calendar struct {
	node
	face   render.Font
	sizePx float64
	names  CalendarNames
	mode   CalendarMode
	months int

	view  Date // first day of the first shown month
	sel   Date // single mode's pick
	from  Date // range mode's start
	to    Date // range mode's end; zero while only the start is picked
	today Date
	// cursor is the keyboard's day: the selection in single mode, the
	// day Enter picks in range mode.
	cursor Date

	// nav holds the previous and next month buttons.
	nav [2]*Button

	// firstDay overrides the name set's first weekday when set;
	// weekend marks the days the `.weekend` style part tints, and
	// weekendPart is that part.
	firstDay    time.Weekday
	firstDaySet bool
	weekend     [7]bool
	weekendPart stylePart

	cell    int
	titleH  int
	headerH int

	// marks are the marked days (MarkDay); hover is the day under the
	// pointer, zero when none.
	marks map[Date]bool
	hover Date

	// Detail, when set, describes a date - an event title, a holiday -
	// shown as the tooltip of the day under the pointer ("" for none).
	Detail func(date Date) string

	// OnSelect fires with the picked day in single mode, on every
	// applied selection - click, keyboard, or Select.
	OnSelect func(Date)
	// OnRangeSelect fires in range mode on every pick: the start and,
	// once picked, the end (zero until then).
	OnRangeSelect func(from, to Date)
}

// NewCalendar returns a single-mode calendar showing initial's month
// with initial selected; a zero initial shows today's month with no
// selection. Today is read from the clock now (SetToday overrides).
func NewCalendar(face render.Font, sizePx float64, initial Date) *Calendar {
	face = requireFace("widget.NewCalendar", face)
	c := &Calendar{face: face, sizePx: sizePx, months: 1, today: Today(), sel: initial}
	shown := initial
	if shown.IsZero() {
		shown = c.today
	}
	c.view, c.cursor = shown.FirstOfMonth(), shown
	lineH := face.Shape("lg", sizePx).LineHeight()
	c.cell = max(32, lineH+12)
	c.titleH = c.cell + 8
	c.headerH = lineH + 8
	for i, kind := range [2]SymbolKind{SymbolChevronLeft, SymbolChevronRight} {
		b := NewButton(NewSymbol(kind, 20), 0, 0)
		b.AddClass("nav")
		b.SetSizeRequest(c.cell-4, c.cell-4)
		c.nav[i] = b
	}
	c.nav[0].SetAccessibleName("Previous month")
	c.nav[1].SetAccessibleName("Next month")
	c.nav[0].OnClick = func() { c.showMonth(c.view.AddMonths(-1)) }
	c.nav[1].OnClick = func() { c.showMonth(c.view.AddMonths(1)) }
	c.SetElement("calendar")
	c.weekend[time.Saturday], c.weekend[time.Sunday] = true, true
	c.weekendPart.AddClass("weekend")
	setParents(c, &c.weekendPart)
	return c
}

// SetNames swaps the locale's names; nil restores CalendarCatalog.
func (c *Calendar) SetNames(names CalendarNames) {
	c.names = names
	c.Invalidate()
}

// SetFirstWeekday makes day the week's first column, overriding the
// name set's (lys-shell's calendar takes it from its config).
func (c *Calendar) SetFirstWeekday(day time.Weekday) {
	c.firstDay, c.firstDaySet = day, true
	c.Invalidate()
}

// firstWeekday is the week's first column: SetFirstWeekday's day, else
// the name set's.
func (c *Calendar) firstWeekday() time.Weekday {
	if c.firstDaySet {
		return c.firstDay
	}
	return c.namesOf().FirstWeekday()
}

// SetWeekend names the weekend days, Saturday and Sunday by default:
// their cells take the `calendar .weekend` rule's color and
// background, which the theme leaves unset.
func (c *Calendar) SetWeekend(days ...time.Weekday) {
	c.weekend = [7]bool{}
	for _, d := range days {
		c.weekend[d] = true
	}
	c.Invalidate()
}

// appendStyleKids is the navigation buttons and the weekend part.
func (c *Calendar) appendStyleKids(buf []Widget) []Widget {
	return append(buf, c.nav[0], c.nav[1], &c.weekendPart)
}

// namesOf returns the active name set.
func (c *Calendar) namesOf() CalendarNames {
	if c.names != nil {
		return c.names
	}
	return CalendarCatalog{}
}

// SetMode switches between picking a day and a range; the picks of
// the other mode clear.
func (c *Calendar) SetMode(m CalendarMode) {
	c.mode = m
	c.sel, c.from, c.to = Date{}, Date{}, Date{}
	c.Invalidate()
}

// Mode returns what clicks pick.
func (c *Calendar) Mode() CalendarMode { return c.mode }

// SetMonths shows n months side by side from the shown one, at least
// one.
func (c *Calendar) SetMonths(n int) {
	c.months = max(1, n)
	c.InvalidateLayout()
}

// SetToday sets the day marked as today (NewCalendar reads the clock).
func (c *Calendar) SetToday(d Date) {
	c.today = d
	c.Invalidate()
}

// ShowMonth shows the month of d first, leaving the picks alone.
func (c *Calendar) ShowMonth(d Date) { c.showMonth(d) }

// showMonth moves the view to d's month.
func (c *Calendar) showMonth(d Date) {
	c.view = d.FirstOfMonth()
	c.Invalidate()
}

// Shown returns the first day of the first shown month.
func (c *Calendar) Shown() Date { return c.view }

// MarkDay marks d (GTK mark_day): a dot under its number. Marks are
// per date, so they stay with their month as the view moves.
func (c *Calendar) MarkDay(d Date) {
	if c.marks == nil {
		c.marks = map[Date]bool{}
	}
	c.marks[d] = true
	c.Invalidate()
}

// UnmarkDay removes d's mark.
func (c *Calendar) UnmarkDay(d Date) {
	delete(c.marks, d)
	c.Invalidate()
}

// ClearMarks removes every mark.
func (c *Calendar) ClearMarks() {
	clear(c.marks)
	c.Invalidate()
}

// DayMarked reports d's mark.
func (c *Calendar) DayMarked(d Date) bool { return c.marks[d] }

// Selection returns single mode's selected day, zero when none.
func (c *Calendar) Selection() Date { return c.sel }

// Select applies d as single mode's selection, shows its month if it
// is not shown, and fires OnSelect. A zero d clears the selection,
// keeping the shown month and the keyboard's day.
func (c *Calendar) Select(d Date) {
	c.sel = d
	if !d.IsZero() {
		c.cursor = d
		c.reveal(d)
	}
	c.Invalidate()
	if c.OnSelect != nil {
		c.OnSelect(d)
	}
}

// Range returns range mode's start and end (zero while unpicked).
func (c *Calendar) Range() (from, to Date) { return c.from, c.to }

// SetRange applies a range, the earlier day starting it, and shows its
// start's month; it fires nothing.
func (c *Calendar) SetRange(from, to Date) {
	if !to.IsZero() && to.Before(from) {
		from, to = to, from
	}
	c.from, c.to = from, to
	if !from.IsZero() {
		c.cursor = from
		c.reveal(from)
	}
	c.Invalidate()
}

// pick applies a click or Enter on d in the current mode.
func (c *Calendar) pick(d Date) {
	if c.mode == CalendarSingle {
		c.Select(d)
		return
	}
	switch {
	case c.from.IsZero() || !c.to.IsZero():
		c.from, c.to = d, Date{}
	case d.Before(c.from):
		c.from, c.to = d, c.from
	default:
		c.to = d
	}
	c.cursor = d
	c.reveal(d)
	c.Invalidate()
	if c.OnRangeSelect != nil {
		c.OnRangeSelect(c.from, c.to)
	}
}

// reveal shows d's month when none of the shown months is it.
func (c *Calendar) reveal(d Date) {
	first := c.view
	last := c.view.AddMonths(c.months - 1)
	if d.FirstOfMonth().Before(first) || last.Before(d.FirstOfMonth()) {
		c.view = d.FirstOfMonth()
	}
}

// firstOffset is the number of the previous month's days before day 1
// of month m in the header's week order.
func (c *Calendar) firstOffset(m Date) int {
	return (int(m.Weekday()-c.firstWeekday()) + 7) % 7
}

// monthWidth is one month's grid width.
func (c *Calendar) monthWidth() int { return calendarCols * c.cell }

// monthOrigin is the top-left of shown month i.
func (c *Calendar) monthOrigin(i int) (int, int) {
	return c.bounds.X + i*(c.monthWidth()+calendarMonthGap), c.bounds.Y
}

// dayGrid is the top-left of month i's day cells.
func (c *Calendar) dayGrid(i int) (int, int) {
	x, y := c.monthOrigin(i)
	return x, y + c.titleH + c.headerH
}

// cellDate is the day shown in month i's cell at column col, row row:
// the month's own days, or the adjacent months' outside them.
func (c *Calendar) cellDate(i, col, row int) Date {
	m := c.view.AddMonths(i)
	return m.AddDays(row*calendarCols + col - c.firstOffset(m))
}

// cellRect is month i's cell at (col, row).
func (c *Calendar) cellRect(i, col, row int) render.Rect {
	x, y := c.dayGrid(i)
	return render.Rect{X: x + col*c.cell, Y: y + row*c.rowStride(), W: c.cell, H: c.cell}
}

// rowStride is a week row's height and the gap below it.
func (c *Calendar) rowStride() int { return c.cell + calendarRowGap }

// gridHeight is the day grid's height: the week rows and the gaps
// between them.
func (c *Calendar) gridHeight() int { return calendarRows*c.rowStride() - calendarRowGap }

// dayRect is d's cell in the shown month holding it, and whether one
// does.
func (c *Calendar) dayRect(d Date) (render.Rect, bool) {
	for i := range c.months {
		m := c.view.AddMonths(i)
		if d.Year != m.Year || d.Month != m.Month {
			continue
		}
		at := d.Day - 1 + c.firstOffset(m)
		return c.cellRect(i, at%calendarCols, at/calendarCols), true
	}
	return render.Rect{}, false
}

// Measure wants each month's title, header and six week rows, side by
// side, clamped to con.
func (c *Calendar) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	w := c.months*c.monthWidth() + (c.months-1)*calendarMonthGap
	h := c.titleH + c.headerH + c.gridHeight()
	return c.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Arrange records the rect and places the previous button at the
// first month's left and the next at the last month's right.
func (c *Calendar) Arrange(r render.Rect) {
	c.node.Arrange(r)
	side := c.cell - 4
	y := r.Y + (c.titleH-side)/2
	c.nav[0].Arrange(render.Rect{X: r.X, Y: y, W: side, H: side})
	lastX, _ := c.monthOrigin(c.months - 1)
	c.nav[1].Arrange(render.Rect{X: lastX + c.monthWidth() - side, Y: y, W: side, H: side})
	setParents(c, c.nav[0], c.nav[1])
}

// Children exposes the navigation buttons for focus traversal and the
// damage walk; the day cells are painted geometry, not widgets.
func (c *Calendar) Children() []Widget { return []Widget{c.nav[0], c.nav[1]} }

// appendChildren appends the navigation buttons, matching Children.
func (c *Calendar) appendChildren(buf []Widget) []Widget { return append(buf, c.nav[0], c.nav[1]) }

// inRange reports d strictly between a picked range's ends.
func (c *Calendar) inRange(d Date) bool {
	return !c.to.IsZero() && c.from.Before(d) && d.Before(c.to)
}

// picked reports d as a picked day: single mode's selection or a
// range's end.
func (c *Calendar) picked(d Date) bool {
	if c.mode == CalendarSingle {
		return !c.sel.IsZero() && d == c.sel
	}
	return !d.IsZero() && (d == c.from || d == c.to)
}

// Paint draws each month's title, weekday header and days, then the
// navigation buttons.
func (c *Calendar) Paint(cv *render.Canvas) {
	th := Current()
	names := c.namesOf()
	radius := min(6, c.cell/4)
	small := c.sizePx * 0.85
	for i := range c.months {
		m := c.view.AddMonths(i)
		x, y := c.monthOrigin(i)
		title := names.MonthName(m.Month) + " " + itoa(m.Year)
		c.face.DrawAligned(cv, title, render.Rect{X: x, Y: y, W: c.monthWidth(), H: c.titleH}, c.sizePx, th.Text, render.AlignCenter)
		gx, _ := c.dayGrid(i)
		start := c.firstWeekday()
		for col := range calendarCols {
			wd := time.Weekday((int(start) + col) % 7)
			c.face.DrawAligned(cv, names.WeekdayName(wd), render.Rect{X: gx + col*c.cell, Y: y + c.titleH, W: c.cell, H: c.headerH}, small, th.TextMuted, render.AlignCenter)
		}
		for row := range calendarRows {
			for col := range calendarCols {
				c.paintDay(cv, m, c.cellDate(i, col, row), c.cellRect(i, col, row), radius)
			}
		}
	}
	for _, b := range c.nav {
		PaintChild(cv, b)
	}
}

// paintDay draws one cell of month m: the range band, the picked or
// today fill, the number, its mark, and the keyboard cursor.
func (c *Calendar) paintDay(cv *render.Canvas, m, d Date, cell render.Rect, radius int) {
	th := Current()
	outside := d.Month != m.Month
	picked := !outside && c.picked(d)
	col := th.Text
	if c.weekend[d.Weekday()] && !outside {
		wv := c.weekendPart.style(&c.weekendPart)
		col = pickc(0, wv, style.PropColor, col)
		if bg := pickc(0, wv, style.PropBackgroundColor, 0); bg != 0 {
			cv.FillRect(cell, bg)
		}
	}
	switch {
	case outside:
		col = th.TextMuted
	case picked:
		col = th.OnAccent
	}
	if !outside {
		switch {
		case c.inRange(d):
			cv.FillRect(cell, th.MutedColor())
		case c.mode == CalendarRange && !c.to.IsZero() && c.from != c.to && d == c.from:
			band := cell
			band.X += cell.W / 2
			band.W -= cell.W / 2
			cv.FillRect(band, th.MutedColor())
		case c.mode == CalendarRange && !c.to.IsZero() && c.from != c.to && d == c.to:
			band := cell
			band.W = cell.W / 2
			cv.FillRect(band, th.MutedColor())
		}
		switch {
		case picked:
			cv.RoundedRect(cell, radius, th.Accent)
		case d == c.today && !c.inRange(d):
			cv.RoundedRect(cell, radius, th.MutedColor())
		}
	}
	c.face.DrawAligned(cv, dayNumber(d.Day), cell, c.sizePx, col, render.AlignCenter)
	if !outside && c.marks[d] {
		dot := render.Rect{X: cell.X + cell.W/2 - 2, Y: cell.Y + cell.H - 7, W: 4, H: 4}
		cv.RoundedRect(dot, 2, col)
	}
	if !outside && d == c.cursor && c.focused && c.focusVisible {
		cv.RoundedBorder(cell, render.UniformCorners(radius), render.UniformInsets(2), th.RingColor())
	}
}

// dayNumbers holds "1".."31": the day grid shapes the same strings
// every frame, and a fresh conversion per cell per frame was 31
// allocations on the calendar's paint path.
var dayNumbers = [...]string{
	"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
	"11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
	"21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31",
}

func dayNumber(d int) string { return dayNumbers[d-1] }

// itoa renders a small integer without fmt.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Role implements Roleer.
func (c *Calendar) Role() Role { return RoleCalendar }

// HitTest returns a navigation button under p, else the calendar (the
// day cells are its own geometry).
func (c *Calendar) HitTest(p Point) Widget {
	for _, b := range c.nav {
		if b.HitTest(p) != nil {
			return b
		}
	}
	return c.HitLeaf(c, p)
}

// dayAt maps p to the day of the cell under it, and whether p is on a
// cell.
func (c *Calendar) dayAt(p Point) (Date, bool) {
	for i := range c.months {
		gx, gy := c.dayGrid(i)
		if p.X < gx || p.Y < gy || p.X >= gx+c.monthWidth() || p.Y >= gy+c.gridHeight() || (p.Y-gy)%c.rowStride() >= c.cell {
			continue
		}
		return c.cellDate(i, (p.X-gx)/c.cell, (p.Y-gy)/c.rowStride()), true
	}
	return Date{}, false
}

// ClickAt picks the day under p; clicks off the day grid change
// nothing.
func (c *Calendar) ClickAt(p Point) {
	if !IsEnabled(c) {
		return
	}
	if d, ok := c.dayAt(p); ok {
		c.pick(d)
	}
}

// HoverMove tracks the day under the pointer for its Detail tooltip.
func (c *Calendar) HoverMove(p Point) {
	c.hover, _ = c.dayAt(p)
}

// SetHovered forgets the day when the pointer leaves.
func (c *Calendar) SetHovered(on bool) {
	if !on {
		c.hover = Date{}
	}
}

// TooltipText is the hovered day's Detail, else the calendar's own
// tooltip.
func (c *Calendar) TooltipText() string {
	if c.Detail != nil && !c.hover.IsZero() {
		if s := c.Detail(c.hover); s != "" {
			return s
		}
	}
	return c.node.TooltipText()
}

// KeyAction implements KeyActionHandler: the arrows move a day (a week
// vertically) and PageUp/PageDown a month (with Shift, a year) - the
// selection itself in single mode, the cursor in range mode, where
// Enter and Space pick it. The view follows. Disabled calendars ignore
// keys.
func (c *Calendar) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(c) {
		return
	}
	cur := c.cursor
	if cur.IsZero() {
		cur = c.view
	}
	months := 1
	if mods&ModShift != 0 {
		months = 12
	}
	var next Date
	switch a {
	case KeyLeft:
		next = cur.AddDays(-1)
	case KeyRight:
		next = cur.AddDays(1)
	case KeyUp:
		next = cur.AddDays(-7)
	case KeyDown:
		next = cur.AddDays(7)
	case KeyPriorPage:
		next = cur.AddMonths(-months)
	case KeyNextPage:
		next = cur.AddMonths(months)
	case KeyEnter, KeySpace:
		if c.mode == CalendarRange {
			c.pick(cur)
		}
		return
	default:
		return
	}
	if c.mode == CalendarSingle {
		c.Select(next)
		return
	}
	c.cursor = next
	c.reveal(next)
	c.Invalidate()
}
