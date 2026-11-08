package core

import (
	"encoding/json"
	"testing"
)

func TestEncodeCallPreservesRequest(t *testing.T) {
	body, err := encodeCall(42, "test.method", map[string]any{"text": "a\"b", "n": 3})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Text string `json:"text"`
			N    int    `json:"n"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.JSONRPC != "2.0" || got.ID != 42 || got.Method != "test.method" || got.Params.Text != "a\"b" || got.Params.N != 3 {
		t.Fatalf("request = %+v", got)
	}
	if _, err := encodeCall(1, "invalid", make(chan int)); err == nil {
		t.Fatal("unsupported params should return an error")
	}
}
