package fdtest

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestBeyondNamesWhatIsHeldMoreOftenThanTheBaselineHoldsIt(t *testing.T) {
	for _, tc := range []struct {
		name           string
		held, baseline []string
		want           []string
	}{
		{"the same", []string{"/a", "socket", "socket"}, []string{"/a", "socket", "socket"}, nil},
		{"one more of a kind held twice", []string{"/a", "socket", "socket", "socket"}, []string{"/a", "socket", "socket"}, []string{"socket"}},
		{"one of a kind the baseline lacks", []string{"/a", "/b", "socket"}, []string{"/a", "socket"}, []string{"/b"}},
		{"fewer of one kind does not pay for more of another", []string{"/b", "/b"}, []string{"/a", "/b"}, []string{"/b"}},
		{"fewer", []string{"socket"}, []string{"/a", "socket", "socket"}, nil},
		{"nothing held", nil, []string{"/a"}, nil},
		{"no baseline", []string{"/a", "socket"}, nil, []string{"/a", "socket"}},
	} {
		if got := Beyond(tc.held, tc.baseline); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Beyond(%q, %q) = %q, want %q", tc.name, tc.held, tc.baseline, got, tc.want)
		}
	}
}

func TestDescriptorsNameAFileAProcessOpenedUntilItClosesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held")
	before := Own(t)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := Beyond(Own(t), before); !slices.Equal(got, []string{path}) {
		_ = f.Close()
		t.Fatalf("with %s open the process holds %q beyond what it did, want that file alone", path, got)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := Beyond(Own(t), before); len(got) != 0 {
		t.Errorf("with the file closed the process still holds %q beyond what it did", got)
	}
}

func openPipe(t *testing.T) (closePipe func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		_ = r.Close()
		_ = w.Close()
	}
}

func TestDescriptorsCompareSocketsAndPipesByKind(t *testing.T) {
	// The first pipe of a process brings the runtime's poller with it.
	openPipe(t)()
	before := Own(t)
	defer openPipe(t)()
	if got := Beyond(Own(t), before); !slices.Equal(got, []string{"pipe", "pipe"}) {
		t.Errorf("with a pipe open the process holds %q beyond what it did, want its two ends by kind", got)
	}
}

func TestARawPipesReadEndGivesUpAtItsDeadline(t *testing.T) {
	r, w := RawPipe(t)
	defer r.Close()
	defer syscall.Close(int(w))
	if err := r.SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("the read end takes no deadline: %v", err)
	}
	if _, err := r.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("a read past the deadline with the write end open returned %v, want the deadline", err)
	}
}

func TestReceiveReturnsWhatTheSenderWroteBeforeClosing(t *testing.T) {
	got := Receive(t, func(w uintptr) {
		f := os.NewFile(w, "raw-pipe-w")
		if _, err := f.WriteString("payload"); err != nil {
			t.Error(err)
		}
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	if string(got) != "payload" {
		t.Errorf("received %q, want what was written", got)
	}
}

func TestStreamDeliversThePayloadThroughItsDup(t *testing.T) {
	got := Receive(t, func(w uintptr) {
		wait := Stream(t, w, []byte("from the peer"))
		if err := syscall.Close(int(w)); err != nil {
			t.Fatal(err)
		}
		if err := wait(); err != nil {
			t.Fatal(err)
		}
	})
	if string(got) != "from the peer" {
		t.Errorf("received %q, want the streamed payload", got)
	}
}

func TestHoldSilentKeepsThePipeOpenWithoutWriting(t *testing.T) {
	r, w := RawPipe(t)
	defer r.Close()
	HoldSilent(t, w)
	if err := syscall.Close(int(w)); err != nil {
		t.Fatal(err)
	}
	if err := r.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("read %d bytes, %v; want the deadline while the silent peer holds the pipe", n, err)
	}
}
