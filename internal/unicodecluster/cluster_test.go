package unicodecluster

import "testing"

func TestSegmentRepresentativeClusters(t *testing.T) {
	tests := []struct {
		input string
		text  string
		width int
		emoji bool
	}{
		{input: "e\u0301", text: "e\u0301", width: 1, emoji: false},
		{input: "❤️", text: "❤️", width: 2, emoji: true},
		{input: "✍️", text: "✍️", width: 2, emoji: true},
		{input: "👩\u200d💻", text: "👩\u200d💻", width: 2, emoji: true},
		{input: "🧑🏽\u200d🚀", text: "🧑🏽\u200d🚀", width: 2, emoji: true},
		{input: "1️⃣", text: "1️⃣", width: 2, emoji: true},
		{input: "🇦🇷", text: "🇦🇷", width: 2, emoji: true},
		{input: "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", text: "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", width: 2, emoji: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cluster, ok := First(tt.input)
			if !ok {
				t.Fatalf("First(%q) failed", tt.input)
			}
			if cluster.Text != tt.text || cluster.Width != tt.width || cluster.IsEmoji != tt.emoji {
				t.Fatalf("First(%q) = %#v", tt.input, cluster)
			}
		})
	}
}

func TestSegmentSeparatesPlainText(t *testing.T) {
	clusters := Segment("ab")
	if len(clusters) != 2 || clusters[0].Text != "a" || clusters[1].Text != "b" {
		t.Fatalf("Segment plain text = %#v", clusters)
	}
}

func TestIsFlagString(t *testing.T) {
	if !IsFlagString("🇦🇷") {
		t.Fatalf("regional indicator pair should be a flag")
	}
	if !IsFlagString("🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F") {
		t.Fatalf("tag sequence should be a flag")
	}
	if IsFlagString("👩\u200d💻") {
		t.Fatalf("ZWJ emoji should not be a flag")
	}
}
