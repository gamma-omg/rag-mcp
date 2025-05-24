package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gamma-omg/rag-mcp/docstore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type docRetriever interface {
	Retrieve(ctx context.Context, query string) ([]docstore.SearchResult, error)
}

func NewRagServer(retriever docRetriever) *server.MCPServer {
	tool := mcp.NewTool("RAG tool",
		mcp.WithDescription("This tool allows searching user documents and get results for RAG"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		))

	srv := server.NewMCPServer("RAG", "0.0.1", server.WithToolCapabilities(false))
	srv.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		res, err := retriever.Retrieve(ctx, q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var response string
		for _, r := range res {
			raw, err := json.Marshal(struct {
				Score float32 `json:"score"`
				File  string  `json:"file"`
				Text  string  `json:"text"`
			}{
				Score: r.Score,
				File:  r.File,
				Text:  r.Text,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			response += fmt.Sprintf("%s\n", string(raw))
		}

		return mcp.NewToolResultText(response), nil
	})

	return srv
}
