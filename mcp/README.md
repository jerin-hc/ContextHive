# CtxHive MCP Server

An [MCP (Model Context Protocol)](https://modelcontextprotocol.io) server that exposes the CtxHive HTTP API's content endpoints as tools over stdio. It lets LLM clients (Claude Code, Claude Desktop, etc.) store records in and run semantic searches against a CtxHive instance.

## How it works

The server is a **thin client** — it does not embed or index anything itself. Every tool call is forwarded to the CtxHive HTTP API:

```
LLM client ──stdio──> MCP server ──HTTP──> CtxHive API (CTXHIVE_API_ADDR)
                       (this repo)              └─ POST  /content
                                                └─ QUERY /content
```

The API address comes from the `CTXHIVE_API_ADDR` environment variable and defaults to `http://localhost:8080` (where the `ctxhive` service from the root `docker-compose.yml` is published on the host).

## Tools

### `store_content`

Stores a record in CtxHive (equivalent of `POST /content`). The summary is embedded for semantic search; the remaining fields are preserved alongside it.

| Parameter     | Required | Description                                                        |
| ------------- | -------- | ------------------------------------------------------------------ |
| `summary`     | yes      | The description of the record; also the text embedded for semantic search |
| `content`     | yes      | The full markdown text of the record                               |
| `kind`        | yes      | The kind of content, e.g. `message`, `git_pr`, or `jira`           |
| `title`       | yes      | Short human-readable title                                         |
| `projectName` | no       | The collection to store into; defaults to `"default"`              |
| `tags`        | no       | Free-form tags for filtering                                       |
| `source`      | no       | Where the content came from                                        |
| `metadata`    | no       | Extra context such as branch or ticket id                          |

### `query_content`

Searches stored records in CtxHive by meaning (equivalent of `QUERY /content`). Returns the most similar documents with their distance scores.

| Parameter     | Required | Description                                                        |
| ------------- | -------- | ------------------------------------------------------------------ |
| `text`        | yes      | The search text; embedded and compared against stored summaries    |
| `projectName` | no       | The collection to search; defaults to `"default"`                  |
| `topK`        | no       | Number of results to return; defaults to 3                         |
| `maxDistance` | no       | Maximum distance score to include; lower means more similar; defaults to 0.9 |

## Installation

```sh
./install.sh
```

Builds the binary into `./bin/mcp` (created if missing). The script:

- reuses an existing Go toolchain if it satisfies the version in `go.mod` (`go 1.27.0`),
- otherwise downloads and installs Go to `/usr/local/go` (via `sudo` if needed, falling back to `~/.local/go` when `/usr/local` isn't writable),
- builds a stripped static binary (`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`).

Overrides:

```sh
BIN_DIR=/opt/ctxhive ./install.sh   # output directory
GO_VERSION=1.28.0 ./install.sh      # Go version to install if none is present
```

Requires Go 1.27.0+ if you build manually:

```sh
go build -o bin/mcp .
```

## Configuration

| Variable           | Default                  | Description                          |
| ------------------ | ------------------------ | ------------------------------------ |
| `CTXHIVE_API_ADDR` | `http://localhost:8080`  | Address of the CtxHive HTTP API      |

The server logs a warning at startup if the API is unreachable but still starts — it can come up before the stack is, and every tool call reports its own errors.

## Registering with a client

**Claude Code:**

```sh
claude mcp add ctxhive --env CTXHIVE_API_ADDR=http://localhost:8080 -- /path/to/CtxHive/mcp/bin/mcp
```

**Claude Desktop** (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ctxhive": {
      "command": "/path/to/CtxHive/mcp/bin/mcp",
      "env": {
        "CTXHIVE_API_ADDR": "http://localhost:8080"
      }
    }
  }
}
```

**GitHub Copilot** (`.vscode/mcp.json` at the workspace root):

```json
{
  "servers": {
    "ctxhive": {
      "type": "stdio",
      "command": "/path/to/CtxHive/mcp/bin/mcp",
      "env": {
        "CTXHIVE_API_ADDR": "http://localhost:8080"
      }
    }
  }
}
```

Requires VS Code 1.99+ with Copilot Agent Mode; the file is picked up on save. (Or run *MCP: Add Server* from the Command Palette, which writes it for you.)

**IBM BOB** (`.bob/mcp.json`):

Project-level — place in `.bob/mcp.json` at the project root (safe to commit and share with the team):

```json
{
  "mcpServers": {
    "ctxhive": {
      "command": "/path/to/CtxHive/mcp/bin/mcp",
      "env": {
        "CTXHIVE_API_ADDR": "http://localhost:8080"
      }
    }
  }
}
```

Global — `~/.bob/mcp.json` applies to all workspaces. Restart Bob (or run *Bob: Restart MCP Servers*) after editing, and use Advanced or Agent mode for tool access.

## Development

```sh
go build ./...
go vet ./...
```
