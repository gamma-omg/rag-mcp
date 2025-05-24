# RAG-MCP: Retrieval-Augmented Generation Model Context Protocol Server

RAG-MCP is a Retrieval-Augmented Generation server that implements the Model Context Protocol (MCP) to provide document search capabilities for AI assistants. Built in Go, it allows AI models to search through your documents to provide more accurate, contextually relevant answers.

## Features

- **Document Processing**: Automatically indexes documents from a specified directory
- **Real-time Monitoring**: Watches for file changes and updates the index automatically
- **Efficient Chunking**: Splits documents into optimally sized chunks with configurable overlap
- **Vector Database Integration**: Uses Chroma DB for efficient semantic search
- **MCP Implementation**: Exposes search capabilities via the Model Context Protocol
- **Multiple Embedding Models**: Supports both OpenAI and Google Gemini embeddings
- **Universal Document Support**: Reads various document formats including PDF, DOCX, ODT, TXT, and more

## Prerequisites

- Go 1.23 or later
- Docker and Docker Compose (for running ChromaDB)
- OpenAI API key or Google Gemini API key

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/gamma-omg/rag-mcp.git
   cd rag-mcp
   ```

2. Configure your settings:
   ```bash
   cp cfg/template.yaml cfg/config.yaml
   ```
   Edit `cfg/config.yaml` to set your API keys and other preferences.

3. Start the tool:
   ```bash
   docker-compose up -d
   ```

4. MCP server is now available at (the port can be changed in config.yaml)
   ```
   http://localhost:3001/sse
   ```

## Cursor

To make this tool avaialbe in Cursor, go to Settings -> MCP -> Add new global MCP server and use this configuration:
```json
"mcpServers": {
    "rag-mcp": {
        "url": "http://localhost:3001/sse"
    }
}
```
