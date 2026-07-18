package quickselect

const labelAlphabet = "asdfghjklqwertyuiopzxcvbnm"

// Labels returns deterministic fixed-width ASCII labels. Equal-length labels
// are prefix-free, including when the candidate count crosses an alphabet tier.
func Labels(count int) []string {
	if count <= 0 {
		return nil
	}
	if count > MaxCandidates {
		count = MaxCandidates
	}
	base := len(labelAlphabet)
	width, capacity := 1, base
	for capacity < count {
		width++
		capacity *= base
	}
	labels := make([]string, count)
	for i := range labels {
		value := i
		label := make([]byte, width)
		for pos := width - 1; pos >= 0; pos-- {
			label[pos] = labelAlphabet[value%base]
			value /= base
		}
		labels[i] = string(label)
	}
	return labels
}
