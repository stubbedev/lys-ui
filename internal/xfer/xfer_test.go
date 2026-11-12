package xfer

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/fdtest"
)

// peerStream plays the foreign client at the far end of a transfer
// pipe: it dups the descriptor (the way the compositor hands one out),
// streams payload from its own goroutine, and returns a wait func that
// reports once every byte landed without error — a bounded wait, so a
// regression that leaves the peer blocked on the pipe fails instead of
// hanging.
func peerStream(t *testing.T, fd uintptr, payload []byte) func() error {
	t.Helper()
	d, err := syscall.Dup(int(fd))
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(d), "xfer-test-peer")
	done := make(chan error, 1)
	go func() {
		_, werr := f.Write(payload)
		_ = f.Close()
		done <- werr
	}()
	return func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			return errors.New("peer still blocked on the pipe after 5s")
		}
	}
}

func TestReadReturnsTheWholeSmallPayload(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	wait := peerStream(t, w.Fd(), []byte("hello paste"))
	w.Close() // our end drops; the peer's dup keeps the pipe alive

	got, err := Read(r, 1<<20, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello paste" {
		t.Errorf("read %q, want the peer's payload", got)
	}
	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

func TestReadAcceptsAPayloadExactlyAtTheCap(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	payload := bytes.Repeat([]byte("z"), 4096)
	wait := peerStream(t, w.Fd(), payload)
	w.Close()

	got, err := Read(r, int64(len(payload)), 5*time.Second)
	if err != nil {
		t.Fatalf("payload at the cap read as %v, want success", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("read %d bytes, want the full %d", len(got), len(payload))
	}
	if err := wait(); err != nil {
		t.Fatal(err)
	}
}

// A peer streaming past the cap gets truncated — with an error — and
// its pipe drained: the wait func only returns once the peer's whole
// payload landed without error, which takes the reader draining past
// the cap.
func TestReadCapsAnOversizePayloadAndDrainsThePipe(t *testing.T) {
	const limit = 4096
	payload := bytes.Repeat([]byte{0xA5}, 256<<10) // far past the cap and the pipe buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	wait := peerStream(t, w.Fd(), payload)
	w.Close()

	got, err := Read(r, limit, 5*time.Second)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("read = %v, want ErrTooLarge", err)
	}
	if int64(len(got)) != limit {
		t.Errorf("read %d bytes, want the %d-byte cap", len(got), limit)
	}
	if err := wait(); err != nil {
		t.Errorf("peer blocked or errored past the cap: %v", err)
	}
}

func TestReadTimesOutOnASilentPeer(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close() // our write end stays open: only the deadline can end the read
	defer r.Close()

	before := runtime.NumGoroutine()
	start := time.Now()
	_, err = Read(r, 1<<20, 75*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("read = %v, want ErrTimeout", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("silent peer hung the read for %s, want the short deadline", elapsed)
	}
	// The deadline is a runtime timer, not a parked goroutine: nothing
	// may outlive the transfer.
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the deadline leaked one", before, after)
	}
}

func TestDeadlineWriterDeliversAndCloses(t *testing.T) {
	got := fdtest.Receive(t, func(w uintptr) {
		f, err := DeadlineWriter(w, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, "payload"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if string(got) != "payload" {
		t.Errorf("consumer read %q, want the payload", got)
	}
}

func TestDeadlineWriterCutsOffAConsumerThatNeverReads(t *testing.T) {
	r, w := fdtest.RawPipe(t) // the read end is never touched
	defer r.Close()

	f, err := DeadlineWriter(w, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	before := runtime.NumGoroutine()
	start := time.Now()
	_, err = f.Write(bytes.Repeat([]byte{0x5A}, 1<<20)) // far past the pipe buffer
	elapsed := time.Since(start)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write = %v, want a deadline error", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("stuck consumer held the write for %s, want the short deadline", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines %d -> %d: the deadline leaked one", before, after)
	}
}

func TestDeadlineWriterEndsOnEPIPE(t *testing.T) {
	r, w := fdtest.RawPipe(t)
	r.Close() // the consumer hung up before reading

	f, err := DeadlineWriter(w, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	start := time.Now()
	_, werr := f.Write([]byte("nobody home"))
	if !errors.Is(werr, syscall.EPIPE) {
		t.Fatalf("write = %v, want EPIPE", werr)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("EPIPE took %s to surface, want it immediate", elapsed)
	}
}

func TestDeadlineWriterRefusesADescriptorThatIsNotOpen(t *testing.T) {
	const notOpen = 1 << 20
	if _, err := DeadlineWriter(notOpen, time.Second); !errors.Is(err, syscall.EBADF) {
		t.Errorf("DeadlineWriter(%d) = %v, want EBADF", notOpen, err)
	}
}

func TestTransfersBoundAndStop(t *testing.T) {
	var tr Transfers
	silent := make([]*os.File, 0, MaxTransfers)
	t.Cleanup(func() {
		for _, f := range silent {
			_ = f.Close()
		}
	})
	done := make(chan error, MaxTransfers)
	hold := func(fd uintptr) error {
		d, err := syscall.Dup(int(fd))
		if err != nil {
			return err
		}
		silent = append(silent, os.NewFile(uintptr(d), "silent-peer"))
		return nil
	}
	for range MaxTransfers {
		err := tr.Receive(MaxPayload, time.Hour, hold, func(_ []byte, err error) { done <- err })
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.Receive(MaxPayload, time.Hour, hold, func([]byte, error) {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("a transfer past the bound = %v, want ErrBusy", err)
	}
	if err := tr.Go(func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("Go past the bound = %v, want ErrBusy", err)
	}
	tr.Stop()
	if len(done) != MaxTransfers {
		t.Fatalf("Stop returned with %d of %d transfers done", len(done), MaxTransfers)
	}
	if err := tr.Go(func() {}); !errors.Is(err, ErrStopped) {
		t.Errorf("Go after Stop = %v, want ErrStopped", err)
	}
}

func TestRefusedSendClosesTheDescriptor(t *testing.T) {
	var tr Transfers
	tr.Stop()
	got := fdtest.Receive(t, func(w uintptr) {
		err := tr.Send(w, time.Hour, func(io.Writer) error { return nil }, func(error) {})
		if !errors.Is(err, ErrStopped) {
			t.Fatalf("send after Stop = %v, want ErrStopped", err)
		}
	})
	if len(got) != 0 {
		t.Errorf("consumer read %q from a refused send, want EOF", got)
	}
}

func TestSendRunsOffTheCaller(t *testing.T) {
	var tr Transfers
	defer tr.Stop()
	release := make(chan struct{})
	data := fdtest.Receive(t, func(w uintptr) {
		err := tr.Send(w, time.Hour, func(w io.Writer) error {
			select {
			case <-release:
			case <-time.After(10 * time.Second):
				return errors.New("the send was still waiting for its release after 10s: Send ran on its caller")
			}
			_, err := w.Write([]byte("late"))
			return err
		}, func(err error) { t.Errorf("send: %v", err) })
		if err != nil {
			t.Fatal(err)
		}
		close(release)
	})
	if string(data) != "late" {
		t.Errorf("consumer read %q, want the late write", data)
	}
}
