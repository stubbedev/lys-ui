package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// calendarIn is the Calendar in the popover a picker opened.
func calendarIn(t *testing.T, host *fakeSurfaceHost) *Calendar {
	t.Helper()
	var cal *Calendar
	walkTree(host.popovers[len(host.popovers)-1], 0, func(w Widget, _ int) {
		if c, ok := w.(*Calendar); ok && cal == nil {
			cal = c
		}
	})
	if cal == nil {
		t.Fatal("the popover holds no calendar")
	}
	return cal
}

func newTestPicker(t *testing.T) (*DatePicker, *fakeSurfaceHost) {
	t.Helper()
	p := NewDatePicker(goldenFace(t), "Pick a date")
	p.SetNames(CalendarENUS{})
	host := &fakeSurfaceHost{}
	SetSurfaceHost(p, host)
	arrangeTree(t, p, 300, 40)
	return p, host
}

// A single-date picker shows its pick formatted and closes; with a
// time input it stays open and shows the time after the date.
func TestDatePickerSingle(t *testing.T) {
	p, host := newTestPicker(t)
	var got []Date
	p.OnDate = func(d Date) { got = append(got, d) }
	if p.text.Text() != "Pick a date" {
		t.Fatalf("trigger reads %q", p.text.Text())
	}
	p.trigger.OnClick()
	calendarIn(t, host).Select(Date{2026, time.October, 24})
	if p.text.Text() != "October 24, 2026" || host.closed != 1 || len(got) != 1 {
		t.Fatalf("trigger %q after %d closes and %d picks", p.text.Text(), host.closed, len(got))
	}
	p.SetTimeInput(true)
	p.SetTime(TimeOfDay{9, 5})
	p.trigger.OnClick()
	calendarIn(t, host).Select(Date{2026, time.October, 25})
	if p.text.Text() != "October 25, 2026 09:05" || host.closed != 1 {
		t.Errorf("with time: trigger %q after %d closes", p.text.Text(), host.closed)
	}
}

// A range picker closes once both ends are picked; a preset applies at
// once, ordered.
func TestDatePickerRange(t *testing.T) {
	p, host := newTestPicker(t)
	p.SetMode(CalendarRange)
	var ranges [][2]Date
	p.OnRange = func(from, to Date) { ranges = append(ranges, [2]Date{from, to}) }
	p.trigger.OnClick()
	cal := calendarIn(t, host)
	if cal.months != 2 {
		t.Fatalf("the range calendar shows %d months", cal.months)
	}
	cal.pick(Date{2026, time.October, 6})
	if host.closed != 0 {
		t.Fatal("the picker closed on the first end")
	}
	cal.pick(Date{2026, time.September, 24})
	if host.closed != 1 || p.text.Text() != "September 24, 2026 - October 6, 2026" || len(ranges) != 1 {
		t.Fatalf("trigger %q after %d closes, ranges %v", p.text.Text(), host.closed, ranges)
	}
	p.SetPresets(DatePreset{Label: "Last week", From: Date{2026, time.October, 20}, To: Date{2026, time.October, 13}})
	p.trigger.OnClick()
	p.pickPreset(p.presets[0])
	if from, to := p.Range(); from != (Date{2026, time.October, 13}) || to != (Date{2026, time.October, 20}) {
		t.Errorf("preset gave %v-%v", from, to)
	}
}

// germanCatalog translates the calendar's strings to German.
type germanCatalog struct{}

func (germanCatalog) Get(id string) string {
	if id == "calendar:week_start:0" {
		return "calendar:week_start:1"
	}
	return id
}

func (germanCatalog) GetN(s, p string, n int) string { return s }

func (germanCatalog) GetCtx(ctx, id string) string {
	switch {
	case ctx == "month" && id == "October":
		return "Oktober"
	case ctx == "date format":
		return "%-d. %B %Y"
	}
	return id
}

// The default names read the message catalog: month names, the week's
// first day, and the date layout.
func TestCalendarCatalogLocalizes(t *testing.T) {
	SetMessageCatalog(germanCatalog{})
	defer SetMessageCatalog(nil)
	n := CalendarCatalog{}
	if got := n.FormatDate(Date{2026, time.October, 4}); got != "4. Oktober 2026" {
		t.Errorf("FormatDate = %q, want 4. Oktober 2026", got)
	}
	if n.FirstWeekday() != time.Monday {
		t.Error("the week does not start on Monday")
	}
	SetMessageCatalog(nil)
	if got := n.FormatDate(Date{2026, time.October, 4}); got != "October 4, 2026" {
		t.Errorf("English FormatDate = %q", got)
	}
}

func TestDatePickerRejectsBadTimes(t *testing.T) {
	p, _ := newTestPicker(t)
	defer func() {
		if recover() == nil {
			t.Error("24:00 did not panic")
		}
	}()
	p.SetTime(TimeOfDay{24, 0})
}

// TestGoldenDatePicker pins the trigger empty and picked, and the
// open range popover with presets and two months, in both palettes.
func TestGoldenDatePicker(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			empty, _ := newTestPicker(t)
			picked, _ := newTestPicker(t)
			picked.SetDate(Date{2026, time.October, 24})
			rng, host := newTestPicker(t)
			rng.SetMode(CalendarRange)
			rng.SetRange(Date{2026, time.September, 24}, Date{2026, time.October, 6})
			rng.SetPresets(DatePreset{Label: "Today"}, DatePreset{Label: "Last 7 days"}, DatePreset{Label: "Last 30 days"})
			rng.trigger.OnClick()
			cal := calendarIn(t, host)
			cal.SetToday(Date{2026, time.October, 2})
			col := NewBox(Column, 12, 0)
			col.Append(empty, false).Append(picked, false).Append(rng, false).Append(host.popovers[0], false)
			NewGolden(t, col, "datepicker-"+tc.name, goldenTheme(tc.th))
		})
	}
}

// The trigger's padding rule reaches only the trigger: the popover is
// style-anchored under the picker, and `datepicker button` padded the
// calendar's month buttons too, squeezing their chevrons to specks
// (#281).
func TestDatePickerTriggerRuleSparesThePopover(t *testing.T) {
	p, host := newTestPicker(t)
	p.trigger.OnClick()
	content := host.popovers[len(host.popovers)-1]
	SetStyleAnchor(content, p)
	arrangeTree(t, content, 320, 400)
	if pad := boxOf(p.trigger.style(p.trigger), render.Insets{}).padding; pad.Left != 12 || pad.Right != 12 {
		t.Errorf("trigger padding %+v, want 12 each side", pad)
	}
	nav := calendarIn(t, host).nav[0]
	if pad := boxOf(nav.style(nav), render.Insets{}).padding; pad.Left != 0 || pad.Right != 0 {
		t.Errorf("month button padding %+v, want none", pad)
	}
}
