package app

// OpenDialogs returns the dialogs a test cannot reach through the API
// that opened them, oldest first.
func (a *Application) OpenDialogs() []*Dialog { return a.dialogs }

// SetVideoMemory makes a's Video surfaces take their buffers from alloc
// instead of /dev/udmabuf: an fd the compositor imports, its mapping,
// and how to free both. Writes through the mapping need no bracket.
func (a *Application) SetVideoMemory(alloc func(size int) (fd int, mem []byte, free func() error, err error)) {
	a.videoMemory = func(size int) (videoMemory, error) {
		fd, mem, free, err := alloc(size)
		if err != nil {
			return nil, err
		}
		return plainMemory{fdv: fd, mem: mem, free: free}, nil
	}
}

type plainMemory struct {
	fdv  int
	mem  []byte
	free func() error
}

func (m plainMemory) fd() int           { return m.fdv }
func (m plainMemory) bytes() []byte     { return m.mem }
func (m plainMemory) beginWrite() error { return nil }
func (m plainMemory) endWrite() error   { return nil }
func (m plainMemory) close() error      { return m.free() }
