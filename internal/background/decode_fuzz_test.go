package background

import "testing"

func FuzzDecodeBytesBounded(f *testing.F) {
	f.Add([]byte("not-an-image"))
	f.Add([]byte("GIF89a"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		source, _ := DecodeBytes(0, data, NewBudget())
		if source != nil {
			_ = source.Close()
		}
	})
}
