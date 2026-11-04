package mailbox

import (
	"slices"
	"sync"
	"testing"
)

func TestPutArmsOnceUntilTake(t *testing.T) {
	var b Box[int]
	arms := 0
	for i := range 100 {
		if b.Put(i) {
			arms++
		}
	}
	if arms != 1 {
		t.Fatalf("100 puts armed %d drains, want 1", arms)
	}
	if got := b.Take(); len(got) != 100 || got[0] != 0 || got[99] != 99 {
		t.Fatalf("take = %d items, want 0..99 in order", len(got))
	}
	if !b.Put(7) {
		t.Error("put after take did not re-arm")
	}
}

func TestStopDropsAndRefuses(t *testing.T) {
	var b Box[string]
	b.Put("queued")
	b.Stop()
	if b.Put("late") {
		t.Error("a stopped box armed a drain")
	}
	if got := b.Take(); len(got) != 0 {
		t.Errorf("a stopped box kept %v", got)
	}
	if !b.Stopped() {
		t.Error("Stopped is false after Stop")
	}
}

func TestConcurrentPutsKeepPerProducerOrder(t *testing.T) {
	var b Box[[2]int]
	var wg sync.WaitGroup
	for p := range 4 {
		wg.Go(func() {
			for i := range 250 {
				b.Put([2]int{p, i})
			}
		})
	}
	wg.Wait()
	seen := make([][]int, 4)
	for _, it := range b.Take() {
		seen[it[0]] = append(seen[it[0]], it[1])
	}
	for p, got := range seen {
		if len(got) != 250 || !slices.IsSorted(got) {
			t.Errorf("producer %d: %d items, sorted=%v", p, len(got), slices.IsSorted(got))
		}
	}
}

func TestReleasedBatchCarriesTheNextOne(t *testing.T) {
	var b Box[*int]
	item := new(int)
	cycle := func() {
		b.Put(item)
		batch := b.Take()
		b.Release(batch)
	}
	cycle()
	cycle()
	if allocs := testing.AllocsPerRun(100, cycle); allocs != 0 {
		t.Errorf("a put, take and release allocate %v", allocs)
	}
}

func TestReleaseDropsWhatTheBatchReferenced(t *testing.T) {
	var b Box[*int]
	b.Put(new(int))
	batch := b.Take()
	b.Release(batch)
	if batch[0] != nil {
		t.Error("a released batch still references its item")
	}
}

func TestATakeInsideADrainGetsItsOwnBatch(t *testing.T) {
	var b Box[int]
	b.Put(1)
	first := b.Take()
	b.Put(2)
	second := b.Take()
	b.Release(second)
	if first[0] != 1 || second[0] != 0 {
		t.Fatalf("batches shared an array: %v %v", first, second)
	}
}

func TestAStormsBatchGoesToTheCollector(t *testing.T) {
	var b Box[int]
	for i := range maxSpareItems + 1 {
		b.Put(i)
	}
	b.Release(b.Take())
	if b.spare != nil {
		t.Errorf("the box kept a %d item batch", cap(b.spare))
	}
}
