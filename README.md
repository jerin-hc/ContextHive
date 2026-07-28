# CtxHive

**Engineering memory that works like your brain — store context once, find it when you need it.**

CtxHive captures the context you accumulate while working on issues, features, and pull requests, then distills it into dense, keyword-rich summaries stored in a vector database. When a new requirement or question comes up, semantic search surfaces the relevant past context so you don't start from scratch.

## How It Works

```
Jira + PR + Message  ──▶  LLM Summarisation  ──▶  Embedding  ──▶  Milvus Vector DB
                                                                          │
New Requirement  ──▶  Embed Query  ──▶  Semantic Search  ◀───────────────┘
                                          │
                                          ▼
                                   Ranked Results
```

1. **Ingest** — POST a mix of Jira issue details, GitHub PR info, and/or free-form developer notes. The LLM (`qwen2.5-coder:7b`) produces a dense, keyword-rich summary that preserves every technical detail (API names, file paths, error codes, code snippets, diffs).
2. **Store** — The summary is embedded with `nomic-embed-text:v1.5` and stored in [Milvus](https://milvus.io/), along with all original metadata (Jira keys, PR titles, diffs, comments).
3. **Search** — When a new requirement or question arises, embed the query and run a vector similarity search. CtxHive returns the most semantically relevant past context, so you can recall decisions, approaches, and pitfalls without digging through channels or tickets.

## Architecture

```
┌─────────────┐     ┌──────────┐     ┌───────────┐
│   Go HTTP   │────▶│  Ollama  │────▶│  Milvus   │
│   Server    │     │  (LLM)   │     │ (Vector   │
│  :8080      │◀────│          │◀────│   DB)     │
└─────────────┘     └──────────┘     └───────────┘
```

| Component | Role |
|-----------|------|
| **Go HTTP Server** (`:8080`) | REST API + embedded Web UI |
| **Ollama** (`:11434`) | Local LLM inference — `qwen2.5-coder:7b` for summarisation, `nomic-embed-text:v1.5` for embeddings |
| **Milvus** (`:19530`) | Vector database for semantic storage and similarity search |

## Prerequisites

- [Go](https://go.dev/) 1.26+
- [Docker](https://www.docker.com/) & Docker Compose
- [Ollama](https://ollama.com/) (or use the Docker Compose Ollama service)

## Quick Start

### 1. Start Infrastructure

```sh
docker compose up -d
```

This brings up Milvus (with etcd + MinIO) and Ollama. Wait for the health checks to pass:

```sh
docker compose ps
```

### 2. Pull Required Models

If using the containerised Ollama, pull the models inside the container:

```sh
docker exec ollama ollama pull qwen2.5-coder:7b
docker exec ollama ollama pull nomic-embed-text:v1.5
```

The app will also pull them on startup, but pre-pulling is faster.

### 3. Run CtxHive

```sh
go run main.go
```

The server starts on `http://localhost:8080`. The Web UI is served at that address.

## API Reference

### `POST /content` — Store Context

Submit Jira, PR, and/or message data. CtxHive runs it through the LLM to create a dense summary, embeds it, and persists everything in Milvus.

```json
{
  "pr_title": "Fix OOM in search indexing pipeline",
  "pr_description": "The indexing worker was loading entire segment into memory...",
  "pr_diff": "@@ -45,7 +45,7 @@ func buildIndex(seg *Segment) error { ... }",
  "pr_comments": "LGTM, but let's add a guard on segment size before merging.",
  "jira_issue_key": "SEARCH-421",
  "jira_summary": "Search OOM on large indexes",
  "jira_description": "Production search nodes OOM when processing indexes > 2GB.",
  "jira_comments": "Root cause: the chunker reads the full segment upfront.",
  "message": "We should also backport this fix to the v2 release branch."
}
```

Any field can be omitted — provide at least one.

**Response:** `{"status": "ok"}`

### `QUERY /content` — Semantic Search

Search your stored context by meaning, not keywords. CtxHive embeds your query text and finds the most similar stored documents.

```json
{
  "text": "memory leak in indexing pipeline",
  "topK": 3,
  "maxDistance": 0.9
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `text` | Yes | — | The query text to search for |
| `topK` | No | `3` | Max number of results to return |
| `maxDistance` | No | `0.9` | Distance threshold (lower = stricter similarity) |

**Response:**

```json
{
  "status": "ok",
  "results": [
    {
      "document": {
        "content": "## Pull Request\n\n**Title:** Fix OOM in search indexing pipeline...",
        "pr_title": "Fix OOM in search indexing pipeline",
        "pr_diff": "@@ -45,7 +45,7 @@ func buildIndex...",
        "jira_issue_key": "SEARCH-421",
        "message": "We should also backport this fix..."
      },
      "score": 0.234
    }
  ]
}
```

### `GET /` — Web UI

A browser-based interface for testing ingest and search. Open `http://localhost:8080` in your browser.

## Project Structure

```
CtxHive/
├── main.go                          # Entry point — wires up Milvus, Ollama, and HTTP server
├── docker-compose.yml               # Milvus (etcd + MinIO) + Ollama
├── configs/
│   └── milvus.yaml                  # Milvus cluster config
├── internal/
│   ├── model/
│   │   └── model.go                 # Model interface (Generate, Embed)
│   ├── repository/
│   │   └── repository.go            # Repository interface + Document/SearchResult types
│   ├── milvus/
│   │   ├── client.go                # Milvus client — schema, insert, search
│   │   └── document.go              # Milvus-specific document type
│   ├── ollama/
│   │   └── ollama.go                # Ollama client — generate, embed, pull
│   └── server/
│       ├── server.go                # HTTP server setup
│       ├── handler.go               # Route handlers + LLM summarisation prompt
│       ├── request_schema.go        # ContentRequest / QueryRequest types
│       └── frontend/
│           └── index.html           # Embedded Web UI
└── go.mod
```

## Design Decisions

- **LLM-driven summarisation.** Raw Jira and PR data often contains noise (boilerplate, long threads). The LLM condenses it into a dense, search-optimised form while preserving every technical keyword, code snippet, and decision detail. This dramatically improves semantic search recall compared to embedding raw input directly.
- **Single-collection simplicity.** All documents live in one Milvus collection with typed metadata columns. For single-project use, this keeps the API simple and avoids the fragmentation of per-source collections.
- **Local-first, no external APIs.** Ollama + Milvus run entirely on your machine. No data leaves your environment, and no API keys are required.

## Default Models

| Purpose | Model | Notes |
|---------|-------|-------|
| Summarisation | `qwen2.5-coder:7b` | Code-aware, good for technical content |
| Embedding | `nomic-embed-text:v1.5` | 768-dimensional embeddings |

You can change these by editing the constants in `main.go`.

## Use Cases

- **On-call handoffs.** Store incident context (Jira ticket + PR fix + Slack thread dump) and search past incidents by symptom description.
- **Feature planning.** Ingest design docs, spike PRs, and decision logs; query them when a related feature comes up.
- **Code review continuity.** Store PR diffs and review discussions; surface prior art when reviewing similar changes later.
- **Knowledge retention.** As team members come and go, CtxHive keeps the *why* behind decisions searchable.

## License

MIT
