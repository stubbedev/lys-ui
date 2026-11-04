package widget

import (
	"maps"
	"slices"
	"sync"
)

// maxAssetQueue bounds the decodes waiting: only what a frame painted
// asks, so a long queue means what it asked first is off screen by now.
const maxAssetQueue = 1024

// assetJob is one decode off the loop and whom it answers on the loop.
type assetJob struct {
	key     any
	load    func()
	waiters []func()
	deliver func(func())
}

// assets runs the decodes paints ask for, in order, on one goroutine
// that exists only while there is work. Its load functions touch no
// widget; answers come back through the invoker.
var assets = newAssetQueue()

type assetQueue struct {
	mu      sync.Mutex
	idle    sync.Cond
	queue   []*assetJob
	jobs    map[any]*assetJob
	running bool
}

func newAssetQueue() *assetQueue {
	q := &assetQueue{jobs: map[any]*assetJob{}}
	q.idle.L = &q.mu
	return q
}

// loadAsset runs load off the loop, once per key however many ask, then
// done on the loop for each asker. Without an invoker there is no loop
// to answer on, and both run inline.
func loadAsset(key any, load, done func()) {
	deliver, ok := loopInvoker()
	if !ok {
		load()
		done()
		return
	}
	if dropped := assets.add(key, load, done, deliver); dropped != nil {
		answer(dropped)
	}
}

// add queues the job, or joins the one already there, and returns the
// job the bound dropped.
func (q *assetQueue) add(key any, load, done func(), deliver func(func())) (dropped *assetJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if j, ok := q.jobs[key]; ok {
		j.waiters = append(j.waiters, done)
		return nil
	}
	if len(q.queue) >= maxAssetQueue {
		oldest := q.queue[0]
		q.queue = slices.Delete(q.queue, 0, 1)
		delete(q.jobs, oldest.key)
		dropped = oldest
	}
	j := &assetJob{key: key, load: load, waiters: []func(){done}, deliver: deliver}
	q.jobs[key] = j
	q.queue = append(q.queue, j)
	if !q.running {
		q.running = true
		go q.run()
	}
	return dropped
}

func (q *assetQueue) run() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.queue) > 0 {
		j := q.queue[0]
		q.queue = slices.Delete(q.queue, 0, 1)
		q.mu.Unlock()
		j.load()
		q.mu.Lock()
		delete(q.jobs, j.key)
		q.mu.Unlock()
		answer(j)
		q.mu.Lock()
	}
	q.running = false
	q.idle.Broadcast()
}

// answer runs the waiters on the loop. A dropped job answers too: whoever
// still paints the asset asks again.
func answer(j *assetJob) {
	j.deliver(func() {
		for _, w := range j.waiters {
			w()
		}
	})
}

// stop drops what has not started; the running decode finishes and ends
// the goroutine, and its answer goes nowhere once the loop has ended.
func (q *assetQueue) stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.queue {
		delete(q.jobs, j.key)
	}
	q.queue = nil
}

// wait blocks until the goroutine has run out of work.
func (q *assetQueue) wait() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.running {
		q.idle.Wait()
	}
}

// StopAssetLoads drops the icon, stylesheet image and font loads that
// have not started. Application calls it when its loop ends; answers from
// here on go nowhere, so nothing stays asked for.
func StopAssetLoads() {
	assets.stop()
	maps.DeleteFunc(cssImages, func(_ string, img *cssImage) bool { return !img.settled })
}

// WaitAssetLoads blocks until every icon, stylesheet image and font load
// asked for so far has finished and queued its answer on the loop, whose
// next pass then applies it. It is what a test that needs a settled frame
// waits on; a UI loop must never call it.
func WaitAssetLoads() { assets.wait() }
