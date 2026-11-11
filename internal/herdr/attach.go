package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"rove/internal/core"
)

// Attach is a pane's view of one core terminal: what the program writes is
// read, what is typed is written, and the size and colors are told to the
// core, which answers the program's queries with them.
type Attach struct {
	cl     *core.Client
	term   string
	ch     uint32
	stream <-chan core.Stream
	mu     sync.Mutex
	buf    []byte
	size   [2]int
	cell   [2]int
	fg     ui.Color
	bg     ui.Color
	cursor ui.Color
	endErr error
	done   chan struct{}
	once   sync.Once
}

// Attach starts viewing a terminal at the given grid size.
func (s *Session) Attach(addr string, cols, rows int, fg, bg, cursor ui.Color, cellW, cellH int) (*Attach, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	facts := displayFacts(fg, bg, cursor, cellW, cellH)
	ch, _, err := s.Core.Attach(ctx, addr, cols, rows, facts)
	if err != nil {
		return nil, err
	}
	stream, err := s.Core.Stream(ch)
	if err != nil {
		return nil, err
	}
	return &Attach{cl: s.Core, term: addr, ch: ch, stream: stream, done: make(chan struct{}), fg: fg, bg: bg, cursor: cursor, cell: [2]int{cellW, cellH}}, nil
}

func displayFacts(fg, bg, cursor ui.Color, cellW, cellH int) map[string]any {
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	return map[string]any{"fg": hex(fg), "bg": hex(bg), "cursor": hex(cursor), "cell": [2]int{cellW, cellH}}
}

// Read returns what the program wrote, blocking until there is some.
func (a *Attach) Read(p []byte) (int, error) {
	for {
		a.mu.Lock()
		if len(a.buf) > 0 {
			n := copy(p, a.buf)
			a.buf = a.buf[n:]
			a.mu.Unlock()
			a.cl.Credit(a.ch, n)
			return n, nil
		}
		if a.endErr != nil {
			err := a.endErr
			a.mu.Unlock()
			return 0, err
		}
		a.mu.Unlock()
		select {
		case <-a.done:
			a.setEnd(io.ErrClosedPipe)
		case item, ok := <-a.stream:
			if !ok {
				a.setEnd(errors.New("terminal stream interrupted"))
				continue
			}
			switch item.Kind {
			case core.FrameData:
				a.mu.Lock()
				a.buf = append(a.buf, item.Data...)
				a.mu.Unlock()
			case core.FrameClose:
				a.setEnd(streamEnd(item))
			}
		}
	}
}

// streamEnd only treats a confirmed final process exit as EOF. A stream can
// also end when a kept program is lost and the core is about to restart it.
func streamEnd(item core.Stream) error {
	if item.Data == nil {
		return errors.New("lost the core connection")
	}
	var close struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(item.Data, &close) == nil {
		switch close.Reason {
		case "exited", "killed":
			return io.EOF
		case "":
		default:
			return fmt.Errorf("terminal stream interrupted: %s", close.Reason)
		}
	}
	return errors.New("terminal stream interrupted")
}

func (a *Attach) setEnd(err error) {
	a.mu.Lock()
	if a.endErr == nil {
		a.endErr = err
	}
	a.mu.Unlock()
}

// EndError reports why the stream ended; io.EOF means a confirmed process exit.
// It is nil until Read observes an end, and is safe to call after the view ends.
func (a *Attach) EndError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.endErr
}

// Write sends what is typed to the program.
func (a *Attach) Write(p []byte) (int, error) {
	a.cl.Input(a.ch, p)
	return len(p), nil
}

// Close ends the view; the program keeps running.
func (a *Attach) Close() error {
	a.once.Do(func() {
		close(a.done)
		a.cl.Detach(a.ch)
	})
	return nil
}

// Resize tells the core the view's grid.
func (a *Attach) Resize(cols, rows int) error {
	a.mu.Lock()
	a.size = [2]int{cols, rows}
	w, h := a.cell[0], a.cell[1]
	a.mu.Unlock()
	a.cl.Resize(a.ch, cols, rows)
	if w > 0 {
		a.cl.Facts(a.ch, displayFacts(a.fg, a.bg, a.cursor, w, h))
	}
	return nil
}

// ResizePixels tells the core the grid and the size of a cell in pixels.
func (a *Attach) ResizePixels(cols, rows, cellW, cellH int) error {
	a.mu.Lock()
	a.size = [2]int{cols, rows}
	a.cell = [2]int{cellW, cellH}
	fg, bg, cursor := a.fg, a.bg, a.cursor
	a.mu.Unlock()
	a.cl.Resize(a.ch, cols, rows)
	a.cl.Facts(a.ch, displayFacts(fg, bg, cursor, cellW, cellH))
	return nil
}

// UseTheme tells the core the colors the view shows.
func (a *Attach) UseTheme(theme *terminal.Theme) error {
	a.mu.Lock()
	a.fg = theme.Foreground
	a.bg = theme.Background
	a.cursor = theme.Cursor
	if a.cursor.R == 0 && a.cursor.G == 0 && a.cursor.B == 0 {
		a.cursor = theme.Foreground
	}
	cell := a.cell
	a.mu.Unlock()
	if cell[0] > 0 {
		a.cl.Facts(a.ch, displayFacts(a.fg, a.bg, a.cursor, cell[0], cell[1]))
	}
	return nil
}

// Focus tells the core whether this view has the keyboard.
func (a *Attach) Focus(focused bool) { a.cl.Focus(a.ch, focused) }
