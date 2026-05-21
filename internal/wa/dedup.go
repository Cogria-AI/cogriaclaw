package wa

import "sync"

// dedup remembers recently-seen message IDs so a message redelivered by
// WhatsApp (e.g. after a reconnect, before our ack lands) isn't handled twice.
// Bounded, in-memory, FIFO eviction — losing old IDs is fine since WhatsApp
// only redelivers recent messages.
type dedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
	ring []string
	idx  int
}

func newDedup(capacity int) *dedup {
	if capacity < 1 {
		capacity = 4096
	}
	return &dedup{
		seen: make(map[string]struct{}, capacity),
		ring: make([]string, capacity),
	}
}

// seenBefore reports whether id was already seen, recording it if not.
func (d *dedup) seenBefore(id string) bool {
	if id == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[id]; ok {
		return true
	}
	if old := d.ring[d.idx]; old != "" {
		delete(d.seen, old)
	}
	d.ring[d.idx] = id
	d.idx = (d.idx + 1) % len(d.ring)
	d.seen[id] = struct{}{}
	return false
}
