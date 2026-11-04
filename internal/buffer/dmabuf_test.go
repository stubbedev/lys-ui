package buffer

import (
	"errors"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/gpu"
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
	(*imps)[0].failed(errors.New("buffer: the compositor refused a 4x4 dmabuf import"))
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

func TestADmabufAllocatesNothingWhereItIsAsked(t *testing.T) {
	var asked []func(*gpu.Target, error)
	allocate := func(_, _ int, done func(*gpu.Target, error)) { asked = append(asked, done) }
	p := New(func() (*Buffer, error) {
		return NewDmabuf(allocate, nil, 4, 4, 1, func(err error) { t.Errorf("rejected: %v", err) })
	}, 2)
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatalf("acquire with the target on its way: %v, want ErrBusy", err)
	}
	if len(asked) != 1 || p.buffers[0].Target != nil {
		t.Fatalf("%d allocations asked, target %v: want one asked and none made here", len(asked), p.buffers[0].Target)
	}
	p.Resize(func() (*Buffer, error) { return nil, errors.New("not used") })
	asked[0](nil, gpu.ErrDeviceClosed)
	if n := p.Pending(); n != 0 {
		t.Errorf("an allocation ended by its device's close left %d buffers tracked", n)
	}
}

func TestAFailedDmabufAllocationLeavesThePool(t *testing.T) {
	var refused error
	var done func(*gpu.Target, error)
	p := New(func() (*Buffer, error) {
		return NewDmabuf(func(_, _ int, d func(*gpu.Target, error)) { done = d }, nil, 4, 4, 1, func(err error) { refused = err })
	}, 2)
	if _, err := p.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	done(nil, errors.New("out of device memory"))
	if refused == nil || !strings.Contains(refused.Error(), "allocate a 4x4 dmabuf: out of device memory") {
		t.Errorf("the failure reported as %v", refused)
	}
	if n := p.Pending(); n != 0 {
		t.Errorf("the pool still tracks %d buffers after the failure", n)
	}
}
