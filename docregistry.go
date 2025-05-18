package main

import (
	"context"
	"fmt"
	"hash/crc32"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/gamma-omg/rag-mcp/docstore"
)

type docStore interface {
	Injest(ctx context.Context, doc docstore.Doc) error
	Retrieve(ctx context.Context, query string) ([]docstore.SearchResult, error)
	Forget(ctx context.Context, doc docstore.InjestedDoc) error
	GetInjested(ctx context.Context) ([]docstore.InjestedDoc, error)
}

type fileReader interface {
	CanRead(path string) bool
	ReadText(path string) (string, error)
}

type chunkifier interface {
	Chunkify(text string) []string
}

type DocRegistry struct {
	log        *slog.Logger
	root       string
	store      docStore
	chunkifier chunkifier
	readers    []fileReader
}

type DiskDoc struct {
	File string
	Crc  uint32
}

type diskDocs map[string]DiskDoc
type dbDocs map[string]docstore.InjestedDoc

func (dr *DocRegistry) RegisterReader(readers ...fileReader) {
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

func (dr *DocRegistry) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create new watcher: %w", err)
	}

	err = w.Add(dr.root)
	if err != nil {
		w.Close()
		return fmt.Errorf("add %s to watcher: %w", dr.root, err)
	}

	go func() {
		defer w.Close()

		for {
			select {
			case e := <-w.Events:
				dr.processFsEvent(e)
			case e := <-w.Errors:
				dr.log.Error(fmt.Sprintf("error watching docs: %s", e.Error()))
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (dr *DocRegistry) processFsEvent(evt fsnotify.Event) {
	if evt.Op.Has(fsnotify.Create) {
		err := dr.injestFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle create file", slog.String("error", err.Error()))
		}
		return
	}

	if evt.Op.Has(fsnotify.Write) {
		err := dr.forgetFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write file: failed to forget file", slog.String("error", err.Error()))
			return
		}

		err = dr.injestFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write file: failed to injest file", slog.String("error", err.Error()))
			return
		}

		return
	}

	if evt.Op.Has(fsnotify.Rename) {
		err := dr.forgetFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write rename file", slog.String("error", err.Error()))
		}
		return
	}

	if evt.Op.Has(fsnotify.Remove) {
		err := dr.forgetFile(evt.Name)
		if err != nil {
			slog.Warn("forget file failed", slog.String("error", err.Error()))
		}
	}
}

func (dr *DocRegistry) injestFile(path string) error {
	reader, err := dr.findReader(path)
	if err != nil {
		dr.log.Warn("unable to injest file: reader not found", slog.String("file", path))
		return nil
	}

	text, err := reader.ReadText(path)
	if err != nil {
		return fmt.Errorf("injestFile unable to read %s: %w", path, err)
	}

	rel, err := filepath.Rel(dr.root, path)
	if err != nil {
		return fmt.Errorf("injestFile invalid file path %s: %w", path, err)
	}

	err = dr.store.Injest(context.Background(), docstore.Doc{
		File:   rel,
		Crc:    crc32.Checksum([]byte(text), crc32.IEEETable),
		Chunks: dr.chunkifier.Chunkify(text),
	})
	if err != nil {
		return fmt.Errorf("injestFile failed to store %s content to db: %w", path, err)
	}

	return nil
}

func (dr *DocRegistry) forgetFile(path string) error {
	docs, err := dr.store.GetInjested(context.Background())
	if err != nil {
		return fmt.Errorf("forgetFile failed to get injested files: %w", err)
	}

	rel, err := filepath.Rel(dr.root, path)
	if err != nil {
		return fmt.Errorf("forgetFile failed to get relative path for %s: %w", path, err)
	}

	for _, d := range docs {
		if d.File != rel {
			continue
		}

		err := dr.store.Forget(context.Background(), d)
		if err != nil {
			return fmt.Errorf("forgetFile failed to remove %s from db: %w", rel, err)
		}
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

func (dr *DocRegistry) findReader(file string) (fileReader, error) {
	for _, r := range dr.readers {
		if r.CanRead(file) {
			return r, nil
		}
	}

	return nil, fmt.Errorf("unable to find reader for file: %s", file)
}
