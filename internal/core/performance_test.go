package core

import "testing"

func BenchmarkRPCEncode(b *testing.B) {
	params := map[string]any{"terminal": "machine/t42", "cols": 120, "rows": 30}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := encodeCall(42, "terminal.attach", params); err != nil {
			b.Fatal(err)
		}
	}
}
