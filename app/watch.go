package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/stubbedev/lys-lib/intconv"
	"golang.org/x/sys/unix"
)

// File descriptors and paths watched on the application loop: a
// goroutine waits in poll(2) on the fd and an eventfd, hands the
// readiness to the loop through Invoke, and polls again only after the
// loop ran the callback, so a level-triggered fd the callback drains
// never spins. Stopping (or the loop ending) wakes the poller through
// the eventfd, without a timeout.

// ErrLoopEnded refuses a watcher once the application loop is gone.
var ErrLoopEnded = errors.New("app: the application loop has ended")

// WatchFD calls fn on the loop goroutine each time fd is ready for the
// poll events (unix.POLLIN, say), with the revents seen. fn should
// consume what made the fd ready: the next wait starts only after fn
// returned. The fd stays the caller's; stop (or the loop ending) ends
// the watch, after which fn is not called again.
func (a *Application) WatchFD(fd int, events int16, fn func(revents int16)) (stop func(), err error) {
	pollFD, err := intconv.Exact[int32](fd)
	if err != nil {
		return nil, fmt.Errorf("app: watch fd %d: %w", fd, err)
	}
	wake, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("app: watch eventfd: %w", err)
	}
	pollWake, err := intconv.Exact[int32](wake)
	if err != nil {
		_ = unix.Close(wake)
		return nil, fmt.Errorf("app: watch eventfd %d: %w", wake, err)
	}
	var once sync.Once
	var mu sync.Mutex
	stopped := false
	halt := func() {
		once.Do(func() {
			mu.Lock()
			stopped = true
			mu.Unlock()
			var one [8]byte
			one[0] = 1
			_, _ = unix.Write(wake, one[:])
		})
	}
	id, ok := a.watchers.add(halt)
	if !ok {
		_ = unix.Close(wake)
		return nil, ErrLoopEnded
	}
	go func() {
		defer func() {
			_ = unix.Close(wake)
			a.watchers.remove(id)
		}()
		pfds := []unix.PollFd{{Fd: pollFD, Events: events}, {Fd: pollWake, Events: unix.POLLIN}}
		// One ping-pong channel for the watch's life: each readiness
		// pass drains it before the loop polls again, so the buffered
		// send never blocks and never reallocates.
		ran := make(chan struct{}, 1)
		for {
			pfds[0].Revents, pfds[1].Revents = 0, 0
			if _, err := unix.Poll(pfds, -1); err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			if pfds[1].Revents != 0 {
				return
			}
			revents := pfds[0].Revents
			if revents&unix.POLLNVAL != 0 {
				return
			}
			a.Invoke(func() {
				mu.Lock()
				live := !stopped
				mu.Unlock()
				if live {
					fn(revents)
				}
				ran <- struct{}{}
			})
			// Wait for the loop to run fn (or for the stop): a loop that
			// ended drops the invoke, and the stop then ends this wait.
			select {
			case <-ran:
			case <-a.loopEnded():
				return
			}
			if revents&(unix.POLLHUP|unix.POLLERR) != 0 {
				return
			}
		}
	}()
	return halt, nil
}

// WatchFiles calls fn on the loop goroutine when anything under paths
// changes: a watched file written, created, removed, renamed or its
// metadata changed, or the same for the entries of a watched
// directory. With recursive, directories are watched with their whole
// trees, including directories created later. A burst of changes read
// in one pass is one call. A path that does not exist yet is skipped.
func (a *Application) WatchFiles(paths []string, recursive bool, fn func()) (stop func(), err error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("app: inotify: %w", err)
	}
	w := &fileWatch{fd: fd, recursive: recursive, dirs: map[int]string{}, buf: make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))}
	for _, p := range paths {
		w.add(p)
	}
	stopFD, err := a.WatchFD(fd, unix.POLLIN, func(int16) {
		if w.drain() {
			fn()
		}
	})
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			stopFD()
			_ = unix.Close(fd)
		})
	}, nil
}

// fileChanges is what counts as a change.
const fileChanges uint32 = unix.IN_CREATE | unix.IN_MODIFY | unix.IN_DELETE | unix.IN_CLOSE_WRITE |
	unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF

// fileWatch is one inotify instance's watches (loop-owned after start).
type fileWatch struct {
	fd        int
	recursive bool
	dirs      map[int]string
	// buf is the inotify read buffer, allocated once per watch.
	buf []byte
}

// add watches path (and, recursive, its tree).
func (w *fileWatch) add(path string) {
	if !w.recursive {
		w.watch(path)
		return
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || p == path {
			w.watch(p)
		}
		return nil
	})
}

func (w *fileWatch) watch(path string) {
	if wd, err := unix.InotifyAddWatch(w.fd, path, fileChanges); err == nil {
		w.dirs[wd] = path
	}
}

// drain reads every queued event and reports whether any was a
// change, watching directories created inside a recursive tree.
func (w *fileWatch) drain() bool {
	changed := false
	for {
		n, err := unix.Read(w.fd, w.buf)
		if n <= 0 || err != nil {
			return changed
		}
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			wd := int(intconv.Int32Bits(lenU32(w.buf[off:])))
			mask := lenU32(w.buf[off+4:])
			nameLen := int(lenU32(w.buf[off+12:]))
			name := w.buf[off+unix.SizeofInotifyEvent : min(n, off+unix.SizeofInotifyEvent+nameLen)]
			off += unix.SizeofInotifyEvent + nameLen
			if mask&unix.IN_IGNORED != 0 {
				delete(w.dirs, wd)
				continue
			}
			if mask&fileChanges == 0 {
				continue
			}
			changed = true
			if w.recursive && mask&unix.IN_ISDIR != 0 && mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 {
				if dir, ok := w.dirs[wd]; ok {
					w.add(filepath.Join(dir, cString(name)))
				}
			}
		}
	}
}

func lenU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// cString is the name up to its NUL padding.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// loopEnded is closed once the application loop ended.
func (a *Application) loopEnded() <-chan struct{} { return a.endedChan() }

func (a *Application) endedChan() chan struct{} {
	a.endedMu.Lock()
	defer a.endedMu.Unlock()
	if a.ended == nil {
		a.ended = make(chan struct{})
	}
	return a.ended
}

func (a *Application) endLoop() {
	ch := a.endedChan()
	a.endedOnce.Do(func() { close(ch) })
}
