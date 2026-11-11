package herdr

import (
	"errors"
	"io"
	"testing"

	"rove/internal/core"
)

func TestAttachDistinguishesProcessExitFromStreamLoss(t *testing.T) {
	for _, tc := range []struct {
		name   string
		item   *core.Stream
		exited bool
	}{
		{"normal exit", &core.Stream{Kind: core.FrameClose, Data: []byte(`{"reason":"exited","exit_code":7}`)}, true},
		{"explicit kill", &core.Stream{Kind: core.FrameClose, Data: []byte(`{"reason":"killed"}`)}, true},
		{"kept process signaled", &core.Stream{Kind: core.FrameClose, Data: []byte(`{"reason":"signaled","finished_ms":null}`)}, false},
		{"holder lost", &core.Stream{Kind: core.FrameClose, Data: []byte(`{"reason":"holder_lost"}`)}, false},
		{"unknown close reason", &core.Stream{Kind: core.FrameClose, Data: []byte(`{"reason":"new_reason"}`)}, false},
		{"empty close", &core.Stream{Kind: core.FrameClose, Data: []byte{}}, false},
		{"core connection lost", &core.Lost, false},
		{"stream closed unexpectedly", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan core.Stream, 1)
			if tc.item != nil {
				ch <- *tc.item
			}
			close(ch)
			a := &Attach{stream: ch, done: make(chan struct{})}
			for range 2 {
				_, err := a.Read(make([]byte, 1))
				if err == nil || errors.Is(err, io.EOF) != tc.exited {
					t.Fatalf("Read error = %v, exited = %v", err, tc.exited)
				}
				if a.EndError() != err {
					t.Fatalf("EndError = %v, Read error = %v", a.EndError(), err)
				}
			}
		})
	}
}
