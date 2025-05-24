package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	gemini "github.com/amikos-tech/chroma-go/pkg/embeddings/gemini"
	openai "github.com/amikos-tech/chroma-go/pkg/embeddings/openai"
	"github.com/gamma-omg/rag-mcp/docstore"
	"github.com/gamma-omg/rag-mcp/readers"
	"github.com/mark3labs/mcp-go/server"
)

func createEmbeddingFunction(cfg *Config) (embeddings.EmbeddingFunction, error) {
	if cfg.OpenAI != nil {
		ef, err := openai.NewOpenAIEmbeddingFunction(
			cfg.OpenAI.ApiKey,
			openai.WithModel(openai.EmbeddingModel(cfg.OpenAI.Model)))
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI embedding function: %w", err)
		}

		return ef, nil
	}

	if cfg.Gemini != nil {
		ef, err := gemini.NewGeminiEmbeddingFunction(
			gemini.WithAPIKey(cfg.Gemini.ApiKey),
			gemini.WithDefaultModel(embeddings.EmbeddingModel(cfg.Gemini.Model)))
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini embedding function: %w", err)
		}

		return ef, nil
	}

	return nil, errors.New("invalid embeddings provider configuration")
}

func initDocStore(cfg *Config, reset bool) (*docstore.ChromaStore, error) {
	ef, err := createEmbeddingFunction(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to creat emedding function: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := docstore.NewChromaStore(ctx, docstore.ChromaStoreConfig{
		BaseURL:       cfg.ChromaAddr,
		EmbeddingFunc: ef,
		Results:       cfg.Results,
		RequestSize:   cfg.RequestSize,
		Reset:         reset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Chroma doc store: %w", err)
	}

	return store, nil
}

func main() {
	reset := flag.Bool("reset", false, "Reinitialized the database from scratch if set")
	cfgPath := flag.String("config", "cfg/config.yaml", "Configuration file for the MCP server")
	flag.Parse()

	cfg, err := readConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	logFile, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Fatalf("failed to open log file: %s", err)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, nil))

	store, err := initDocStore(cfg, *reset)
	if err != nil {
		log.Fatal(err)
	}

	reg := DocRegistry{
		log:              logger,
		root:             cfg.DocRoot,
		mergeEventsDelay: time.Duration(cfg.MergeEventsMs) * time.Millisecond,
		storer:           store,
		chunkifier: &DefaultChunkfier{
			chunkSize:    cfg.ChunkSize,
			chunkOverlap: cfg.ChunkOverlap,
		},
		readers: []fileReader{&readers.UniversalFileReader{}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = reg.Sync(ctx)
		if err != nil {
			log.Fatal(err)
		}

		err = reg.Watch(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}()

	srv := NewRagServer(store, logger)
	sse := server.NewSSEServer(srv, server.WithBaseURL(fmt.Sprintf("http://%s", cfg.ServerAddr)))
	log.Println(sse.Start(cfg.ServerAddr))
}
