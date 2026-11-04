package animclock

import "time"

const frameClockSlack = 100 * time.Millisecond

// FrameClock converts a frame callback's timestamp, milliseconds from an
// undefined base that wraps, to an instant on the local clock.
type FrameClock struct {
	anchor   time.Time
	anchorMS uint32
}

// Instant is when the compositor's ms happened on the local clock, never
// after now. It keeps the earliest local reading of the compositor's clock
// as its anchor, so a delivery's latency is what it subtracts, and starts
// again from now when ms sits further behind than any delivery plausibly does.
func (c *FrameClock) Instant(ms uint32, now time.Time) time.Time {
	if !c.anchor.IsZero() {
		at := c.anchor.Add(time.Duration(int32(ms-c.anchorMS)) * time.Millisecond) //nolint:gosec
		if !at.After(now) && now.Sub(at) <= frameClockSlack {
			return at
		}
	}
	c.anchor, c.anchorMS = now, ms
	return now
}
