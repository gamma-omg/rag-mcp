package main

type DefaultChunkfier struct {
	chunkSize    int
	chunkOverlap int
}

func (c *DefaultChunkfier) Chunkify(text string) []string {
	l := len(text)
	if l == 0 {
		return []string{}
	}

	step := c.chunkSize - c.chunkOverlap
	pos := 0
	res := make([]string, 0, l/step+1)

	for {
		end := min(pos+c.chunkSize, l)
		res = append(res, text[pos:end])
		if end >= l {
			break
		}

		pos += step
	}

	return res
}
