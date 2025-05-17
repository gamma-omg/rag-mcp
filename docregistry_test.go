package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gamma-omg/rag-mcp/docstore"
	mocks "github.com/gamma-omg/rag-mcp/mocks/main"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_injestNewDocuments(t *testing.T) {
	store := new(mocks.MockDocStore)

	reader := new(mocks.MockFileReader)
	reader.On("Ext").Return(".txt")
	reader.On("ReadText", "f1.txt").Return("f1 content", nil)

	chunkifier := new(mocks.MockChunkifier)
	chunkifier.On("Chunkify", mock.Anything).Return([]string{"f1 content"})

	reg := DocRegistry{
		store:      store,
		chunkifier: chunkifier,
		readers:    make(map[string]FileReader),
	}

	err := reg.RegisterReader(reader)
	assert.NoError(t, err)

	disk := diskDocs{
		"f1.txt": DiskDoc{File: "f1.txt", Crc: 12345},
		"f2.txt": DiskDoc{File: "f2.txt", Crc: 23456},
	}
	db := dbDocs{
		"f2.txt": docstore.InjestedDoc{File: "f2.txt", Crc: 23456},
		"f3.txt": docstore.InjestedDoc{File: "f3.txt", Crc: 34567},
	}

	expectedDoc := docstore.Doc{
		File:   "f1.txt",
		Crc:    12345,
		Chunks: []string{"f1 content"},
	}
	store.On("Injest", mock.Anything, expectedDoc).Return(nil)

	err = reg.injestNewDocuments(context.Background(), disk, db)
	assert.NoError(t, err)
	store.AssertExpectations(t)
	reader.AssertExpectations(t)
	chunkifier.AssertExpectations(t)
}

func Test_forgetRemovedDocuments(t *testing.T) {
	store := new(mocks.MockDocStore)
	reg := DocRegistry{
		store:   store,
		readers: make(map[string]FileReader),
	}

	disk := diskDocs{
		"f1.txt": DiskDoc{File: "f1.txt", Crc: 12345},
		"f2.txt": DiskDoc{File: "f2.txt", Crc: 23456},
	}
	db := dbDocs{
		"f2.txt": docstore.InjestedDoc{File: "f2.txt", Crc: 23456},
		"f3.txt": docstore.InjestedDoc{File: "f3.txt", Crc: 34567},
	}

	expectedDocument := docstore.InjestedDoc{
		File: "f3.txt",
		Crc:  34567,
	}
	store.On("Forget", mock.Anything, expectedDocument).Return(nil)

	err := reg.forgetRemovedDocuments(context.Background(), disk, db)
	assert.NoError(t, err)
	store.AssertExpectations(t)
}

func Test_collectDocuments(t *testing.T) {
	tmp, err := os.MkdirTemp(os.TempDir(), "test_")
	assert.NoError(t, err)
	createFile := func(name string, content string) {
		path := filepath.Join(tmp, name)
		err := os.WriteFile(path, []byte(content), 0644)
		assert.NoError(t, err)
	}

	createFile("f1.txt", "f1 content")
	createFile("f2.txt", "f2 content")
	createFile("f3.pdf", "f3 content")
	createFile("unsupported.bin", "f3 content")

	txtReader := new(mocks.MockFileReader)
	txtReader.On("Ext").Return(".txt")
	txtReader.On("ReadText", mock.Anything).Return("content", nil)

	pdfReaer := new(mocks.MockFileReader)
	pdfReaer.On("Ext").Return(".pdf")
	pdfReaer.On("ReadText", mock.Anything).Return("content", nil)

	reg := DocRegistry{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		root:    tmp,
		readers: make(map[string]FileReader),
	}
	err = reg.RegisterReader(txtReader, pdfReaer)
	assert.NoError(t, err)

	docs, err := reg.collectDocs()
	assert.NoError(t, err)

	var files []string
	for _, d := range docs {
		files = append(files, filepath.Base(d.File))
	}

	assert.ElementsMatch(t, files, []string{"f1.txt", "f2.txt", "f3.pdf"})
}
