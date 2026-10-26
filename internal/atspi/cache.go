package atspi

import (
	"slices"

	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// The AT-SPI Cache (org.a11y.atspi.Cache at /org/a11y/atspi/cache):
// one call hands an AT every object with its role, name, states, and
// interfaces, and AddAccessible / RemoveAccessible keep its copy
// current - the batch that spares a screen reader one round trip per
// property when it first meets the application.

// CachePath is the Cache object's path, fixed by the AT-SPI spec.
const CachePath dbus.ObjectPath = "/org/a11y/atspi/cache"

const ifaceCache = "org.a11y.atspi.Cache"

// cacheItem is one object's entry, (so)(so)(so)iiassusau: itself, its
// application, its parent, its index there, its child count, its
// interfaces, name, role, description, and state set.
type cacheItem struct {
	Path        objRef
	App         objRef
	Parent      objRef
	Index       int32
	Children    int32
	Interfaces  []string
	Name        string
	Role        uint32
	Description string
	States      []uint32
}

// cacheIface serves GetItems from the published snapshot.
type cacheIface struct{ b *Bridge }

// GetItems lists every object, root first, then by stable id.
func (o *cacheIface) GetItems() []cacheItem {
	o.b.mu.Lock()
	defer o.b.mu.Unlock()
	tr := o.b.cur
	ids := sortedIDs(tr)
	out := make([]cacheItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, o.b.cacheItemOf(tr, tr.nodes[id]))
	}
	return out
}

// cacheItemOf builds n's entry in tr. Callers hold the bridge mutex.
func (b *Bridge) cacheItemOf(tr *tree, n *anode) cacheItem {
	role := roleApplication
	if n.id != 0 {
		role = atspiRole(n.st.Role)
	}
	return cacheItem{
		Path:       objRef{Name: b.name, Path: pathOf(n)},
		App:        objRef{Name: b.name, Path: RootPath},
		Parent:     b.parentRef(n),
		Index:      intconv.Int32(n.index),
		Children:   intconv.Int32(len(n.children)),
		Interfaces: ifacesOf(n),
		Name:       nameOf(n),
		Role:       role,
		States:     stateSet(b.statesOf(tr, n.id)),
	}
}

// exportCache serves the Cache object.
func (b *Bridge) exportCache() {
	mustExport(b, CachePath, ifaceCache, &cacheIface{b: b})
	mustExport(b, CachePath, ifaceIntrospectable, &xmlIntrospect{xml: introspectionDoc(xmlCache)})
}

// cacheEvents are the Cache signals between two passes: objects that
// appeared (their full entries) and objects that went away.
func (b *Bridge) cacheEvents(old, cur *tree) []event {
	var out []event
	for _, id := range sortedIDs(cur) {
		if _, had := old.nodes[id]; !had {
			out = append(out, event{
				path: CachePath, name: ifaceCache + ".AddAccessible",
				args: []any{b.cacheItemOf(cur, cur.nodes[id])},
			})
		}
	}
	for _, id := range sortedIDs(old) {
		if _, has := cur.nodes[id]; !has {
			out = append(out, event{
				path: CachePath, name: ifaceCache + ".RemoveAccessible",
				args: []any{objRef{Name: b.name, Path: pathOf(old.nodes[id])}},
			})
		}
	}
	return out
}

// sortedIDs is tr's ids in ascending order.
func sortedIDs(tr *tree) []int32 {
	ids := make([]int32, 0, len(tr.nodes))
	for id := range tr.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
