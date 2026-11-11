package core

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
)

func testStreamClient() *Client {
	return &Client{streams: map[uint32]chan Stream{}, early: map[uint32][]Stream{}, replies: map[uint64]chan reply{}, done: make(chan struct{})}
}

func TestStreamEndSurvivesFullQueue(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire []byte
		want []byte
	}{
		{"connection lost", nil, nil},
		{"process exited", encode(7, FrameClose, []byte(`{"reason":"exited"}`)), []byte(`{"reason":"exited"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testStreamClient()
			ch := make(chan Stream, 2)
			c.TakeStream(7, ch)
			ch <- Stream{Kind: FrameData, Data: []byte("one")}
			ch <- Stream{Kind: FrameData, Data: []byte("two")}
			c.readLoop(bufio.NewReader(bytes.NewReader(tc.wire)))
			found := false
			for len(ch) > 0 {
				item := <-ch
				if item.Kind == FrameClose && bytes.Equal(item.Data, tc.want) && (item.Data == nil) == (tc.want == nil) {
					found = true
				}
			}
			if !found {
				t.Fatalf("end marker %q was dropped", tc.want)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func (failingWriter) Close() error              { return nil }

func TestWriteFailureSignalsConnectionLoss(t *testing.T) {
	c := testStreamClient()
	c.stdin = failingWriter{}
	c.writes = make(chan []byte, 1)
	ch := make(chan Stream, 1)
	c.TakeStream(7, ch)
	c.writes <- []byte("frame")
	c.writeLoop()
	select {
	case <-c.Done():
	default:
		t.Fatal("write failure did not close the connection")
	}
	if item := <-ch; item.Kind != FrameClose || item.Data != nil {
		t.Fatalf("write failure sent %#v, want connection loss", item)
	}
}

func TestStreamRegisteredAfterCloseOrLoss(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire []byte
		want []byte
	}{
		{"lost before registration", nil, nil},
		{"close before registration", encode(7, FrameClose, []byte(`{"reason":"exited"}`)), []byte(`{"reason":"exited"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testStreamClient()
			c.readLoop(bufio.NewReader(bytes.NewReader(tc.wire)))
			ch := make(chan Stream, 2)
			c.TakeStream(7, ch)
			got, err := c.Stream(7)
			if err != nil || got != ch {
				t.Fatalf("Stream after registration = %v, %v", got, err)
			}
			select {
			case item := <-ch:
				if item.Kind != FrameClose || !bytes.Equal(item.Data, tc.want) || (item.Data == nil) != (tc.want == nil) {
					t.Fatalf("end = %#v, want %q", item, tc.want)
				}
			default:
				t.Fatal("missing end marker")
			}
		})
	}
}
