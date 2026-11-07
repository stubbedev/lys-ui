package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// TestCalendarMarksAndDetail pins marks as dates (they survive a view
// change and stay with their month) and the per-day detail tooltip
// following the hovered day.
func TestCalendarMarksAndDetail(t *testing.T) {
	face := chromeFace(t)
	day := func(d int) Date { return Date{2026, time.September, d} }
	c := NewCalendar(face, 13, day(14))
	c.MarkDay(day(3))
	c.MarkDay(day(20))
	c.UnmarkDay(day(20))
	if !c.DayMarked(day(3)) || c.DayMarked(day(20)) || c.DayMarked(day(3).AddMonths(1)) {
		t.Error("marks are not per date")
	}
	c.Detail = func(d Date) string {
		if d.Day == 14 {
			return "Release"
		}
		return ""
	}
	c.SetTooltip("calendar")
	SizeWithin(c, Size{W: 400, H: 400})
	c.Arrange(render.Rect{W: 400, H: 400})
	c.HoverMove(center(t, c, day(14)))
	if got := c.TooltipText(); got != "Release" {
		t.Errorf("tooltip over the 14th = %q", got)
	}
	c.HoverMove(center(t, c, day(15)))
	if got := c.TooltipText(); got != "calendar" {
		t.Errorf("tooltip over a day without detail = %q, want the calendar's own", got)
	}
	c.ClearMarks()
	if c.DayMarked(day(3)) {
		t.Error("ClearMarks kept a mark")
	}
}

// TestGoldenCalendarMarks pins the mark dots, on a plain day and on
// the selected one.
func TestGoldenCalendarMarks(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	day := func(d int) Date { return Date{2026, time.September, d} }
	cal := NewCalendar(face, 13, day(14))
	cal.SetToday(day(3))
	for _, d := range []int{8, 14, 22} {
		cal.MarkDay(day(d))
	}
	NewGolden(t, cal, "calendar-marks", goldenTheme(th))
}

func TestCalendarOnMonthFollowsTheView(t *testing.T) {
	day := func(m time.Month, d int) Date { return Date{2026, m, d} }
	c := NewCalendar(chromeFace(t), 13, day(time.September, 14))
	var months []Date
	c.OnMonth = func(first Date) { months = append(months, first) }

	c.nav[1].OnClick()
	c.nav[0].OnClick()
	c.ShowMonth(day(time.September, 30))
	c.Select(day(time.September, 2))
	c.Select(day(time.December, 24))
	c.KeyAction(KeyNextPage, 0)
	want := []Date{day(time.October, 1), day(time.September, 1), day(time.December, 1), {2027, time.January, 1}}
	if len(months) != len(want) {
		t.Fatalf("OnMonth fired with %v, want %v", months, want)
	}
	for i := range want {
		if months[i] != want[i] {
			t.Errorf("OnMonth %d = %v, want %v", i, months[i], want[i])
		}
	}
}
