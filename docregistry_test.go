package main

import (
	"context"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gamma-omg/rag-mcp/docstore"
	mocks "github.com/gamma-omg/rag-mcp/mocks/main"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockTextReader struct{}

func (r *mockTextReader) CanRead(path string) bool { return true }

func (r *mockTextReader) ReadText(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

type fakeDocStore struct {
	ingested     []docstore.IngestedDoc
	ingestCalls  []docstore.Doc
	foregetCalls []docstore.IngestedDoc
}

func (s *fakeDocStore) Ingest(ctx context.Context, doc docstore.Doc) error {
	s.ingested = append(s.ingested, docstore.IngestedDoc{
		File: doc.File,
		Crc:  doc.Crc,
	})
	s.ingestCalls = append(s.ingestCalls, doc)
	return nil
}

func (s *fakeDocStore) Retrieve(ctx context.Context, query string) ([]docstore.SearchResult, error) {
	panic("not implemented")
}

func (s *fakeDocStore) Forget(ctx context.Context, doc docstore.IngestedDoc) error {
	s.ingested = slices.DeleteFunc(s.ingested, func(d docstore.IngestedDoc) bool {
		return d.File == doc.File && d.Crc == doc.Crc
	})
	s.foregetCalls = append(s.foregetCalls, doc)
	return nil
}

func (s *fakeDocStore) GetIngested(ctx context.Context) ([]docstore.IngestedDoc, error) {
	return s.ingested, nil
}

func (s *fakeDocStore) getIngestCalls() []string {
	calls := make([]string, 0, len(s.ingestCalls))
	for _, d := range s.ingestCalls {
		calls = append(calls, d.File)
	}

	return calls
}

func (s *fakeDocStore) getForgetCalls() []string {
	calls := make([]string, 0, len(s.foregetCalls))
	for _, d := range s.foregetCalls {
		calls = append(calls, d.File)
	}

	return calls
}

func Test_Sync(t *testing.T) {
	tmp, err := os.MkdirTemp(os.TempDir(), "test_")
	require.NoError(t, err)

	createFile := func(name string, content string) DiskDoc {
		buff := []byte(content)
		e := os.WriteFile(filepath.Join(tmp, name), buff, 0o644)
		require.NoError(t, e)
		return DiskDoc{
			File: name,
			Crc:  crc32.Checksum(buff, crc32.IEEETable),
		}
	}

	createFile("f1.txt", "f1")
	createFile("f3.pdf", "f3")
	f2 := createFile("f2.txt", "f2")

	store := &fakeDocStore{
		ingested: []docstore.IngestedDoc{
			{File: "f2.txt", Crc: f2.Crc},
			{File: "f3.pdf", Crc: 0},
			{File: "f4.pdf", Crc: 4},
		},
	}

	chunkifier := new(mocks.MockChunkifier)
	chunkifier.EXPECT().Chunkify(mock.Anything).Return([]string{"content"})

	reg := DocRegistry{
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		storer:     store,
		chunkifier: chunkifier,
		root:       tmp,
	}
	reg.RegisterReader(&mockTextReader{})

	require.NoError(t, reg.Sync(context.Background()))

	assert.ElementsMatch(t, []string{"f1.txt", "f3.pdf"}, store.getIngestCalls())
	assert.ElementsMatch(t, []string{"f3.pdf", "f4.pdf"}, store.getForgetCalls())
	chunkifier.AssertExpectations(t)
}

func Test_Watch(t *testing.T) {
	tmp, err := os.MkdirTemp(os.TempDir(), "test_")
	require.NoError(t, err)

	createFile := func(name string, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644))
	}
	removeFile := func(name string) {
		require.NoError(t, os.Remove(filepath.Join(tmp, name)))
	}
	renameFile := func(oldname, newname string) {
		require.NoError(t, os.Rename(
			filepath.Join(tmp, oldname),
			filepath.Join(tmp, newname)))
	}

	store := &fakeDocStore{}

	chunkifier := new(mocks.MockChunkifier)
	chunkifier.EXPECT().Chunkify(mock.Anything).Return([]string{"content"})

	reg := DocRegistry{
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		root:             tmp,
		storer:           store,
		chunkifier:       chunkifier,
		mergeEventsDelay: 50 * time.Millisecond,
	}
	reg.RegisterReader(&mockTextReader{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, reg.Watch(ctx))
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		createFile("f1.txt", "f1")
		time.Sleep(100 * time.Millisecond)

		createFile("f2.txt", "f2")
		time.Sleep(100 * time.Millisecond)

		createFile("f1.txt", "new f1")
		time.Sleep(100 * time.Millisecond)

		renameFile("f1.txt", "f3.txt")
		time.Sleep(100 * time.Millisecond)

		removeFile("f2.txt")
		time.Sleep(100 * time.Millisecond)

		done <- struct{}{}
	}()

	<-done

	assert.ElementsMatch(t, []string{"f1.txt", "f2.txt", "f1.txt", "f3.txt"}, store.getIngestCalls())
	assert.ElementsMatch(t, []string{"f1.txt", "f1.txt", "f2.txt"}, store.getForgetCalls())
	chunkifier.AssertExpectations(t)
}

func Test_Watch_MergeEvents(t *testing.T) {
	tmp, err := os.MkdirTemp(os.TempDir(), "test_")
	require.NoError(t, err)

	createFile := func(name string, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644))
	}
	renameFile := func(oldname, newname string) {
		require.NoError(t, os.Rename(
			filepath.Join(tmp, oldname),
			filepath.Join(tmp, newname)))
	}

	store := &fakeDocStore{}

	chunkifier := new(mocks.MockChunkifier)
	chunkifier.EXPECT().Chunkify(mock.Anything).Return([]string{"content"})

	reg := DocRegistry{
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		root:             tmp,
		storer:           store,
		chunkifier:       chunkifier,
		mergeEventsDelay: 250 * time.Millisecond,
	}
	reg.RegisterReader(&mockTextReader{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, reg.Watch(ctx))
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		createFile("f1.txt", "f1")
		time.Sleep(50 * time.Millisecond)

		createFile("f1.txt", "new f1")
		time.Sleep(50 * time.Millisecond)

		renameFile("f1.txt", "f3.txt")
		time.Sleep(50 * time.Millisecond)

		createFile("f2.txt", "f2")
		time.Sleep(300 * time.Millisecond)

		createFile("f2.txt", "new f2")
		time.Sleep(300 * time.Millisecond)

		done <- struct{}{}
	}()

	<-done

	assert.ElementsMatch(t, []string{"f3.txt", "f2.txt", "f2.txt"}, store.getIngestCalls())
	chunkifier.AssertExpectations(t)
}

func Test_ingestNewDocuments(t *testing.T) {
	store := new(mocks.MockDocStore)

	reader := new(mocks.MockFileReader)
	reader.On("CanRead", mock.Anything).Return(true)
	reader.On("ReadText", "f1.txt").Return("f1 content", nil)

	chunkifier := new(mocks.MockChunkifier)
	chunkifier.On("Chunkify", mock.Anything).Return([]string{"f1 content"})

	reg := DocRegistry{
		log:        slog.Default(),
		storer:     store,
		chunkifier: chunkifier,
	}
	reg.RegisterReader(reader)

	disk := diskDocs{
		"f1.txt": DiskDoc{File: "f1.txt", Crc: 12345},
		"f2.txt": DiskDoc{File: "f2.txt", Crc: 23456},
	}
	db := dbDocs{
		"f2.txt": docstore.IngestedDoc{File: "f2.txt", Crc: 23456},
		"f3.txt": docstore.IngestedDoc{File: "f3.txt", Crc: 34567},
	}

	expectedDoc := docstore.Doc{
		File:   "f1.txt",
		Crc:    12345,
		Chunks: []string{"f1 content"},
	}
	store.On("Ingest", mock.Anything, expectedDoc).Return(nil)

	require.NoError(t, reg.ingestNewDocuments(context.Background(), disk, db))

	store.AssertExpectations(t)
	reader.AssertExpectations(t)
	chunkifier.AssertExpectations(t)
}

func Test_forgetRemovedDocuments(t *testing.T) {
	store := new(mocks.MockDocStore)
	reg := DocRegistry{
		log:    slog.Default(),
		storer: store,
	}

	disk := diskDocs{
		"f1.txt": DiskDoc{File: "f1.txt", Crc: 12345},
		"f2.txt": DiskDoc{File: "f2.txt", Crc: 23456},
	}
	db := dbDocs{
		"f2.txt": docstore.IngestedDoc{File: "f2.txt", Crc: 23456},
		"f3.txt": docstore.IngestedDoc{File: "f3.txt", Crc: 34567},
	}

	expectedDocument := docstore.IngestedDoc{
		File: "f3.txt",
		Crc:  34567,
	}
	store.EXPECT().Forget(mock.Anything, expectedDocument).Return(nil)

	require.NoError(t, reg.forgetRemovedDocuments(context.Background(), disk, db))

	store.AssertExpectations(t)
}

func Test_collectDocuments(t *testing.T) {
	tmp, err := os.MkdirTemp(os.TempDir(), "test_")
	require.NoError(t, err)

	createFile := func(name string, content string) {
		path := filepath.Join(tmp, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	createFile("f1.txt", "f1 content")
	createFile("f2.txt", "f2 content")
	createFile("f3.pdf", "f3 content")
	createFile("unsupported.bin", "f3 content")

	reader := new(mocks.MockFileReader)
	reader.EXPECT().
		CanRead(mock.MatchedBy(func(path string) bool {
			ext := filepath.Ext(path)
			return ext == ".txt" || ext == ".pdf"
		})).
		Return(true)
	reader.EXPECT().CanRead(mock.Anything).Return(false)
	reader.EXPECT().ReadText(mock.Anything).Return("", nil)

	reg := DocRegistry{
		log:  slog.Default(),
		root: tmp,
	}
	reg.RegisterReader(reader)

	docs, err := reg.collectDocs()
	require.NoError(t, err)

	var files []string
	for _, d := range docs {
		files = append(files, filepath.Base(d.File))
	}

	assert.ElementsMatch(t, files, []string{"f1.txt", "f2.txt", "f3.pdf"})
	reader.AssertExpectations(t)
}
