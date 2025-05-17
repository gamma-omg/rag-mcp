package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Chunkify(t *testing.T) {
	var cases = []struct {
		input   string
		size    int
		overlap int
		output  []string
	}{
		{input: "abcdefg", size: 3, overlap: 0, output: []string{"abc", "def", "g"}},
		{input: "abcdefg", size: 3, overlap: 1, output: []string{"abc", "cde", "efg"}},
		{input: "abcdefg", size: 9, overlap: 5, output: []string{"abcdefg"}},
		{input: "", size: 9, overlap: 5, output: []string{}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			out := Chunkify(c.input, c.size, c.overlap)
			assert.Equal(t, c.output, out)
		})
	}
}
