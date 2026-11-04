package app

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/stubbedev/lys-lib/drm"
	"github.com/stubbedev/lys-lib/gpu"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

type syncTimelines interface {
	create() (drm.Syncobj, int, error)
	acquire(s drm.Syncobj, point uint64, syncFile int) error
	watch(s drm.Syncobj, point uint64, released func()) error
	destroy(s drm.Syncobj)
	close()
}

func openTimelines(node string) (*drmTimelines, error) {
	d, err := drm.Open(node)
	if err != nil {
		return nil, err
	}
	if v, err := d.Cap(drm.CapSyncobjTimeline); err != nil || v == 0 {
		return nil, errors.Join(fmt.Errorf("app: %s has no timeline syncobjs", node), err, d.Close())
	}
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("app: epoll for release points: %w", err), d.Close())
	}
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("app: eventfd to stop the release watch: %w", err), unix.Close(epfd), d.Close())
	}
	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, stop, &unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(stop)}); err != nil { //nolint:gosec // file descriptors fit an int32
		return nil, errors.Join(fmt.Errorf("app: watch the stop eventfd: %w", err), unix.Close(stop), unix.Close(epfd), d.Close())
	}
	t := &drmTimelines{dev: d, epfd: epfd, stop: stop, fds: map[drm.Syncobj]int{}, released: map[int]func(){}}
	t.watcher.Go(t.run)
	return t, nil
}

type drmTimelines struct {
	dev     *drm.Device
	epfd    int
	stop    int
	invoke  func(func())
	watcher sync.WaitGroup

	fds      map[drm.Syncobj]int
	closed   bool
	mu       sync.Mutex
	released map[int]func()
}

func (t *drmTimelines) setInvoke(invoke func(func())) {
	t.mu.Lock()
	t.invoke = invoke
	t.mu.Unlock()
}

func (t *drmTimelines) create() (drm.Syncobj, int, error) {
	s, err := t.dev.CreateSyncobj(false)
	if err != nil {
		return 0, -1, err
	}
	fd, err := t.dev.ExportSyncobj(s)
	if err != nil {
		return 0, -1, errors.Join(err, t.dev.DestroySyncobj(s))
	}
	return s, fd, nil
}

func (t *drmTimelines) acquire(s drm.Syncobj, point uint64, syncFile int) error {
	if syncFile < 0 {
		return t.dev.Signal(s, point)
	}
	return t.dev.ImportSyncFile(s, point, syncFile)
}

func (t *drmTimelines) watch(s drm.Syncobj, point uint64, released func()) error {
	efd, ok := t.fds[s]
	if !ok {
		var err error
		if efd, err = unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK); err != nil {
			return fmt.Errorf("app: eventfd for a release point: %w", err)
		}
		if err := unix.EpollCtl(t.epfd, unix.EPOLL_CTL_ADD, efd, &unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(efd)}); err != nil { //nolint:gosec // file descriptors fit an int32
			return errors.Join(fmt.Errorf("app: watch a release point: %w", err), unix.Close(efd))
		}
		t.fds[s] = efd
	}
	t.mu.Lock()
	t.released[efd] = released
	t.mu.Unlock()
	return t.dev.EventFD(s, point, efd, false)
}

func (t *drmTimelines) destroy(s drm.Syncobj) {
	if t.closed {
		return
	}
	if efd, ok := t.fds[s]; ok {
		delete(t.fds, s)
		t.mu.Lock()
		delete(t.released, efd)
		t.mu.Unlock()
		if err := errors.Join(unix.EpollCtl(t.epfd, unix.EPOLL_CTL_DEL, efd, nil), unix.Close(efd)); err != nil {
			logutil.L().Warn("app: stop watching a timeline", "err", err)
		}
	}
	if err := t.dev.DestroySyncobj(s); err != nil {
		logutil.L().Warn("app: destroy a timeline", "err", err)
	}
}

func (t *drmTimelines) run() {
	var events [16]unix.EpollEvent
	var count [8]byte
	for {
		n, err := unix.EpollWait(t.epfd, events[:], -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			logutil.L().Error("app: wait on release points; buffers stay busy", "err", err)
			return
		}
		for _, ev := range events[:n] {
			fd := int(ev.Fd)
			if fd == t.stop {
				return
			}
			if _, err := unix.Read(fd, count[:]); err != nil {
				continue
			}
			t.mu.Lock()
			released, invoke := t.released[fd], t.invoke
			delete(t.released, fd)
			t.mu.Unlock()
			if released != nil && invoke != nil {
				invoke(released)
			}
		}
	}
}

func (t *drmTimelines) close() {
	if t.closed {
		return
	}
	var one [8]byte
	binary.NativeEndian.PutUint64(one[:], 1)
	if _, err := unix.Write(t.stop, one[:]); err != nil {
		logutil.L().Error("app: stop the release watch", "err", err)
		return
	}
	t.watcher.Wait()
	for s := range t.fds {
		t.destroy(s)
	}
	t.closed = true
	if err := errors.Join(unix.Close(t.stop), unix.Close(t.epfd), t.dev.Close()); err != nil {
		logutil.L().Warn("app: close the timelines", "err", err)
	}
}

func explicitTimelines(dev *gpu.Device) syncTimelines {
	node, ok := dev.RenderNode()
	if !ok {
		logutil.L().Info("app: no explicit synchronization; surfaces sync implicitly", "device", dev.Name(), "why", "the driver names no render node")
		return nil
	}
	if !dev.SyncFiles() {
		logutil.L().Info("app: no explicit synchronization; surfaces sync implicitly", "device", dev.Name(), "why", "the device has no sync_files")
		return nil
	}
	t, err := openTimelines(node)
	if err != nil {
		logutil.L().Info("app: no explicit synchronization; surfaces sync implicitly", "node", node, "err", err)
		return nil
	}
	return t
}
