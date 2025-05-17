package main

import (
	"context"
	"fmt"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

type DocStore struct {
	chunkSize    int
	chunkOverlap int
	results      int
	col          chroma.Collection
}

type Doc struct {
	Text string
	File string
}

func (ds *DocStore) Injest(ctx context.Context, doc Doc) error {
	chunks := chunkify(doc.Text, ds.chunkSize, ds.chunkOverlap)
	return ds.col.Add(ctx,
		chroma.WithIDGenerator(chroma.NewULIDGenerator()),
		chroma.WithTexts(chunks...),
		chroma.WithMetadatas(
			chroma.NewDocumentMetadata(chroma.NewStringAttribute("file_path", doc.File)),
		),
	)
}

func (ds *DocStore) Retrieve(ctx context.Context, query string) ([]string, error) {
	r, err := ds.col.Query(ctx,
		chroma.WithQueryTexts(query),
		chroma.WithNResults(ds.results),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve texts: %w", err)
	}

	res := make([]string, 0, ds.results)
	for _, docs := range r.GetDocumentsGroups() {
		for _, doc := range docs {
			res = append(res, doc.ContentString())
		}
	}

	return res, nil
}

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
