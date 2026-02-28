package markers

func SplitLinesKeepEOL(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}

	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			out = append(out, b[start:i+1])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
