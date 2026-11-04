package widget

import "testing"

func TestRemovingAnInvokerLeavesALaterOne(t *testing.T) {
	t.Cleanup(func() { SetInvoker(nil) })
	removeFirst := SetInvoker(func(fn func()) { fn() })
	SetInvoker(func(fn func()) { fn() })
	removeFirst()
	if !hasInvoker() {
		t.Error("removing the first invoker removed the one installed after it")
	}
}

func TestALoadInFlightAnswersTheLoopThatAskedNotTheCaller(t *testing.T) {
	var queued []func()
	remove := SetInvoker(func(fn func()) { queued = append(queued, fn) })
	t.Cleanup(func() { SetInvoker(nil) })
	release := make(chan struct{})
	answered := false
	loadAsset("in-flight", func() { <-release }, func() { answered = true })
	remove()
	close(release)
	WaitAssetLoads()
	if answered {
		t.Fatal("the answer ran off the loop after the bridge went")
	}
	if len(queued) != 1 {
		t.Fatalf("%d answers reached the asking loop's bridge, want 1", len(queued))
	}
}
