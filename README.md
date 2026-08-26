# CtxHive

**Engineering memory that works like your brain — store context once, find it when you need it.**

```text
┌──────────────┐        ┌──────────────┐        ┌──────────────┐
│ Coding Agent │  MCP   │  MCP Server  │  HTTP  │   CtxHive    │
│ (MCP client) │───────▶│    (mcp/)    │───────▶│    :8080     │
└──────────────┘        └──────────────┘        └──────────────┘
```

CtxHive stores the context you accumulate while working: design docs, decision logs, incident notes, code snippets, meeting summaries. Each record is described by a `summary` that gets embedded and stored in a vector database. When a new requirement or question comes up, semantic search surfaces the relevant past context so you don't start from scratch.

## How It Works

```
Store:  summary + content ──▶  Ollama Embed  ──▶  Milvus Vector DB
                                                  (one collection per project)
                                                                     │
Query:  text ──▶  Ollama Embed  ──▶  Similarity Search  ◀───────────┘
                                         │
                                         ▼
                                  Ranked Results
```

1. **Store** — `POST /content` accepts a generic record: a required `summary` (the text that gets embedded for search) plus optional `content`, `kind`, `title`, `tags`, `source`, and `metadata`. The summary is embedded with `nomic-embed-text:v1.5` and stored in [Milvus](https://milvus.io/), along with the full record. Each `projectName` maps to its own Milvus collection.
2. **Search** — `QUERY /content` embeds your query text and runs a vector similarity search against a project's collection. Results are ranked by L2 distance (lower = more similar) and filtered by a maximum distance threshold.

## Architecture

```
┌─────────────┐     ┌──────────┐     ┌───────────┐
│   Go HTTP   │────▶│  Ollama  │     │  Milvus   │
│   Server    │     │ (embed)  │     │ (Vector   │
│  :8080      │◀────│          │     │   DB)     │
└─────────────┘     └──────────┘     └───────────┘
```

| Component | Role |
|-----------|------|
| **Go HTTP Server** (`:8080`) | REST API + embedded Web UI |
| **Ollama** (`:11434`) | Local embedding inference — `nomic-embed-text:v1.5` |
| **Milvus** (`:19530`) | Vector database for semantic storage and similarity search |

## Prerequisites

- [Docker](https://www.docker.com/) & Docker Compose — for the full stack
- [Go](https://go.dev/) 1.26.5+ — only if running the server outside Docker
- [Ollama](https://ollama.com/) — only if running the server outside Docker (the Compose stack ships its own)

## Quick Start

### Option A: Full stack with Docker Compose (recommended)

```sh
docker compose up -d --build
```

This builds the CtxHive image and starts it alongside Milvus (with etcd + MinIO) and Ollama, waiting for each dependency's health check before the app comes up. On first start the app pulls the embedding model through Ollama — this can take a minute.

```sh
docker compose ps
```

Once `ctxhive` is healthy, open the Web UI at `http://localhost:8080`.

### Option B: Run with Go locally

Start the Milvus side of the stack:

```sh
docker compose up -d etcd minio standalone
```

Run a local Ollama and pull the embedding model:

```sh
ollama serve
ollama pull nomic-embed-text:v1.5
```

Then set the configuration and run the server. All environment variables are required — the app exits with a fatal error if any is missing.

```sh
export CTXHIVE_MILVUS_ADDR=localhost:19530
export CTXHIVE_OLLAMA_ADDR=http://localhost:11434
export CTXHIVE_PORT=8080
export CTXHIVE_EMBED_MODEL=nomic-embed-text:v1.5
export CTXHIVE_CAPACITY=65535
export CTXHIVE_DIM=768

go run main.go
```

The server starts on `http://localhost:8080` with the Web UI at that address.

## MCP Server

For LLM clients (Copilot, IBM BOB, Claude Code, Claude Desktop, …) there is an [MCP (Model Context Protocol)](https://modelcontextprotocol.io) server in [`mcp/`](mcp/). It is a thin stdio server that exposes the API's content endpoints as `store_content` and `query_content` tools, forwarding every call to the CtxHive HTTP API:

```
LLM client ──stdio──▶ MCP server ──HTTP──▶ CtxHive API (:8080)
```

Installation, configuration, and client registration are documented in [`mcp/README.md`](mcp/README.md).

## Configuration

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `CTXHIVE_MILVUS_ADDR` | Yes | Milvus server address | `standalone:19530` |
| `CTXHIVE_OLLAMA_ADDR` | Yes | Ollama base URL | `http://ollama:11434` |
| `CTXHIVE_PORT` | Yes | HTTP server port | `8080` |
| `CTXHIVE_EMBED_MODEL` | Yes | Ollama model used for embeddings | `nomic-embed-text:v1.5` |
| `CTXHIVE_CAPACITY` | Yes | Max length (chars) of the stored `content` field | `65535` |
| `CTXHIVE_DIM` | Yes | Embedding vector dimension | `768` |

## API Reference

### `POST /content` — Store Context

Store a record. The `summary` is the description of the record and the text that gets embedded for semantic search; the rest of the fields are stored alongside it and returned in search results.

```json
{
  "summary": "Indexing worker OOMs on segments > 2GB because the chunker loads the full segment into memory. Fix: stream chunks and guard segment size before merging. Root cause found during SEARCH-421 on-call.",
  "content": "## Incident report\n\nProduction search nodes OOM when processing indexes > 2GB...",
  "kind": "incident",
  "title": "Fix OOM in search indexing pipeline",
  "projectName": "search-service",
  "tags": ["oom", "indexing", "memory"],
  "source": "on-call-notes",
  "metadata": {
    "ticket": "SEARCH-421",
    "branch": "fix/oom-indexer"
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `summary` | Yes | Description of the record — embedded for semantic search |
| `content` | No | The full record text |
| `kind` | No | Kind of content, e.g. `discovery`, `incident` |
| `title` | No | Short human-readable title |
| `projectName` | No | Which collection to store in; defaults to `default` |
| `tags` | No | Free-form tags |
| `source` | No | Where the content came from |
| `metadata` | No | Extra context, e.g. ticket ids or branch names |

**Response:** `{"status": "ok"}`

### `QUERY /content` — Semantic Search

Search a project's stored context by meaning, not keywords. The server embeds your query text and finds the most similar stored documents. Note the custom HTTP method: **`QUERY`**.

```json
{
  "projectName": "search-service",
  "text": "memory leak in indexing pipeline",
  "topK": 3,
  "maxDistance": 0.9
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `projectName` | No | `default` | Collection to search |
| `text` | Yes | — | The query text to embed and search for |
| `topK` | No | `3` | Max number of results to return |
| `maxDistance` | No | `0.9` | L2 distance threshold (lower = stricter similarity) |

**Response:**

```json
{
  "status": "ok",
  "results": [
    {
      "document": {
        "summary": "Indexing worker OOMs on segments > 2GB because...",
        "content": "## Incident report\n\nProduction search nodes OOM...",
        "kind": "incident",
        "title": "Fix OOM in search indexing pipeline",
        "tags": ["oom", "indexing", "memory"],
        "source": "on-call-notes",
        "metadata": { "ticket": "SEARCH-421", "branch": "fix/oom-indexer" }
      },
      "score": 0.234
    }
  ]
}
```

`score` is the L2 distance from the query vector — **lower means more similar**. Results above `maxDistance` are filtered out.

### `GET /` — Web UI

A browser-based interface for storing and searching context. Open `http://localhost:8080` in your browser.

## Project Structure

```
CtxHive/
├── main.go                          # Entry point — env config, wires up Milvus, Ollama, and HTTP server
├── Dockerfile                       # Multi-stage static build
├── docker-compose.yml               # Full stack: CtxHive + Milvus (etcd + MinIO) + Ollama
├── configs/
│   └── milvus.yaml                  # Milvus config
├── internal/
│   ├── model/
│   │   ├── model.go                 # Model interface (Embed)
│   │   └── ollama/
│   │       └── ollama.go            # Ollama client — embed, pull
│   ├── repository/
│   │   ├── repository.go            # Repository interface + Document/SearchResult types
│   │   └── milvus/
│   │       ├── client.go            # Milvus client — schema, insert, search
│   │       └── document.go          # Milvus-specific document type
│   └── server/
│       ├── server.go                # HTTP server setup
│       ├── handler.go               # Route handlers
│       ├── request_schema.go        # ContentRequest / QueryRequest types
│       └── frontend/
│           └── index.html           # Embedded Web UI
├── mcp/                              # MCP server for LLM clients — see mcp/README.md
│   ├── main.go                       # stdio MCP server — forwards tool calls to the HTTP API
│   ├── install.sh                    # Builds a static binary into ./bin/mcp
│   ├── bin/                          # Built binary (from install.sh)
│   └── README.md                     # MCP server documentation
└── go.mod
```

## Design Decisions

- **You write the summary, the system embeds it.** There is no LLM summarisation step — the caller controls exactly what gets embedded. Keep the summary dense and keyword-rich (API names, file paths, error codes, decisions); the full detail lives in `content` and is returned with every match.
- **One collection per project.** Each `projectName` maps to its own Milvus collection (special characters are normalised to underscores), so projects stay isolated without cross-talk in search results.
- **Local-first, no external APIs.** Ollama + Milvus run entirely on your machine. No data leaves your environment, and no API keys are required.

## Use Cases

- **On-call handoffs.** Store incident context (report, fix details, symptom description) and search past incidents by symptom when the next one hits.
- **Feature planning.** Ingest design docs, spike results, and decision logs; query them when a related feature comes up.
- **Knowledge retention.** As team members come and go, CtxHive keeps the *why* behind decisions searchable.
- **Personal engineering memory.** Drop in anything worth remembering — commands, debugging sessions, API quirks — and recall it by meaning later.

## License

MIT
