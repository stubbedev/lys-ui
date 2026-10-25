package widget

import (
	"fmt"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// TimeOfDay is a wall-clock time: an hour 0-23 and a minute 0-59.
type TimeOfDay struct {
	Hour, Minute int
}

// String is the 24-hour time, 14:05.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }

// DatePreset is a ready-made pick beside a DatePicker's calendar:
// "Today", "Next week". A single-date picker picks From; a range picker
// picks From to To.
type DatePreset struct {
	Label    string
	From, To Date
}

// datePickerWidth is the trigger's width.
const datePickerWidth = 240

// DatePicker is a date field: an outline
// button showing the picked date - or range - formatted for the
// locale, that opens a Calendar in a popover. A single-date picker
// closes on a pick; a range picker shows two months and closes once
// the range has both ends. Presets list ready-made picks beside the
// calendar, and SetTimeInput adds an hour and minute under it. Dates
// are widget.Date and times TimeOfDay, so nothing depends on the local
// zone. It styles as the `datepicker` element: the button.outline
// trigger and its .placeholder text.
type DatePicker struct {
	composite
	face        render.Font
	trigger     *Button
	text        *Label
	placeholder string
	mode        CalendarMode
	date        Date
	from, to    Date
	presets     []DatePreset
	withTime    bool
	time        TimeOfDay
	names       CalendarNames
	close       func()

	// OnDate fires with a single-date picker's pick; OnRange with a
	// range picker's, once both ends are picked; OnTime with the time
	// the user set. None fires for the setters.
	OnDate  func(Date)
	OnRange func(from, to Date)
	OnTime  func(TimeOfDay)
}

// NewDatePicker returns a single-date picker showing placeholder until
// a date is picked.
func NewDatePicker(face render.Font, placeholder string) *DatePicker {
	face = requireFace("widget.NewDatePicker", face)
	p := &DatePicker{face: face, placeholder: placeholder}
	p.SetElement("datepicker")
	p.text = NewLabel(face, 14, "", 0)
	row := NewBox(Row, 8, 0)
	row.AppendAligned(NewSymbol(SymbolCalendar, 16), false, AlignCenter)
	row.AppendAligned(p.text, true, AlignCenter)
	p.trigger = NewButton(row, 0, 0)
	p.trigger.AddClass(css.Outline)
	p.trigger.SetSizeRequest(datePickerWidth, 0)
	p.trigger.OnClick = p.open
	p.initComposite(p, p.trigger)
	p.boxed = true
	p.sync()
	return p
}

// SetMode switches between picking a date and a range; the picks
// clear.
func (p *DatePicker) SetMode(m CalendarMode) {
	p.mode = m
	p.date, p.from, p.to = Date{}, Date{}, Date{}
	p.sync()
}

// SetNames localizes the calendar and the formatting; nil reads the
// message catalog (CalendarCatalog).
func (p *DatePicker) SetNames(names CalendarNames) {
	p.names = names
	p.sync()
}

// namesOf returns the active name set.
func (p *DatePicker) namesOf() CalendarNames {
	if p.names != nil {
		return p.names
	}
	return CalendarCatalog{}
}

// SetDate sets a single-date picker's date; zero clears it.
func (p *DatePicker) SetDate(d Date) {
	p.date = d
	p.sync()
}

// Date returns a single-date picker's date, zero when none.
func (p *DatePicker) Date() Date { return p.date }

// SetRange sets a range picker's range, the earlier day starting it.
func (p *DatePicker) SetRange(from, to Date) {
	if !to.IsZero() && to.Before(from) {
		from, to = to, from
	}
	p.from, p.to = from, to
	p.sync()
}

// Range returns a range picker's range.
func (p *DatePicker) Range() (from, to Date) { return p.from, p.to }

// SetPresets lists ready-made picks beside the calendar.
func (p *DatePicker) SetPresets(presets ...DatePreset) { p.presets = presets }

// SetTimeInput adds an hour and minute under the calendar, shown on
// the trigger after the date.
func (p *DatePicker) SetTimeInput(on bool) {
	p.withTime = on
	p.sync()
}

// SetTime sets the time; it must be a valid wall-clock time, or the
// call panics.
func (p *DatePicker) SetTime(t TimeOfDay) {
	if t.Hour < 0 || t.Hour > 23 || t.Minute < 0 || t.Minute > 59 {
		panic(fmt.Sprintf("widget: DatePicker.SetTime(%v): not a time of day", t))
	}
	p.time = t
	p.sync()
}

// Time returns the time.
func (p *DatePicker) Time() TimeOfDay { return p.time }

// label is the trigger's text: the date or range for the locale, the
// time after a date when the picker has one, else the placeholder.
func (p *DatePicker) label() string {
	names := p.namesOf()
	switch {
	case p.mode == CalendarSingle && !p.date.IsZero():
		if p.withTime {
			return names.FormatDate(p.date) + " " + p.time.String()
		}
		return names.FormatDate(p.date)
	case p.mode == CalendarRange && !p.from.IsZero() && !p.to.IsZero():
		return names.FormatDate(p.from) + " - " + names.FormatDate(p.to)
	case p.mode == CalendarRange && !p.from.IsZero():
		return names.FormatDate(p.from)
	}
	return ""
}

// sync shows the value on the trigger.
func (p *DatePicker) sync() {
	if text := p.label(); text != "" {
		p.text.SetText(text)
		p.text.RemoveClass("placeholder")
		return
	}
	p.text.SetText(p.placeholder)
	p.text.AddClass("placeholder")
}

// content builds the popover: the presets, and the calendar with the
// time input under it.
func (p *DatePicker) content() (Widget, *Calendar) {
	cal := NewCalendar(p.face, 14, Date{})
	cal.SetNames(p.names)
	switch p.mode {
	case CalendarSingle:
		if !p.date.IsZero() {
			cal.Select(p.date)
		}
		cal.OnSelect = p.pickDate
	case CalendarRange:
		cal.SetMode(CalendarRange)
		cal.SetMonths(2)
		cal.SetRange(p.from, p.to)
		cal.OnRangeSelect = p.pickRange
	}
	side := NewBox(Column, 8, 0)
	side.Append(cal, false)
	if p.withTime {
		side.Append(p.timeRow(), false)
	}
	body := NewBox(Row, 16, 12)
	if len(p.presets) > 0 {
		list := NewBox(Column, 2, 0)
		for _, pr := range p.presets {
			b := NewButton(NewLabel(p.face, 14, pr.Label, 0), 0, 0)
			b.AddClass(css.Ghost, css.Small)
			b.OnClick = func() { p.pickPreset(pr) }
			list.Append(b, false)
		}
		body.Append(list, false)
	}
	body.Append(side, false)
	return body, cal
}

// timeRow is the hour and minute input.
func (p *DatePicker) timeRow() Widget {
	hour := NewSpinButton(p.face, 14, 0, 0, 23, 1, 0)
	hour.SetValue(float64(p.time.Hour))
	minute := NewSpinButton(p.face, 14, 0, 0, 59, 1, 0)
	minute.SetValue(float64(p.time.Minute))
	set := func() {
		p.time = TimeOfDay{int(hour.Value()), int(minute.Value())}
		p.sync()
		if p.OnTime != nil {
			p.OnTime(p.time)
		}
	}
	hour.OnValueChanged = func(float64) { set() }
	minute.OnValueChanged = func(float64) { set() }
	row := NewBox(Row, 8, 0)
	row.AppendAligned(NewLabel(p.face, 14, Tr("Time"), 0), true, AlignCenter)
	row.AppendAligned(hour, false, AlignCenter)
	row.AppendAligned(NewLabel(p.face, 14, ":", 0), false, AlignCenter)
	row.AppendAligned(minute, false, AlignCenter)
	return row
}

// open shows the calendar in a popover under the trigger.
func (p *DatePicker) open() {
	body, cal := p.content()
	p.close = OpenPopover(p, p.trigger, body, PopoverOptions{Focus: cal, Gap: PopoverGap})
}

// dismiss closes the open popover.
func (p *DatePicker) dismiss() {
	if p.close != nil {
		p.close()
		p.close = nil
	}
}

// pickDate applies the user's date; without a time input the popover
// closes.
func (p *DatePicker) pickDate(d Date) {
	p.date = d
	p.sync()
	if !p.withTime {
		p.dismiss()
	}
	if p.OnDate != nil {
		p.OnDate(d)
	}
}

// pickRange applies the user's range picks; the popover closes once
// the range has both ends.
func (p *DatePicker) pickRange(from, to Date) {
	p.from, p.to = from, to
	p.sync()
	if to.IsZero() {
		return
	}
	p.dismiss()
	if p.OnRange != nil {
		p.OnRange(from, to)
	}
}

// pickPreset applies a preset in the picker's mode.
func (p *DatePicker) pickPreset(pr DatePreset) {
	if p.mode == CalendarSingle {
		p.pickDate(pr.From)
		return
	}
	from, to := pr.From, pr.To
	if to.Before(from) {
		from, to = to, from
	}
	p.pickRange(from, to)
}
