package input

import "testing"

var phase15EncodedInput []byte

func BenchmarkPhase15InputEncode(b *testing.B) {
	events := []Event{
		{Rune: 'a'},
		{Rune: 'é'},
		{Rune: 'x', Mods: ModAlt},
		{Key: KeyUp},
		{Key: KeyLeft, Mods: ModCtrl | ModShift},
		{Key: KeyF12, Mods: ModAlt},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		encoded, ok := Encode(events[index%len(events)])
		if !ok {
			b.Fatal("representative input stopped encoding")
		}
		phase15EncodedInput = encoded
	}
}
