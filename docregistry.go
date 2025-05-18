package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/gamma-omg/rag-mcp/docstore"
)

type DocStore interface {
	Injest(ctx context.Context, doc docstore.Doc) error
	Retrieve(ctx context.Context, query string) ([]docstore.SearchResult, error)
	Forget(ctx context.Context, doc docstore.InjestedDoc) error
	GetInjested(ctx context.Context) ([]docstore.InjestedDoc, error)
}

type FileReader interface {
	CanRead(path string) bool
	ReadText(path string) (string, error)
}

type Chunkifier interface {
	Chunkify(text string) []string
}

type DocRegistry struct {
	log        *slog.Logger
	root       string
	store      DocStore
	chunkifier Chunkifier
	readers    []FileReader
}

type DiskDoc struct {
	File string
	Crc  uint32
}

type diskDocs map[string]DiskDoc
type dbDocs map[string]docstore.InjestedDoc

func (dr *DocRegistry) RegisterReader(readers ...FileReader) {
	dr.readers = append(dr.readers, readers...)
}

func (dr *DocRegistry) Sync(ctx context.Context) error {
	disk, err := dr.collectDocs()
	if err != nil {
		return err
	}

	diskMap := make(diskDocs)
	for _, d := range disk {
		diskMap[d.File] = d
	}

	db, err := dr.store.GetInjested(ctx)
	if err != nil {
		return err
	}

	dbMap := make(dbDocs)
	for _, d := range db {
		dbMap[d.File] = d
	}

	err = dr.injestNewDocuments(ctx, diskMap, dbMap)
	if err != nil {
		return err
	}

	err = dr.forgetRemovedDocuments(ctx, diskMap, dbMap)
	if err != nil {
		return err
	}

	return nil
}

func (dr *DocRegistry) collectDocs() (docs []DiskDoc, err error) {
	err = filepath.Walk(dr.root, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		reader, e := dr.findReader(path)
		if e != nil {
			dr.log.Warn(fmt.Sprintf("unsupported file: %s", path))
			return nil
		}

		text, e := reader.ReadText(path)
		if e != nil {
			return e
		}

		rel, e := filepath.Rel(dr.root, path)
		if e != nil {
			return err
		}

		docs = append(docs, DiskDoc{
			File: rel,
			Crc:  crc32.Checksum([]byte(text), crc32.IEEETable),
		})

		return nil
	})
	if err != nil {
		return
	}

	return
}

func (dr *DocRegistry) injestNewDocuments(ctx context.Context, disk diskDocs, db dbDocs) error {
	for _, diskDoc := range disk {
		dbDoc, ok := db[diskDoc.File]
		if ok && dbDoc.Crc == diskDoc.Crc {
			continue
		}

		reader, err := dr.findReader(diskDoc.File)
		if err != nil {
			return fmt.Errorf("failed to find reader for document %s: %w", diskDoc.File, err)
		}

		text, err := reader.ReadText(filepath.Join(dr.root, diskDoc.File))
		if err != nil {
			return fmt.Errorf("failed to read document %s: %w", diskDoc.File, err)
		}

		err = dr.store.Injest(ctx, docstore.Doc{
			File:   diskDoc.File,
			Crc:    diskDoc.Crc,
			Chunks: dr.chunkifier.Chunkify(text),
		})
		if err != nil {
			return fmt.Errorf("failed to store document %s: %w", diskDoc.File, err)
		}
	}

	return nil
}

func (dr *DocRegistry) forgetRemovedDocuments(ctx context.Context, disk diskDocs, db dbDocs) error {
	for _, dbDoc := range db {
		diskDoc, ok := disk[dbDoc.File]
		if ok && diskDoc.Crc == dbDoc.Crc {
			continue
		}

		err := dr.store.Forget(ctx, dbDoc)
		if err != nil {
			return fmt.Errorf("failed to remove document %s from store: %w", dbDoc.File, err)
		}
	}

	return nil
}

func (dr *DocRegistry) findReader(file string) (FileReader, error) {
	for _, r := range dr.readers {
		if r.CanRead(file) {
			return r, nil
		}
	}

	return nil, fmt.Errorf("unable to find reader for file: %s", file)
}
