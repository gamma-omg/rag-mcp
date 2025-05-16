package main

func chunkify(text string, size int, overlap int) []string {
	l := len(text)
	if l == 0 {
		return []string{}
	}

	step := size - overlap
	pos := 0
	res := make([]string, 0, l/step+1)

	for {
		end := min(pos+size, l)
		res = append(res, text[pos:end])
		if end >= l {
			break
		}

		pos += step
	}

	return res
}
