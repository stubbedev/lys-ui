package buffer

import (
	"errors"
	"strings"
	"testing"
)

// pendingImports is a pool whose create makes dmabuf buffers still
// waiting on the compositor's verdict, each with its import handler.
func pendingImports(rejected func(error)) (*Pool, *[]*dmabufImport) {
	var imps []*dmabufImport
	p := New(func() (*Buffer, error) {
		b := newTestBuffer(4, 4)
		b.pending = true
		imp := &dmabufImport{b: b, done: func() {}, rejected: rejected}
		imps = append(imps, imp)
		return b, nil
	}, 2)
	return p, &imps
}

// A new dmabuf buffer holds its frame until the compositor imports
// it: the pool reports busy, then hands the buffer out once created
// (#365).
func TestPendingImportIsBusyUntilCreated(t *testing.T) {
	p, imps := pendingImports(func(err error) { t.Fatalf("rejected: %v", err) })
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatalf("acquire with the import pending: %v, want ErrBusy", err)
	}
	b := (*imps)[0].b
	(*imps)[0].created(nil)
	got, err := p.Acquire()
	if err != nil || got != b {
		t.Fatalf("acquire after the import: %v, %v; want the imported buffer", got, err)
	}
}

// A refused import drops the buffer and reports the refusal, with
// nothing left that a frame could draw into or commit (#365).
func TestRefusedImportLeavesThePool(t *testing.T) {
	var refused error
	p, imps := pendingImports(func(err error) { refused = err })
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatalf("acquire with the import pending: %v, want ErrBusy", err)
	}
	(*imps)[0].failed()
	if refused == nil || !strings.Contains(refused.Error(), "refused a 4x4 dmabuf import") {
		t.Errorf("refusal reported as %v", refused)
	}
	if n := p.Pending(); n != 0 {
		t.Errorf("the pool still tracks %d buffers after the refusal", n)
	}
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) || len(*imps) != 2 {
		t.Errorf("acquire after the refusal: %v with %d imports; want a fresh import pending", err, len(*imps))
	}
}

// An import answered after its pool closed is dropped, not reused.
func TestImportAfterCloseIsDropped(t *testing.T) {
	p, imps := pendingImports(func(err error) { t.Fatalf("rejected: %v", err) })
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	p.Close()
	(*imps)[0].created(nil)
	if n := p.Pending(); n != 0 {
		t.Errorf("the closed pool still tracks %d buffers", n)
	}
}
