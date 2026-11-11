package ui

import (
	"io"
	"sync"

	"github.com/egoist/mygo/plugins/terminal"
)

// peekConnection waits for the view's first measured grid before attaching.
// In particular, no 80×24 snapshot can reach the emulator before it is sized.
type peekConnection struct {
	mu         sync.Mutex
	attachOnce sync.Once
	closeOnce  sync.Once
	ready      chan struct{}
	done       chan struct{}
	conn       peekStream
	err        error
	theme      *terminal.Theme
	open       func(cols, rows, cellW, cellH int) (peekStream, error)
}

type peekStream interface {
	io.ReadWriteCloser
	ResizePixels(cols, rows, cellW, cellH int) error
	UseTheme(*terminal.Theme) error
}

func newPeekConnection(open func(cols, rows, cellW, cellH int) (peekStream, error)) *peekConnection {
	return &peekConnection{ready: make(chan struct{}), done: make(chan struct{}), open: open}
}

func (c *peekConnection) stream() (peekStream, error) {
	select {
	case <-c.done:
		return nil, io.ErrClosedPipe
	case <-c.ready:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return nil, io.ErrClosedPipe
	default:
		return c.conn, c.err
	}
}

func (c *peekConnection) Read(p []byte) (int, error) {
	conn, err := c.stream()
	if err != nil {
		return 0, err
	}
	return conn.Read(p)
}

// EndError returns the attached stream's observed end, even after Close.
func (c *peekConnection) EndError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if stream, ok := c.conn.(interface{ EndError() error }); ok {
		return stream.EndError()
	}
	if c.err != nil {
		return c.err
	}
	return io.ErrClosedPipe
}

func (c *peekConnection) Write(p []byte) (int, error) {
	conn, err := c.stream()
	if err != nil {
		return 0, err
	}
	return conn.Write(p)
}

// ResizePixels is called on the terminal's connection goroutine, after the
// emulator has taken the measured size. The RPC never holds up a UI frame.
func (c *peekConnection) ResizePixels(cols, rows, cellW, cellH int) error {
	first := false
	c.attachOnce.Do(func() {
		first = true
		select {
		case <-c.done:
			c.mu.Lock()
			c.err = io.ErrClosedPipe
			c.mu.Unlock()
			close(c.ready)
			return
		default:
		}
		conn, err := c.open(cols, rows, cellW, cellH)
		c.mu.Lock()
		defer c.mu.Unlock()
		select {
		case <-c.done:
			if conn != nil {
				_ = conn.Close()
			}
			c.err = io.ErrClosedPipe
		default:
			c.conn, c.err = conn, err
			if conn != nil && c.theme != nil {
				_ = conn.UseTheme(c.theme)
			}
		}
		close(c.ready)
	})
	conn, err := c.stream()
	if err != nil || first {
		return err
	}
	return conn.ResizePixels(cols, rows, cellW, cellH)
}

func (c *peekConnection) UseTheme(theme *terminal.Theme) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.theme = theme
	if c.conn != nil {
		return c.conn.UseTheme(theme)
	}
	return nil
}

func (c *peekConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
	return nil
}
