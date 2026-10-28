package notify

import (
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stubbedev/lys-lib/intconv"
)

// The wire builders, pure and table-tested: the same Notification
// becomes the portal's vardict or the classic spec's argument tuple.

// portalPriority maps Priority to the portal's priority strings.
var portalPriority = map[Priority]string{
	Low:      "low",
	Normal:   "normal",
	High:     "high",
	Critical: "urgent",
}

// classicUrgency maps Priority to the classic spec's urgency byte.
var classicUrgency = map[Priority]byte{
	Low:      0,
	Normal:   1,
	High:     2,
	Critical: 2,
}

// portalDict builds the AddNotification vardict.
func portalDict(nf Notification) map[string]dbus.Variant {
	d := map[string]dbus.Variant{
		"title":    dbus.MakeVariant(nf.Title),
		"body":     dbus.MakeVariant(nf.Body),
		"priority": dbus.MakeVariant(portalPriority[nf.Priority]),
	}
	if nf.Icon != "" {
		d["icon"] = dbus.MakeVariant(dbus.MakeVariant(nf.Icon))
	}
	if ms := classicTimeout(nf.Timeout); ms != 0 {
		d["timeout"] = dbus.MakeVariant(ms)
	}
	if nf.HasDefault {
		d["default-action"] = dbus.MakeVariant(nf.DefaultAction.Key)
		d["default-action-label"] = dbus.MakeVariant(nf.DefaultAction.Label)
		d["default-action-target"] = dbus.MakeVariant(dbus.MakeVariant(nf.DefaultAction.Key))
	}
	if len(nf.Actions) > 0 {
		buttons := make([]dbus.Variant, 0, len(nf.Actions))
		for _, a := range nf.Actions {
			buttons = append(buttons, dbus.MakeVariant(map[string]dbus.Variant{
				"label":  dbus.MakeVariant(a.Label),
				"action": dbus.MakeVariant(a.Key),
				"target": dbus.MakeVariant(dbus.MakeVariant(a.Key)),
			}))
		}
		d["buttons"] = dbus.MakeVariant(buttons)
	}
	return d
}

// classicActions builds the flat [key, label, ...] action pair list.
func classicActions(nf Notification) []string {
	actions := make([]string, 0, 2*len(nf.Actions)+2)
	if nf.HasDefault {
		actions = append(actions, "default", nf.DefaultAction.Label)
	}
	for _, a := range nf.Actions {
		actions = append(actions, a.Key, a.Label)
	}
	return actions
}

// classicHints builds the hints dict; urgency rides along.
func classicHints(nf Notification) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(classicUrgency[nf.Priority]),
	}
}

// classicTimeout maps the duration to the spec's expire_timeout: 0 is
// the server default, -1 never expires.
func classicTimeout(d time.Duration) int32 {
	switch {
	case d == 0:
		return 0
	case d < 0:
		return -1
	default:
		return intconv.Clamp[int32](d.Milliseconds())
	}
}

// capBody truncates the body to maxBodyRunes runes, marking a cut with
// an ellipsis so nobody mistakes a capped body for a whole one.
func capBody(body string) string {
	if len(body) <= maxBodyRunes {
		return body
	}
	runes := []rune(body)
	if len(runes) <= maxBodyRunes {
		return body
	}
	return string(runes[:maxBodyRunes-1]) + "…"
}
