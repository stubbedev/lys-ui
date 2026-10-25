package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/render"
)

// calFrame measures and arranges a calendar and returns it.
func calFrame(c *Calendar) *Calendar {
	sz := c.Measure(Constraints{Max: Size{W: 4096, H: 4096}})
	c.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	return c
}

// center is the middle of d's shown cell.
func center(t *testing.T, c *Calendar, d Date) Point {
	t.Helper()
	r, ok := c.dayRect(d)
	if !ok {
		t.Fatalf("%v is not shown", d)
	}
	return Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
}

func TestDateArithmetic(t *testing.T) {
	jan31 := Date{2024, time.January, 31}
	if got := jan31.AddMonths(1); got != (Date{2024, time.February, 29}) {
		t.Errorf("Jan 31 2024 + 1 month = %v, want the leap day", got)
	}
	if got := jan31.AddMonths(-2); got != (Date{2023, time.November, 30}) {
		t.Errorf("Jan 31 2024 - 2 months = %v", got)
	}
	if got := (Date{2025, time.December, 31}).AddDays(1); got != (Date{2026, time.January, 1}) {
		t.Errorf("new year = %v", got)
	}
	if !jan31.Before(Date{2024, time.February, 1}) || jan31.Compare(jan31) != 0 {
		t.Error("ordering")
	}
	if jan31.String() != "2024-01-31" {
		t.Errorf("String = %s", jan31)
	}
}

// TestCalendarMonthLayout pins the grid arithmetic: every day maps to
// its own cell and back, and the cells before day 1 hold the previous
// month's trailing days.
func TestCalendarMonthLayout(t *testing.T) {
	for _, m := range []Date{{2026, time.January, 1}, {2026, time.February, 1}, {2024, time.February, 1}, {2026, time.March, 1}} {
		c := calFrame(NewCalendar(entryFace(t), 13, m))
		days := daysIn(m.Year, m.Month)
		if c.firstOffset(m)+days > calendarCols*calendarRows {
			t.Errorf("%v overflows the six rows", m)
		}
		for d := 1; d <= days; d++ {
			day := Date{m.Year, m.Month, d}
			if got, ok := c.dayAt(center(t, c, day)); !ok || got != day {
				t.Errorf("%v round-trips to %v", day, got)
			}
		}
		if off := c.firstOffset(m); off > 0 {
			if got := c.cellDate(0, 0, 0); got != m.AddDays(-off) {
				t.Errorf("%v: the first cell holds %v, want %v", m, got, m.AddDays(-off))
			}
		}
	}
}

// TestCalendarSelectionAndNavigation pins single mode's keyboard: the
// arrows cross months and years with the view following, and
// PageUp/PageDown (Shift: a year) clamp the day.
func TestCalendarSelectionAndNavigation(t *testing.T) {
	var picked []Date
	c := NewCalendar(entryFace(t), 13, Date{2026, time.November, 30})
	c.OnSelect = func(d Date) { picked = append(picked, d) }
	calFrame(c)

	c.KeyAction(KeyRight, 0)
	if c.Selection() != (Date{2026, time.December, 1}) || c.Shown().Month != time.December {
		t.Fatalf("right from Nov 30 = %v showing %v", c.Selection(), c.Shown())
	}
	c.KeyAction(KeyLeft, 0)
	if c.Selection() != (Date{2026, time.November, 30}) || c.Shown().Month != time.November {
		t.Fatalf("back across the boundary = %v showing %v", c.Selection(), c.Shown())
	}
	c.Select(Date{2026, time.January, 3})
	c.KeyAction(KeyUp, 0)
	if c.Selection() != (Date{2025, time.December, 27}) || c.Shown().Year != 2025 {
		t.Fatalf("week up from Jan 3 = %v showing %v", c.Selection(), c.Shown())
	}
	c.Select(Date{2024, time.January, 31})
	c.KeyAction(KeyNextPage, 0)
	if c.Selection() != (Date{2024, time.February, 29}) {
		t.Fatalf("Jan 31 2024 paged = %v, want the leap day", c.Selection())
	}
	c.KeyAction(KeyNextPage, ModShift)
	if c.Selection() != (Date{2025, time.February, 28}) {
		t.Fatalf("Feb 29 2024 plus a year = %v, want Feb 28 2025", c.Selection())
	}
	if len(picked) == 0 {
		t.Error("OnSelect never fired")
	}
}

// The navigation buttons change the shown month, not the selection.
func TestCalendarNavButtonsKeepTheSelection(t *testing.T) {
	sel := Date{2026, time.May, 15}
	c := calFrame(NewCalendar(entryFace(t), 13, sel))
	c.nav[1].OnClick()
	c.nav[1].OnClick()
	c.nav[0].OnClick()
	if c.Shown() != (Date{2026, time.June, 1}) || c.Selection() != sel {
		t.Errorf("showing %v with %v selected, want June with May 15", c.Shown(), c.Selection())
	}
}

// TestCalendarClickSelects pins the pointer path: a click on a day
// selects it once; a click off the grid changes nothing; a click on an
// outside day selects it and shows its month.
func TestCalendarClickSelects(t *testing.T) {
	var picked []Date
	c := calFrame(NewCalendar(entryFace(t), 13, Date{2026, time.May, 1}))
	c.OnSelect = func(d Date) { picked = append(picked, d) }
	c.ClickAt(center(t, c, Date{2026, time.May, 15}))
	if c.Selection() != (Date{2026, time.May, 15}) || len(picked) != 1 {
		t.Fatalf("click selected %v (notified %v)", c.Selection(), picked)
	}
	c.ClickAt(Point{X: c.bounds.X + c.bounds.W/2, Y: c.bounds.Y + 2})
	if len(picked) != 1 {
		t.Errorf("a title click selected a day: %v", picked)
	}
	// May 2026 begins on a Friday: the first cell is April 26.
	r := c.cellRect(0, 0, 0)
	c.ClickAt(Point{X: r.X + 4, Y: r.Y + 4})
	if c.Selection() != (Date{2026, time.April, 26}) || c.Shown().Month != time.April {
		t.Errorf("an outside day picked %v showing %v", c.Selection(), c.Shown())
	}
}

// Range mode: the first pick starts, the second ends (the earlier
// starting), a third starts anew; the keyboard cursor picks on Enter.
func TestCalendarRange(t *testing.T) {
	var ranges [][2]Date
	c := NewCalendar(entryFace(t), 13, Date{2026, time.May, 1})
	c.SetMode(CalendarRange)
	c.SetMonths(2)
	c.OnRangeSelect = func(from, to Date) { ranges = append(ranges, [2]Date{from, to}) }
	calFrame(c)
	c.ClickAt(center(t, c, Date{2026, time.June, 10}))
	c.ClickAt(center(t, c, Date{2026, time.May, 20}))
	if from, to := c.Range(); from != (Date{2026, time.May, 20}) || to != (Date{2026, time.June, 10}) {
		t.Fatalf("range %v-%v, want May 20 to June 10", from, to)
	}
	if !c.inRange(Date{2026, time.June, 1}) || c.inRange(Date{2026, time.June, 10}) {
		t.Error("the band does not cover the days between the ends")
	}
	c.ClickAt(center(t, c, Date{2026, time.May, 3}))
	if from, to := c.Range(); from != (Date{2026, time.May, 3}) || !to.IsZero() {
		t.Fatalf("a third pick gave %v-%v, want a fresh start", from, to)
	}
	c.KeyAction(KeyRight, 0)
	c.KeyAction(KeyRight, 0)
	c.KeyAction(KeyEnter, 0)
	if _, to := c.Range(); to != (Date{2026, time.May, 5}) || len(ranges) != 4 {
		t.Errorf("Enter on the cursor ended the range at %v after %d picks", to, len(ranges))
	}
}

// mondayEN starts weeks on Monday.
type mondayEN struct{ CalendarENUS }

func (mondayEN) FirstWeekday() time.Weekday { return time.Monday }

// TestCalendarNamesSwap pins the locale seam: a custom CalendarNames
// changes the first-weekday offset; nil restores en-US.
func TestCalendarNamesSwap(t *testing.T) {
	may := Date{2026, time.May, 1}
	c := NewCalendar(entryFace(t), 13, may)
	if got := c.firstOffset(may); got != 5 {
		t.Fatalf("en-US offset = %d, want 5 (Friday, Sunday start)", got)
	}
	c.SetNames(mondayEN{})
	if got := c.firstOffset(may); got != 4 {
		t.Fatalf("Monday-first offset = %d, want 4", got)
	}
	c.SetNames(nil)
	if got := c.firstOffset(may); got != 5 {
		t.Fatalf("nil names did not restore en-US: offset %d", got)
	}
}

// TestGoldenShadcnCalendar pins a single calendar and a two-month
// range, in both palettes.
func TestGoldenShadcnCalendar(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			single := NewCalendar(face, 14, Date{2026, time.September, 14})
			single.SetToday(Date{2026, time.September, 3})
			rng := NewCalendar(face, 14, Date{})
			rng.SetToday(Date{2026, time.September, 3})
			rng.SetMode(CalendarRange)
			rng.SetMonths(2)
			rng.SetRange(Date{2026, time.September, 24}, Date{2026, time.October, 6})
			col := NewBox(Column, 24, 0).Append(single, false).Append(rng, false)
			NewGolden(t, col, "shadcn-calendar-"+tc.name, goldenTheme(tc.th))
		})
	}
}

// SetFirstWeekday overrides the name set's first column, and the
// weekend days take the `calendar .weekend` rule, wherever SetWeekend
// puts them (#264).
func TestCalendarWeekStartAndWeekend(t *testing.T) {
	face := goldenFace(t)
	c := NewCalendar(face, 14, DateOf(time.Date(2026, time.October, 14, 0, 0, 0, 0, time.UTC)))
	c.SetFirstWeekday(time.Monday)
	if got := c.cellDate(0, 0, 0).Weekday(); got != time.Monday {
		t.Errorf("first column is %v, want Monday", got)
	}
	loadCSS(t, `calendar .weekend { background-color: #ff0000; }`)
	buf := paintPixels(t, c, 300, 360)
	red := func(d Date) bool {
		for row := range calendarRows {
			for col := range calendarCols {
				if c.cellDate(0, col, row) == d {
					r := c.cellRect(0, col, row)
					return pxAt(buf, 300, r.X+2, r.Y+2) == render.RGB(0xff, 0, 0)
				}
			}
		}
		t.Fatalf("%v is not shown", d)
		return false
	}
	sat := DateOf(time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC))
	fri := DateOf(time.Date(2026, time.October, 16, 0, 0, 0, 0, time.UTC))
	if !red(sat) || red(fri) {
		t.Errorf("default weekend: Saturday tinted %v, Friday %v", red(sat), red(fri))
	}
	c.SetWeekend(time.Friday)
	buf = paintPixels(t, c, 300, 360)
	if red(sat) || !red(fri) {
		t.Errorf("Friday weekend: Saturday tinted %v, Friday %v", red(sat), red(fri))
	}
}

// Selecting the zero date clears the selection without moving the
// calendar off the shown month.
func TestCalendarSelectZeroClears(t *testing.T) {
	day := DateOf(time.Date(2026, time.October, 14, 0, 0, 0, 0, time.UTC))
	c := NewCalendar(goldenFace(t), 14, day)
	c.Select(Date{})
	if !c.Selection().IsZero() || c.Shown() != day.FirstOfMonth() || c.cursor != day {
		t.Errorf("after clearing: selection %v, shown %v, cursor %v; want none, October 2026, the 14th", c.Selection(), c.Shown(), c.cursor)
	}
}
