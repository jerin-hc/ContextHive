// Package main runs the CtxHive MCP server, exposing the HTTP API's content
// endpoints as tools over stdio (stdin/stdout):
//
//	store_content — forwards to POST /content
//	query_content — forwards to QUERY /content
//
// The server is a thin client: it does not embed or index anything itself.
// Every tool call is forwarded to the CtxHive HTTP API (the ctxhive service
// from docker-compose.yml), whose address is taken from CTXHIVE_API_ADDR and
// defaults to http://localhost:8080.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultAPIAddr is where the ctxhive service from docker-compose.yml is
// published on the host.
const defaultAPIAddr = "http://localhost:8080"

// methodQuery is the non-standard HTTP method the CtxHive API uses for
// semantic search.
const methodQuery = "QUERY"

// StoreContentInput matches the HTTP POST /content request body. Summary is
// the description of the record and the text embedded for semantic search.
type StoreContentInput struct {
	ID int64 `json:"id" jsonschema:"the primary key of the record; use this ID to update an existing record"`
	Summary     string            `json:"summary" jsonschema:"the description of the record; also the text embedded for semantic search"`
	Content     string            `json:"content" jsonschema:"the full markdown text of the record"`
	Kind        string            `json:"kind" jsonschema:"the kind of content e.g. message, git_pr or jira"`
	Title       string            `json:"title" jsonschema:"short human-readable title"`
	ProjectName string            `json:"projectName,omitempty" jsonschema:"the collection to store into; defaults to \"default\""`
	Tags        []string          `json:"tags,omitempty" jsonschema:"free-form tags for filtering"`
	Source      string            `json:"source,omitempty" jsonschema:"where the content came from"`
	Metadata    map[string]string `json:"metadata,omitempty" jsonschema:"extra context such as branch or ticket id"`
}

// StoreContentOutput matches the HTTP POST /content response.
type StoreContentOutput struct {
	Status string `json:"status" jsonschema:"always \"ok\" on success"`
}

// QueryContentInput matches the HTTP QUERY /content request body.
type QueryContentInput struct {
	ProjectName string  `json:"projectName,omitempty" jsonschema:"the collection to search; defaults to \"default\""`
	Text        string  `json:"text" jsonschema:"the search text; embedded and compared against stored summaries"`
	TopK        int     `json:"topK,omitempty" jsonschema:"number of results to return; defaults to 3"`
	MaxDistance float32 `json:"maxDistance,omitempty" jsonschema:"maximum distance score to include; lower means more similar; defaults to 0.9"`
}

// Document mirrors the repository.Document JSON shape returned by the API.
type Document struct {
	ID int64 `json:"id"`
	Summary  string            `json:"summary"`
	Content  string            `json:"content"`
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Tags     []string          `json:"tags"`
	Source   string            `json:"source"`
	Metadata map[string]string `json:"metadata"`
}

// SearchResult mirrors the repository.SearchResult JSON shape returned by the
// API.
type SearchResult struct {
	Document Document `json:"document"` // the full structured document
	Score    float64  `json:"score"`    // the distance score from the query vector (lower = more similar)
}

// QueryContentOutput matches the HTTP QUERY /content response.
type QueryContentOutput struct {
	Status  string         `json:"status" jsonschema:"always \"ok\" on success"`
	Results []SearchResult `json:"results" jsonschema:"matching documents ordered by similarity; lower score means more similar"`
}

// ListProjectOutput matches the HTTP GET /collection response.
type ListProjectOutput struct {
	Status  string         `json:"status" jsonschema:"always \"ok\" on success"`
	Results []string `json:"results" jsonschema:"avaliable project collections"`
}

// apiClient is a minimal client for the CtxHive HTTP API.
type apiClient struct {
	baseURL string
	client  *http.Client
}

// newAPIClient builds a client for the given API address (e.g.
// "http://localhost:8080"). The timeout is generous because the API embeds
// text and runs vector searches on every call.
func newAPIClient(addr string) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(addr, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// do marshals body as JSON, sends it to path on the CtxHive API using the
// given HTTP method, and decodes the JSON response into out (when non-nil).
// Non-2xx responses are surfaced as errors carrying the server's message.
func (c *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	// The API rejects anything but an exact application/json content type.
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request to CtxHive failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("CtxHive returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

// ping logs whether the API is reachable at startup. It deliberately does not
// fail: the server may be brought up before the stack is, and every tool call
// reports its own errors.
func (c *apiClient) ping() {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		log.Printf("[WARN] CtxHive API address is malformed: %v", err)
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[WARN] CtxHive API not reachable at %s: %v", c.baseURL, err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[INFO] CtxHive API reachable at %s (%s)", c.baseURL, resp.Status)
}

// hive forwards tool calls to the CtxHive API.
type hive struct {
	api *apiClient
}

// storeContent implements POST /content via the API.
func (h *hive) storeContent(ctx context.Context, _ *mcp.CallToolRequest, in StoreContentInput) (*mcp.CallToolResult, StoreContentOutput, error) {
	if strings.TrimSpace(in.Summary) == "" {
		return nil, StoreContentOutput{}, errors.New("summary is required: it is the description of the record and the text embedded for semantic search")
	}

	if in.ProjectName == "" {
		in.ProjectName = "default"
	}

	var out StoreContentOutput
	if err := h.api.do(ctx, http.MethodPost, "/content", in, &out); err != nil {
		return nil, StoreContentOutput{}, fmt.Errorf("failed to store content: %w", err)
	}

	return nil, out, nil
}

// queryContent implements QUERY /content via the API.
func (h *hive) queryContent(ctx context.Context, _ *mcp.CallToolRequest, in QueryContentInput) (*mcp.CallToolResult, QueryContentOutput, error) {
	if in.Text == "" {
		return nil, QueryContentOutput{}, errors.New("text is required for search")
	}

	// topK and maxDistance default to 0 here; the API applies its own
	// defaults (3 and 0.9) to zero values.
	var out QueryContentOutput
	if err := h.api.do(ctx, methodQuery, "/content", in, &out); err != nil {
		return nil, QueryContentOutput{}, fmt.Errorf("search failed: %w", err)
	}

	return nil, out, nil
}

// List Project implements GET /collections via the API.
func (h *hive) ListProject(ctx context.Context, _ *mcp.CallToolRequest,  _ any) (*mcp.CallToolResult, ListProjectOutput, error) {
	var out ListProjectOutput 
	if err := h.api.do(ctx, http.MethodGet, "/collections","{}", &out); err != nil {
		return nil, ListProjectOutput{}, fmt.Errorf("failed to search projects: %w", err)
	}
	return nil, out, nil
}

// registerTools adds the CtxHive tools to the MCP server.
func registerTools(server *mcp.Server, h *hive) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "store_content",
		Description: "Insert or update a record in CtxHive (equivalent of POST /content). If an id is provided in the request, the existing record with that id is updated. If no id is provided, a new record is inserted. The summary is embedded for semantic search; the remaining fields are preserved alongside it.",
	}, h.storeContent)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_content",
		Description: "Search stored records in CtxHive by meaning (equivalent of QUERY /content). Returns the most similar documents with their distance scores.",
	}, h.queryContent)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List the available project collections in CtxHive (equivalent of GET /collections). Returns the names of all available project collections.",
	}, h.ListProject)
}

func main() {
	apiAddr := os.Getenv("CTXHIVE_API_ADDR")
	if apiAddr == "" {
		apiAddr = defaultAPIAddr
	}

	log.Printf("[INFO] Starting CtxHive MCP server (api=%s)", apiAddr)

	api := newAPIClient(apiAddr)
	api.ping()

	server := mcp.NewServer(&mcp.Implementation{Name: "ctxhive", Version: "v1.0.0"}, nil)
	registerTools(server, &hive{api: api})

	// Run the server over stdin/stdout, until the client disconnects.
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
