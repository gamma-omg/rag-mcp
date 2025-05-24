package docstore

import (
	"context"
	"errors"
	"fmt"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/commons/http"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

type ChromaStore struct {
	results     int
	requestSize int
	col         chroma.Collection
}

const (
	FilePath = "file_path"
	FileCrc  = "file_crc"
)

type ChromaStoreConfig struct {
	BaseURL       string
	EmbeddingFunc embeddings.EmbeddingFunction
	Results       int
	RequestSize   int
	Reset         bool
}

func NewChromaStore(ctx context.Context, cfg ChromaStoreConfig) (*ChromaStore, error) {
	client, err := chroma.NewHTTPClient(chroma.WithBaseURL(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create chroma http client: %w", err)
	}

	if cfg.Reset {
		var chromaErr *http.ChromaError
		if err := client.DeleteCollection(ctx, "documents"); errors.As(err, &chromaErr) {
			if chromaErr.ErrorCode != 404 {
				return nil, fmt.Errorf("failed to delete chroma collection: %w", err)
			}
		}
	}

	col, err := client.GetOrCreateCollection(ctx, "documents", chroma.WithEmbeddingFunctionCreate(cfg.EmbeddingFunc))
	if err != nil {
		return nil, fmt.Errorf("failed to get chroma collection: %w", err)
	}

	return &ChromaStore{
		results:     cfg.Results,
		requestSize: cfg.RequestSize,
		col:         col,
	}, nil
}

func (ds *ChromaStore) Ingest(ctx context.Context, doc Doc) error {
	var bucket []string
	size := 0
	for _, c := range doc.Chunks {
		chunkSize := len(c)
		if size+chunkSize < ds.requestSize {
			bucket = append(bucket, c)
			size += chunkSize
			continue
		}

		if err := ds.ingestBucket(ctx, bucket, doc); err != nil {
			rollbackErr := ds.rollback(ctx, doc)
			if rollbackErr != nil {
				return fmt.Errorf("failed to ingest bucket %w; and failed to rollback: %v", err, rollbackErr)
			}

			return fmt.Errorf("failed to ingest bucket: %w", err)
		}

		bucket = []string{c}
		size = chunkSize
	}

	err := ds.ingestBucket(ctx, bucket, doc)
	if err != nil {
		return fmt.Errorf("failed to ingest final bucket: %w", err)
	}

	return nil
}

func (ds *ChromaStore) ingestBucket(ctx context.Context, texts []string, doc Doc) error {
	size := len(texts)
	metadatas := make([]chroma.DocumentMetadata, size)
	for i := range size {
		metadatas[i] = chroma.NewDocumentMetadata(
			chroma.NewStringAttribute(FilePath, doc.File),
			chroma.NewIntAttribute(FileCrc, int64(doc.Crc)),
		)
	}

	return ds.col.Add(ctx,
		chroma.WithTexts(texts...),
		chroma.WithIDGenerator(chroma.NewUUIDGenerator()),
		chroma.WithMetadatas(metadatas...))
}

func (ds *ChromaStore) rollback(ctx context.Context, doc Doc) error {
	return ds.col.Delete(ctx, chroma.WithWhereDelete(
		chroma.And(
			chroma.EqString(FilePath, doc.File),
			chroma.EqInt(FileCrc, int(doc.Crc)),
		)))
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

func (ds *ChromaStore) Forget(ctx context.Context, doc IngestedDoc) error {
	err := ds.col.Delete(ctx, chroma.WithWhereDelete(
		chroma.And(
			chroma.EqString(FilePath, doc.File),
			chroma.EqInt(FileCrc, int(doc.Crc)),
		)))
	if err != nil {
		return fmt.Errorf("failed to forget doc %s: %w", doc.File, err)
	}

	return nil
}

func (ds *ChromaStore) GetIngested(ctx context.Context) ([]IngestedDoc, error) {
	res, err := ds.col.Get(ctx, chroma.WithIncludeGet(chroma.IncludeMetadatas))
	if err != nil {
		return nil, err
	}

	var docs []IngestedDoc
	seen := make(map[IngestedDoc]struct{})
	metadata := res.GetMetadatas()

	for _, meta := range metadata {
		path, _ := meta.GetString(FilePath)
		crc, _ := meta.GetFloat(FileCrc) // for some reason file crc gets stored as float64 in Chroma
		doc := IngestedDoc{
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
