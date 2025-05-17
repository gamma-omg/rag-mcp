package main

import (
	"context"
	"fmt"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	mocks "github.com/gamma-omg/rag-mcp/mocks/chroma"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
			out := chunkify(c.input, c.size, c.overlap)
			assert.Equal(t, out, c.output)
		})
	}
}

func Test_Injest(t *testing.T) {
	col := new(mocks.MockCollection)
	store := DocStore{
		chunkSize:    100,
		chunkOverlap: 10,
		results:      1,
		col:          col,
	}

	doc := Doc{
		Text: "Bananas are berries, but strawberries aren't.",
		File: "facts.pdf",
		Crc:  12345,
	}

	col.On("Add", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := store.Injest(context.Background(), doc)
	assert.NoError(t, err)
	col.AssertExpectations(t)
}

func Test_Retrieve(t *testing.T) {
	col := new(mocks.MockCollection)
	store := DocStore{
		chunkSize:    100,
		chunkOverlap: 10,
		results:      1,
		col:          col,
	}

	sr := SearchResult{
		Text:  "A day on Venus is longer than its year.",
		File:  "facts.txt",
		Score: 0.9,
	}

	doc := new(mocks.MockDocument)
	doc.On("ContentString").Return(sr.Text)

	meta := new(mocks.MockDocumentMetadata)
	meta.On("GetString", "file_path").Return(sr.File, true)

	qr := new(mocks.MockQueryResult)
	qr.On("GetMetadatasGroups").Return([]chroma.DocumentMetadatas{{meta}})
	qr.On("GetDistancesGroups").Return([]embeddings.Distances{{embeddings.Distance(0.9)}})
	qr.On("GetDocumentsGroups").Return([]chroma.Documents{{doc}})
	col.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(qr, nil)

	res, err := store.Retrieve(context.Background(), "A day on Venus is longer than its year.")
	assert.NoError(t, err)
	assert.Equal(t, res, []SearchResult{sr})
	col.AssertExpectations(t)
}
