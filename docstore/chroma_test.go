package docstore

import (
	"context"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	mocks "github.com/gamma-omg/rag-mcp/mocks/chroma"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_Injest(t *testing.T) {
	col := new(mocks.MockCollection)
	store := ChromaStore{
		results: 1,
		col:     col,
	}

	doc := Doc{
		File:   "facts.pdf",
		Crc:    12345,
		Chunks: []string{"Bananas are berries, but strawberries aren't."},
	}

	col.EXPECT().Add(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, store.Injest(context.Background(), doc))
	col.AssertExpectations(t)
}

func Test_Injest_SplitsToBuckets(t *testing.T) {
	col := new(mocks.MockCollection)
	store := ChromaStore{
		results:     1,
		requestSize: 13,
		col:         col,
	}

	doc := Doc{
		File:   "facts.pdf",
		Crc:    12345,
		Chunks: []string{"Bananas", "are", "berries", "but", "strawberries", "aren't"},
	}

	col.EXPECT().Add(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(4)

	require.NoError(t, store.Injest(context.Background(), doc))
	col.AssertExpectations(t)
}

func Test_Retrieve(t *testing.T) {
	col := new(mocks.MockCollection)
	store := ChromaStore{
		results: 1,
		col:     col,
	}

	sr := SearchResult{
		Text:  "A day on Venus is longer than its year.",
		File:  "facts.txt",
		Score: 0.9,
	}

	doc := new(mocks.MockDocument)
	doc.EXPECT().ContentString().Return(sr.Text)

	meta := new(mocks.MockDocumentMetadata)
	meta.EXPECT().GetString(FilePath).Return(sr.File, true)

	qr := new(mocks.MockQueryResult)
	qr.EXPECT().GetMetadatasGroups().Return([]chroma.DocumentMetadatas{{meta}})
	qr.EXPECT().GetDistancesGroups().Return([]embeddings.Distances{{embeddings.Distance(0.9)}})
	qr.EXPECT().GetDocumentsGroups().Return([]chroma.Documents{{doc}})
	col.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(qr, nil)

	res, err := store.Retrieve(context.Background(), "A day on Venus is longer than its year.")
	require.NoError(t, err)
	assert.Equal(t, res, []SearchResult{sr})
	col.AssertExpectations(t)
}

func Test_Forget(t *testing.T) {
	col := new(mocks.MockCollection)
	store := ChromaStore{
		results: 1,
		col:     col,
	}

	doc := InjestedDoc{
		File: "f1.txt",
		Crc:  123,
	}
	col.EXPECT().Delete(mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, store.Forget(context.Background(), doc))
}

func Test_GetInjestedDocs(t *testing.T) {
	col := new(mocks.MockCollection)
	store := ChromaStore{
		results: 1,
		col:     col,
	}

	meta := new(mocks.MockDocumentMetadata)
	meta.EXPECT().GetString(FilePath).Return("facts.pdf", true)
	meta.EXPECT().GetFloat(FileCrc).Return(float64(12345), true)

	get := new(mocks.MockGetResult)
	get.EXPECT().GetMetadatas().Return(chroma.DocumentMetadatas{meta})

	col.EXPECT().Get(mock.Anything, mock.Anything).Return(get, nil)

	injested, err := store.GetInjested(context.Background())
	require.NoError(t, err)
	assert.Equal(t, injested, []InjestedDoc{{File: "facts.pdf", Crc: 12345}})
	col.AssertExpectations(t)
}
