// Package core speaks to herdr-core through `hc connect`: JSON-RPC on
// channel 0, terminal byte streams on channels of their own. It is the same
// framed protocol the Herdr app uses (herdr-core schema/README.md).
package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Frame kinds.
const (
	FrameData   = 0
	FrameCredit = 1
	FrameClose  = 2
	FrameResize = 3
	FrameFocus  = 4
	FrameFacts  = 5
)

// Stream is one item of a stream channel: output bytes, the end of the
// stream with the core's reason, or the connection to the core dropping.
type Stream struct {
	Kind   byte // FrameData, FrameClose, or FrameClose with Data nil for a lost core
	Data   []byte
	Reason json.RawMessage
}

// Lost is a stream item reporting that the connection to the core dropped.
var Lost = Stream{Kind: FrameClose}

type reply struct {
	result json.RawMessage
	err    error
}

// Error is a JSON-RPC error from the core.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "core call failed"
}

// Kind returns the error's stable kind, such as "not_found" or "conflict".
func (e *Error) Kind() string {
	var d struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(e.Data, &d)
	return d.Kind
}

// Client is a connection to one herdr-core.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex

	writes chan []byte

	mu      sync.Mutex
	replies map[uint64]chan reply
	streams map[uint32]chan Stream
	early   map[uint32][]Stream
	closed  bool
	nextID  atomic.Uint64

	done chan struct{}

	// Hello is what the core said as the connection opened.
	Hello json.RawMessage
	// OnNotify sees channel-0 notifications that carry no reply (heartbeat,
	// missed, and event lines of an unframed subscription).
	OnNotify func(json.RawMessage)
}

// Connect starts `hc connect` and switches it to framed mode.
func Connect(bin string) (*Client, error) {
	if bin == "" {
		var err error
		bin, err = findHC()
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(bin, "connect")
	configureHCProcess(cmd)
	cmd.Env = hcEnvironment(cmd.Path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot run hc connect: %w", err)
	}
	reader := bufio.NewReaderSize(stdout, 1<<16)
	if _, err := io.WriteString(stdin, `{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true}}`+"\n"); err != nil {
		return nil, err
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("cannot read hello: %w", err)
	}
	var hello struct {
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(line, &hello); err != nil {
		return nil, fmt.Errorf("bad hello reply: %w", err)
	}
	if hello.Error != nil {
		return nil, fmt.Errorf("core refused hello: %w", hello.Error)
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		writes:  make(chan []byte, 256),
		replies: map[uint64]chan reply{},
		streams: map[uint32]chan Stream{},
		early:   map[uint32][]Stream{},
		done:    make(chan struct{}),
		Hello:   hello.Result,
	}
	go c.writeLoop()
	go c.readLoop(reader)
	return c, nil
}

func (c *Client) writeLoop() {
	for frame := range c.writes {
		if _, err := c.stdin.Write(frame); err != nil {
			break
		}
	}
}

func encode(channel uint32, kind byte, payload []byte) []byte {
	out := make([]byte, 9+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], channel)
	out[4] = kind
	binary.LittleEndian.PutUint32(out[5:9], uint32(len(payload)))
	copy(out[9:], payload)
	return out
}

func (c *Client) send(channel uint32, kind byte, payload []byte) {
	select {
	case c.writes <- encode(channel, kind, payload):
	case <-c.done:
	}
}

func (c *Client) readLoop(reader *bufio.Reader) {
	header := make([]byte, 9)
	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			c.lost()
			return
		}
		channel := binary.LittleEndian.Uint32(header[0:4])
		kind := header[4]
		size := int(binary.LittleEndian.Uint32(header[5:9]))
		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			c.lost()
			return
		}
		if channel == 0 {
			if kind != FrameData {
				continue
			}
			var msg struct {
				ID     *uint64         `json:"id"`
				Method string          `json:"method"`
				Result json.RawMessage `json:"result"`
				Error  *Error          `json:"error"`
			}
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}
			if msg.ID == nil {
				if c.OnNotify != nil {
					c.OnNotify(payload)
				}
				continue
			}
			c.mu.Lock()
			ch := c.replies[*msg.ID]
			delete(c.replies, *msg.ID)
			c.mu.Unlock()
			if ch == nil {
				continue
			}
			if msg.Error != nil {
				ch <- reply{err: msg.Error}
			} else {
				ch <- reply{result: msg.Result}
			}
			continue
		}
		item := Stream{Kind: kind, Data: payload}
		if kind == FrameClose {
			item.Reason = payload
		}
		c.mu.Lock()
		ch := c.streams[channel]
		if ch == nil {
			c.early[channel] = append(c.early[channel], item)
		}
		if kind == FrameClose && ch != nil {
			delete(c.streams, channel)
		}
		c.mu.Unlock()
		if ch != nil {
			select {
			case ch <- item:
			default:
			}
		}
	}
}

func (c *Client) lost() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	streams := c.streams
	c.streams = map[uint32]chan Stream{}
	replies := c.replies
	c.replies = map[uint64]chan reply{}
	c.mu.Unlock()
	close(c.done)
	for _, ch := range streams {
		select {
		case ch <- Lost:
		default:
		}
	}
	for _, ch := range replies {
		select {
		case ch <- reply{err: errors.New("lost the core")}:
		default:
		}
	}
}

// Binary returns the executable used for this connection, so programs started
// through an existing core need not find hc on that core's inherited PATH.
func (c *Client) Binary() string { return c.cmd.Path }

// Done closes when the connection to the core is gone.
func (c *Client) Done() <-chan struct{} { return c.done }

func encodeCall(id uint64, method string, params any) ([]byte, error) {
	return json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}

// Call runs one JSON-RPC method and waits for its result.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if params == nil {
		params = struct{}{}
	}
	id := c.nextID.Add(1)
	body, err := encodeCall(id, method, params)
	if err != nil {
		return nil, err
	}

	ch := make(chan reply, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("lost the core")
	}
	c.replies[id] = ch
	c.mu.Unlock()

	c.send(0, FrameData, body)
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.result, r.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.replies, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-timer.C:
		c.mu.Lock()
		delete(c.replies, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s: no reply from core", method)
	}
}

// Subscribe opens the event stream and returns its channel and snapshot.
func (c *Client) Subscribe(ctx context.Context) (uint32, json.RawMessage, error) {
	raw, err := c.Call(ctx, "events.subscribe", map[string]any{})
	if err != nil {
		return 0, nil, err
	}
	var res struct {
		Channel uint32          `json:"channel"`
		Seq     uint64          `json:"seq"`
		Term    json.RawMessage `json:"terminals"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, nil, err
	}
	ch := make(chan Stream, 4096)
	c.mu.Lock()
	for _, item := range c.early[res.Channel] {
		ch <- item
	}
	delete(c.early, res.Channel)
	c.streams[res.Channel] = ch
	c.mu.Unlock()
	c.Credit(res.Channel, 1<<20)
	return res.Channel, raw, nil
}

// Streams returns the channel's items, once its stream is open.
func (c *Client) Stream(channel uint32) (<-chan Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.streams[channel]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("no stream on channel %d", channel)
}

// Attach starts viewing a terminal and returns its channel and record.
func (c *Client) Attach(ctx context.Context, terminal string, cols, rows int, display any) (uint32, json.RawMessage, error) {
	raw, err := c.Call(ctx, "terminal.attach", map[string]any{
		"terminal": terminal, "cols": cols, "rows": rows, "history": 2000, "display": display,
	})
	if err != nil {
		return 0, nil, err
	}
	var res struct {
		Channel uint32          `json:"channel"`
		Term    json.RawMessage `json:"terminal"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, nil, err
	}
	ch := make(chan Stream, 4096)
	c.mu.Lock()
	for _, item := range c.early[res.Channel] {
		ch <- item
	}
	delete(c.early, res.Channel)
	c.streams[res.Channel] = ch
	c.mu.Unlock()
	return res.Channel, res.Term, nil
}

// TakeStream registers a stream reading goroutine's channel and returns the
// buffered frames that arrived before it.
func (c *Client) TakeStream(channel uint32, ch chan Stream) {
	c.mu.Lock()
	for _, item := range c.early[channel] {
		select {
		case ch <- item:
		default:
		}
	}
	delete(c.early, channel)
	c.streams[channel] = ch
	c.mu.Unlock()
}

// Input writes bytes to a stream, as the keys a terminal's program reads.
func (c *Client) Input(channel uint32, b []byte) {
	if len(b) > 0 {
		c.send(channel, FrameData, b)
	}
}

// Credit tells the core how many bytes of a stream were consumed.
func (c *Client) Credit(channel uint32, n int) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(n))
	c.send(channel, FrameCredit, b[:])
}

// Resize tells the core (and the program) the view's grid.
func (c *Client) Resize(channel uint32, cols, rows int) {
	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:2], uint16(cols))
	binary.LittleEndian.PutUint16(b[2:4], uint16(rows))
	c.send(channel, FrameResize, b[:])
}

// Facts tells the core the viewer's colors and cell size.
func (c *Client) Facts(channel uint32, display any) {
	b, err := json.Marshal(display)
	if err == nil {
		c.send(channel, FrameFacts, b)
	}
}

// Focus tells the core whether the viewer holds the keyboard focus.
func (c *Client) Focus(channel uint32, focused bool) {
	var b byte
	if focused {
		b = 1
	}
	c.send(channel, FrameFocus, []byte{b})
}

// Detach ends the view, letting the program keep running.
func (c *Client) Detach(channel uint32) {
	c.mu.Lock()
	delete(c.streams, channel)
	c.mu.Unlock()
	c.send(channel, FrameClose, nil)
}

// Close ends the connection to the core. Programs keep running.
func (c *Client) Close() {
	close(c.writes)
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}
