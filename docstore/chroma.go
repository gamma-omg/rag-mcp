package docstore

import (
	"context"
	"fmt"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

type ChromaStore struct {
	results int
	col     chroma.Collection
}

const (
	FilePath = "file_path"
	FileCrc  = "file_crc"
)

func (ds *ChromaStore) Injest(ctx context.Context, doc Doc) error {
	return ds.col.Add(ctx,
		chroma.WithTexts(doc.Chunks...),
		chroma.WithIDGenerator(chroma.NewULIDGenerator()),
		chroma.WithMetadatas(
			chroma.NewDocumentMetadata(chroma.NewStringAttribute(FilePath, doc.File)),
			chroma.NewDocumentMetadata(chroma.NewIntAttribute(FileCrc, int64(doc.Crc))),
		),
	)
}

func (ds *ChromaStore) Retrieve(ctx context.Context, query string) ([]SearchResult, error) {
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
		file, _ := metadatas[i].GetString(FilePath)
		res = append(res, SearchResult{
			Text:  doc.ContentString(),
			File:  file,
			Score: float32(scores[i]),
		})
	}

	return res, nil
}

func (ds *ChromaStore) Forget(ctx context.Context, doc InjestedDoc) error {
	err := ds.col.Delete(ctx, chroma.WithWhereDelete(chroma.EqString(FilePath, doc.File)))
	if err != nil {
		return fmt.Errorf("failed to forget doc %s: %w", doc.File, err)
	}

	return nil
}

func (ds *ChromaStore) GetInjestedDocs(ctx context.Context) ([]InjestedDoc, error) {
	res, err := ds.col.Get(ctx)
	if err != nil {
		return nil, err
	}

	var docs []InjestedDoc
	seen := make(map[InjestedDoc]struct{})
	metadata := res.GetMetadatas()

	for _, meta := range metadata {
		path, _ := meta.GetString(FilePath)
		crc, _ := meta.GetInt(FileCrc)
		doc := InjestedDoc{
			File: path,
			Crc:  uint32(crc),
		}

		if _, ok := seen[doc]; ok {
			continue
		}

		seen[doc] = struct{}{}
		docs = append(docs, doc)
	}

	return docs, nil
}
