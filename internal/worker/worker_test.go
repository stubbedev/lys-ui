package worker

import (
	"testing"
	"time"
)

const patience = 10 * time.Second

func TestQueueRunsInOrder(t *testing.T) {
	q := New(8)
	var got []int
	for i := range 8 {
		if !q.Post(func() { got = append(got, i) }) {
			t.Fatalf("post %d refused", i)
		}
		if !q.Settle(patience) {
			t.Fatal("queue did not settle")
		}
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("ran %v, want posting order", got)
		}
	}
}

func TestBlockedQueueRefusesWithoutBlocking(t *testing.T) {
	q := New(2)
	release := make(chan struct{})
	entered := make(chan struct{})
	q.Post(func() {
		close(entered)
		<-release
	})
	<-entered
	ran := 0
	for range 2 {
		if !q.Post(func() { ran++ }) {
			t.Fatal("a post within capacity was refused")
		}
	}
	if q.Post(func() { ran++ }) {
		t.Fatal("a post past capacity was taken")
	}
	close(release)
	if !q.Stop(patience) {
		t.Fatal("Stop did not finish the queue")
	}
	if ran != 2 {
		t.Errorf("ran %d queued jobs, want both", ran)
	}
	if q.Post(func() { ran++ }) {
		t.Error("a stopped queue took a post")
	}
}

func TestStopReportsStuckWork(t *testing.T) {
	q := New(1)
	release := make(chan struct{})
	entered := make(chan struct{})
	q.Post(func() {
		close(entered)
		<-release
	})
	<-entered
	if q.Stop(time.Millisecond) {
		t.Error("Stop reported success over a stuck job")
	}
	close(release)
	if !q.Stop(patience) {
		t.Error("Stop did not see the job end")
	}
}
