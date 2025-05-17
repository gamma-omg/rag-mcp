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
	Crc  int
}

type SearchResult struct {
	Text  string
	File  string
	Score float32
}

type InjestedDoc struct {
	File string
	Crc  int
}

func (ds *DocStore) Injest(ctx context.Context, doc Doc) error {
	chunks := chunkify(doc.Text, ds.chunkSize, ds.chunkOverlap)
	return ds.col.Add(ctx,
		chroma.WithTexts(chunks...),
		chroma.WithIDGenerator(chroma.NewULIDGenerator()),
		chroma.WithMetadatas(
			chroma.NewDocumentMetadata(chroma.NewStringAttribute("file_path", doc.File)),
			chroma.NewDocumentMetadata(chroma.NewIntAttribute("file_crc", int64(doc.Crc))),
		),
	)
}

func (ds *DocStore) Retrieve(ctx context.Context, query string) ([]SearchResult, error) {
	r, err := ds.col.Query(ctx,
		chroma.WithQueryTexts(query),
		chroma.WithNResults(ds.results),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve texts: %w", err)
	}

	res := make([]SearchResult, 0, ds.results)
	docs := r.GetDocumentsGroups()[0]
	metadatas := r.GetMetadatasGroups()[0]
	scores := r.GetDistancesGroups()[0]
	r.GetDistancesGroups()
	for i := range len(docs) {
		doc := docs[i]
		file, _ := metadatas[i].GetString("file_path")
		res = append(res, SearchResult{
			Text:  doc.ContentString(),
			File:  file,
			Score: float32(scores[i]),
		})
	}

	return res, nil
}

func (ds *DocStore) GetInjestedDocs(ctx context.Context) ([]InjestedDoc, error) {
	res, err := ds.col.Get(ctx)
	if err != nil {
		return nil, err
	}

	var docs []InjestedDoc
	metadata := res.GetMetadatas()
	for _, meta := range metadata {
		path, _ := meta.GetString("file_path")
		crc, _ := meta.GetInt("file_crc")
		docs = append(docs, InjestedDoc{
			File: path,
			Crc:  int(crc),
		})
	}

	return docs, nil
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
