package notify

import (
	"strings"
	"testing"
	"time"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

// TestPortalDict pins the AddNotification vardict: title, body,
// priority always; icon, timeout, default action, and buttons only
// when set, with the double-variant shape the portal spec wants.
func TestPortalDict(t *testing.T) {
	bare := portalDict(Notification{Title: "t", Body: "b", Priority: Normal})
	if got := bare["title"].Value(); got != "t" {
		t.Errorf("title = %v", got)
	}
	if got := bare["priority"].Value(); got != "normal" {
		t.Errorf("priority = %v", got)
	}
	for _, absent := range []string{"icon", "timeout", "default-action", "buttons"} {
		if _, ok := bare[absent]; ok {
			t.Errorf("bare notification carried %q", absent)
		}
	}

	full := portalDict(Notification{
		Title:         "done",
		Body:          "the batch finished",
		Icon:          "document-save",
		Priority:      Critical,
		Timeout:       5 * time.Second,
		DefaultAction: Action{Key: "open", Label: "Show"},
		HasDefault:    true,
		Actions:       []Action{{Key: "retry", Label: "Retry"}},
	})
	if got := full["priority"].Value(); got != "urgent" {
		t.Errorf("critical priority = %v", got)
	}
	if got := full["timeout"].Value(); got != int32(5000) {
		t.Errorf("timeout = %v", got)
	}
	if inner, ok := full["icon"].Value().(interface{ Value() any }); !ok || inner.Value() != "document-save" {
		t.Errorf("icon not the double variant: %v", full["icon"].Value())
	}
	buttons, ok := full["buttons"].Value().([]dbus.Variant)
	if !ok || len(buttons) != 1 {
		t.Fatalf("buttons = %#v", full["buttons"].Value())
	}
	button, ok := buttons[0].Value().(map[string]dbus.Variant)
	if !ok {
		t.Fatalf("button = %#v", buttons[0].Value())
	}
	if got := button["label"].Value(); got != "Retry" {
		t.Errorf("button label = %v", got)
	}
	if got := button["action"].Value(); got != "retry" {
		t.Errorf("button action = %v", got)
	}
}

// buttons, ok := full["buttons"].Value().([]any) is asserted below
// directly; this type alias is no longer needed.

// TestClassicWire pins the classic tuple: the flat action pair list
// with default first, the urgency hint byte, and the timeout mapping.
func TestClassicWire(t *testing.T) {
	nf := Notification{
		Actions:       []Action{{Key: "retry", Label: "Retry"}},
		DefaultAction: Action{Key: "open", Label: "Show"},
		HasDefault:    true,
		Priority:      Low,
	}
	if got := classicActions(nf); strings.Join(got, "|") != "default|Show|retry|Retry" {
		t.Errorf("actions = %v", got)
	}
	if got := classicHints(nf)["urgency"].Value(); got != byte(0) {
		t.Errorf("low urgency hint = %v", got)
	}
	if got := classicHints(Notification{Priority: Critical})["urgency"].Value(); got != byte(2) {
		t.Errorf("critical urgency hint = %v", got)
	}
	if got := classicTimeout(0); got != 0 {
		t.Errorf("zero timeout = %d, want the server default", got)
	}
	if got := classicTimeout(-1); got != -1 {
		t.Errorf("negative timeout = %d, want never", got)
	}
	if got := classicTimeout(1500 * time.Millisecond); got != 1500 {
		t.Errorf("1500ms timeout = %d", got)
	}
}

// TestCapBody pins the body cap: under-cap bodies pass byte-for-byte,
// rune counting decides the cut, and a capped body ends in an ellipsis.
func TestCapBody(t *testing.T) {
	short := strings.Repeat("x", maxBodyRunes)
	if got := capBody(short); got != short {
		t.Error("a body at the cap was touched")
	}
	long := strings.Repeat("é", maxBodyRunes+10)
	got := capBody(long)
	if n := len([]rune(got)); n != maxBodyRunes {
		t.Errorf("capped body has %d runes, want %d", n, maxBodyRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a capped body does not end in an ellipsis")
	}
}

// TestClassicIDMapping pins the classic id routing: the daemon's
// numeric id resolves to lys-ui's string id both ways.
func TestClassicIDMapping(t *testing.T) {
	n := &Notifier{}
	n.rememberClassic(7, "lys-ui-1")
	if got := n.classicID(7); got != "lys-ui-1" {
		t.Errorf("classic 7 routed to %q", got)
	}
	if got := n.classicID(8); got != "" {
		t.Errorf("unknown classic id routed to %q", got)
	}
	if got := n.classicOf["lys-ui-1"]; got != 7 {
		t.Errorf("reverse map = %d", got)
	}
}
