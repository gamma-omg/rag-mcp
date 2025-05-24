package main

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gamma-omg/rag-mcp/docstore"
)

type docStorer interface {
	Ingest(ctx context.Context, doc docstore.Doc) error
	Forget(ctx context.Context, doc docstore.IngestedDoc) error
	GetIngested(ctx context.Context) ([]docstore.IngestedDoc, error)
}

type fileReader interface {
	CanRead(path string) bool
	ReadText(path string) (string, error)
}

type chunkifier interface {
	Chunkify(text string) []string
}

type DocRegistry struct {
	log              *slog.Logger
	root             string
	storer           docStorer
	chunkifier       chunkifier
	readers          []fileReader
	mergeEventsDelay time.Duration
}

type DiskDoc struct {
	File string
	Crc  uint32
}

type diskDocs map[string]DiskDoc
type dbDocs map[string]docstore.IngestedDoc

func (dr *DocRegistry) RegisterReader(readers ...fileReader) {
	dr.readers = append(dr.readers, readers...)
}

func (dr *DocRegistry) Sync(ctx context.Context) error {
	dr.log.Info("syncing documents directory", "root", dr.root)

	err := ensureDir(dr.root)
	if err != nil {
		return fmt.Errorf("failed to create documents directory: %w", err)
	}

	disk, err := dr.collectDocs()
	if err != nil {
		return fmt.Errorf("collect docs from disk: %w", err)
	}

	diskMap := make(diskDocs)
	for _, d := range disk {
		diskMap[d.File] = d
	}

	db, err := dr.storer.GetIngested(ctx)
	if err != nil {
		return fmt.Errorf("collect ingested docs from db: %w", err)
	}

	dbMap := make(dbDocs)
	for _, d := range db {
		dbMap[d.File] = d
	}

	err = dr.ingestNewDocuments(ctx, diskMap, dbMap)
	if err != nil {
		return fmt.Errorf("ingest new documents: %w", err)
	}

	err = dr.forgetRemovedDocuments(ctx, diskMap, dbMap)
	if err != nil {
		return fmt.Errorf("forget removed documents: %w", err)
	}

	dr.log.Info("documents registry synchronized", "root", dr.root)
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

		events := mergeEvents(w.Events, dr.mergeEventsDelay)
		for {
			select {
			case e := <-events:
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

func mergeEvents(in <-chan fsnotify.Event, dt time.Duration) <-chan fsnotify.Event {
	out := make(chan fsnotify.Event)

	go func() {
		defer close(out)

		pending := make(map[string]struct {
			evt   fsnotify.Event
			timer *time.Timer
		})

		flush := func(file string) {
			write := pending[file]
			delete(pending, file)
			out <- write.evt
		}

		for evt := range in {
			e, ok := pending[evt.Name]
			if !ok {
				pending[evt.Name] = struct {
					evt   fsnotify.Event
					timer *time.Timer
				}{
					evt:   evt,
					timer: time.AfterFunc(dt, func() { flush(evt.Name) }),
				}
				continue
			}

			if !e.timer.Stop() {
				<-e.timer.C
			}

			e.timer.Reset(dt)
			e.evt.Op |= evt.Op
			pending[evt.Name] = e
		}

		for _, e := range pending {
			out <- e.evt
		}
	}()

	return out
}

func (dr *DocRegistry) processFsEvent(evt fsnotify.Event) {
	if evt.Op.Has(fsnotify.Write) || evt.Op.Has(fsnotify.Create) {
		dr.log.Debug("fsevent write", "file", evt.Name)

		err := dr.forgetFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write file: failed to forget file", slog.String("error", err.Error()))
			return
		}

		err = dr.ingestFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write file: failed to ingest file", slog.String("error", err.Error()))
			return
		}
	}

	if evt.Op.Has(fsnotify.Rename) {
		dr.log.Debug("fsevent rename", "file", evt.Name)

		err := dr.forgetFile(evt.Name)
		if err != nil {
			dr.log.Warn("failed to handle write rename file", slog.String("error", err.Error()))
		}
		return
	}

	if evt.Op.Has(fsnotify.Remove) {
		dr.log.Debug("fsevent remove", "file", evt.Name)

		err := dr.forgetFile(evt.Name)
		if err != nil {
			slog.Warn("forget file failed", slog.String("error", err.Error()))
		}
	}
}

func (dr *DocRegistry) ingestFile(path string) error {
	reader, err := dr.findReader(path)
	if err != nil {
		dr.log.Warn("unable to ingest file: reader not found", slog.String("file", path))
		return nil
	}

	text, err := reader.ReadText(path)
	if err != nil {
		return fmt.Errorf("ingestFile unable to read %s: %w", path, err)
	}

	rel, err := filepath.Rel(dr.root, path)
	if err != nil {
		return fmt.Errorf("ingestFile invalid file path %s: %w", path, err)
	}

	doc := docstore.Doc{
		File:   rel,
		Crc:    crc32.Checksum([]byte(text), crc32.IEEETable),
		Chunks: dr.chunkifier.Chunkify(text),
	}
	err = dr.storer.Ingest(context.Background(), doc)
	if err != nil {
		return fmt.Errorf("ingestFile failed to store %s content to db: %w", path, err)
	}

	dr.log.Info("document ingested", "file", doc.File, "crc", doc.Crc)
	return nil
}

func (dr *DocRegistry) forgetFile(path string) error {
	docs, err := dr.storer.GetIngested(context.Background())
	if err != nil {
		return fmt.Errorf("forgetFile failed to get ingested files: %w", err)
	}

	rel, err := filepath.Rel(dr.root, path)
	if err != nil {
		return fmt.Errorf("forgetFile failed to get relative path for %s: %w", path, err)
	}

	for _, d := range docs {
		if d.File != rel {
			continue
		}

		err := dr.storer.Forget(context.Background(), d)
		if err != nil {
			return fmt.Errorf("forgetFile failed to remove %s from db: %w", rel, err)
		}

		dr.log.Info("document removed", "file", d.File, "crc", d.Crc)
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

func (dr *DocRegistry) ingestNewDocuments(ctx context.Context, disk diskDocs, db dbDocs) error {
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

		err = dr.storer.Ingest(ctx, docstore.Doc{
			File:   diskDoc.File,
			Crc:    diskDoc.Crc,
			Chunks: dr.chunkifier.Chunkify(text),
		})
		if err != nil {
			return fmt.Errorf("failed to store document %s: %w", diskDoc.File, err)
		}

		dr.log.Info("document ingested", "file", diskDoc.File, "crc", diskDoc.Crc)
	}

	return nil
}

func (dr *DocRegistry) forgetRemovedDocuments(ctx context.Context, disk diskDocs, db dbDocs) error {
	for _, dbDoc := range db {
		diskDoc, ok := disk[dbDoc.File]
		if ok && diskDoc.Crc == dbDoc.Crc {
			continue
		}

		err := dr.storer.Forget(ctx, dbDoc)
		if err != nil {
			return fmt.Errorf("failed to remove document %s from store: %w", dbDoc.File, err)
		}

		dr.log.Info("document removed", "file", dbDoc.File, "crc", dbDoc.Crc)
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

func ensureDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}

	if err != nil {
		return fmt.Errorf("failed to get directory info: %w", err)
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}

	return nil
}
