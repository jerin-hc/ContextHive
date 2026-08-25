package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jerin-stack/CtxHive/internal/repository"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeModel implements model.Model and records every text it embeds.
type fakeModel struct {
	embedded []string
	vec      []float32
	err      error
}

func (f *fakeModel) Embed(content string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.embedded = append(f.embedded, content)
	return [][]float32{f.vec}, nil
}

// searchCall captures the arguments passed to Search.
type searchCall struct {
	collection  string
	queryVector []float32
	topK        int
	maxDistance float32
}

// fakeRepository implements repository.Repository and records every call.
type fakeRepository struct {
	created       []string
	insertedDocs  map[string][]repository.Document
	insertedVecs  [][]float32
	searchCalls   []searchCall
	searchResults []repository.SearchResult
	createErr     error
	insertErr     error
	searchErr     error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{insertedDocs: make(map[string][]repository.Document)}
}

func (f *fakeRepository) CreateSchema(_ context.Context, name string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, name)
	return nil
}

func (f *fakeRepository) Insert(_ context.Context, name string, docs []repository.Document, vectors [][]float32) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.insertedDocs[name] = append(f.insertedDocs[name], docs...)
	f.insertedVecs = append(f.insertedVecs, vectors...)
	return nil
}

func (f *fakeRepository) Search(_ context.Context, collectionName string, queryVector []float32, topK int, maxDistance float32) ([]repository.SearchResult, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	f.searchCalls = append(f.searchCalls, searchCall{
		collection:  collectionName,
		queryVector: append([]float32(nil), queryVector...),
		topK:        topK,
		maxDistance: maxDistance,
	})
	return f.searchResults, nil
}

func (f *fakeRepository) GetMaxCappacity() int64 { return 0 }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestServer(repo *fakeRepository, mdl *fakeModel) *Server {
	return New(context.Background(), "0", repo, mdl)
}

// doRequest routes a request through the same mux RegisterRoutes builds,
// exercising method matching and the JSON content-type middleware.
func doRequest(t *testing.T, s *Server, method, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func postJSON(t *testing.T, s *Server, v any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return doRequest(t, s, http.MethodPost, "/content", body, "application/json")
}

func queryJSON(t *testing.T, s *Server, v any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return doRequest(t, s, "QUERY", "/content", body, "application/json")
}

// ---------------------------------------------------------------------------
// POST /content — 20 input cases
// ---------------------------------------------------------------------------

func TestPostContent20Inputs(t *testing.T) {
	testVec := []float32{0.1, 0.2, 0.3}

	cases := []struct {
		name           string
		req            ContentRequest
		wantCollection string
	}{
		{
			name: "01_full_message",
			req: ContentRequest{
				Summary:     "The login flow returns a 500 when the user password is expired",
				Content:     "## Context\nUsers hit a 500 on /login when their password is expired...",
				Kind:        "message",
				Title:       "Login 500 on expired password",
				ProjectName: "payments",
				Tags:        []string{"bug", "auth"},
				Source:      "slack",
				Metadata:    map[string]string{"channel": "eng-alerts"},
			},
			wantCollection: "payments",
		},
		{
			name: "02_git_pr",
			req: ContentRequest{
				Summary:     "PR adds pagination to the search results endpoint",
				Content:     "# Pagination for /search\nAdds limit/offset params.",
				Kind:        "git_pr",
				Title:       "Add pagination to search",
				ProjectName: "core",
				Tags:        []string{"api"},
				Source:      "github",
				Metadata:    map[string]string{"pr": "1234"},
			},
			wantCollection: "core",
		},
		{
			name: "03_jira_ticket",
			req: ContentRequest{
				Summary:     "Jira ticket describing a memory leak in the vector index",
				Content:     "Tickets: CTX-221",
				Kind:        "jira",
				Title:       "Memory leak in indexer",
				ProjectName: "infra",
				Source:      "jira",
				Metadata:    map[string]string{"assignee": "ij3rry"},
			},
			wantCollection: "infra",
		},
		{
			name: "04_default_project",
			req: ContentRequest{
				Summary: "Note about the release process when ProjectName is omitted",
				Content: "Release steps...",
				Kind:    "note",
				Title:   "Release process",
			},
			wantCollection: "default",
		},
		{
			name: "05_discovery_with_tags",
			req: ContentRequest{
				Summary:     "Discovery: Milvus supports hybrid search with filters",
				Content:     "Hybrid search notes",
				Kind:        "discovery",
				Title:       "Milvus hybrid search",
				ProjectName: "research",
				Tags:        []string{"milvus", "vector"},
			},
			wantCollection: "research",
		},
		{
			name: "06_minimal_summary_only",
			req: ContentRequest{
				Summary: "Only a summary, no other fields",
			},
			wantCollection: "default",
		},
		{
			name: "07_code_snippet",
			req: ContentRequest{
				Summary:     "Code snippet showing how to batch-insert documents",
				Content:     "```go\nclient.Insert(ctx, docs)\n```",
				Kind:        "code",
				Title:       "Batch insert example",
				ProjectName: "snippets",
				Source:      "wiki",
			},
			wantCollection: "snippets",
		},
		{
			name: "08_unicode_summary",
			req: ContentRequest{
				Summary:     "配置 Kubernetes 集群的网络策略",
				Content:     "网络策略配置说明",
				Kind:        "docs",
				Title:       "K8s 网络策略",
				ProjectName: "infra",
			},
			wantCollection: "infra",
		},
		{
			name: "09_meeting_notes",
			req: ContentRequest{
				Summary:     "Sprint planning meeting notes covering the embedding pipeline",
				Content:     "Attendees: A, B. Decisions: ...",
				Kind:        "meeting",
				Title:       "Sprint 42 planning",
				ProjectName: "team",
				Tags:        []string{"sprint-42"},
				Source:      "notion",
				Metadata:    map[string]string{"date": "2026-08-25"},
			},
			wantCollection: "team",
		},
		{
			name: "10_runbook",
			req: ContentRequest{
				Summary:     "Runbook for restarting the search service safely",
				Content:     "1. Drain traffic\n2. Restart\n3. Verify",
				Kind:        "runbook",
				Title:       "Search service restart",
				ProjectName: "ops",
				Tags:        []string{"runbook", "search"},
				Source:      "confluence",
			},
			wantCollection: "ops",
		},
		{
			name: "11_incident_report",
			req: ContentRequest{
				Summary:     "Postmortem of the 2026-08-20 search outage",
				Content:     "Timeline... Root cause...",
				Kind:        "incident",
				Title:       "Search outage postmortem",
				ProjectName: "ops",
				Metadata:    map[string]string{"severity": "sev1"},
			},
			wantCollection: "ops",
		},
		{
			name: "12_empty_kind_and_title",
			req: ContentRequest{
				Summary:     "Summary with kind and title left empty",
				Content:     "Raw content only",
				ProjectName: "misc",
			},
			wantCollection: "misc",
		},
		{
			name: "13_many_tags",
			req: ContentRequest{
				Summary:     "Onboarding guide for new team members",
				Content:     "Setup steps...",
				Kind:        "guide",
				Title:       "Onboarding",
				ProjectName: "team",
				Tags:        []string{"onboarding", "setup", "new-hire", "hr"},
			},
			wantCollection: "team",
		},
		{
			name: "14_rich_metadata",
			req: ContentRequest{
				Summary:     "Benchmark results for embedding throughput",
				Content:     "P50: 12ms, P99: 40ms",
				Kind:        "benchmark",
				Title:       "Embedding throughput",
				ProjectName: "perf",
				Metadata: map[string]string{
					"p50":    "12ms",
					"p99":    "40ms",
					"commit": "abc123",
				},
			},
			wantCollection: "perf",
		},
		{
			name: "15_long_content",
			req: ContentRequest{
				Summary:     "Full architecture decision record for the vector store",
				Content:     "## ADR-007\n\nWe chose Milvus because...\n\n" + string(bytes.Repeat([]byte("detail "), 500)),
				Kind:        "adr",
				Title:       "ADR-007 Vector store",
				ProjectName: "arch",
				Source:      "github",
			},
			wantCollection: "arch",
		},
		{
			name: "16_whitespace_padded_summary",
			req: ContentRequest{
				Summary:     "   summary with surrounding whitespace   ",
				Content:     "Padded summary still accepted",
				Kind:        "note",
				ProjectName: "misc",
			},
			wantCollection: "misc",
		},
		{
			name: "17_git_pr_merge",
			req: ContentRequest{
				Summary:     "Merge PR that removes deprecated summarizer",
				Content:     "Removes the summarizer module",
				Kind:        "git_pr",
				Title:       "Remove summarizer",
				ProjectName: "core",
				Source:      "github",
				Metadata:    map[string]string{"pr": "999"},
			},
			wantCollection: "core",
		},
		{
			name: "18_error_message",
			req: ContentRequest{
				Summary:     "Error seen during migration: duplicate key on insert",
				Content:     "Stack trace...",
				Kind:        "message",
				Title:       "Migration duplicate key",
				ProjectName: "infra",
				Tags:        []string{"error", "db"},
			},
			wantCollection: "infra",
		},
		{
			name: "19_knowledge_article",
			req: ContentRequest{
				Summary:     "How to rotate the database credentials for staging",
				Content:     "Steps to rotate creds...",
				Kind:        "article",
				Title:       "Rotate staging DB creds",
				ProjectName: "kb",
				Tags:        []string{"db", "credentials"},
				Source:      "wiki",
			},
			wantCollection: "kb",
		},
		{
			name: "20_release_notes",
			req: ContentRequest{
				Summary:     "CtxHive v0.2 release notes covering the QUERY endpoint",
				Content:     "New: QUERY /content endpoint",
				Kind:        "release",
				Title:       "CtxHive v0.2",
				ProjectName: "core",
				Source:      "github",
				Metadata:    map[string]string{"version": "v0.2"},
			},
			wantCollection: "core",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			mdl := &fakeModel{vec: testVec}
			s := newTestServer(repo, mdl)

			rr := postJSON(t, s, tc.req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			// Response envelope
			var resp map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp["status"] != "ok" {
				t.Errorf("status field = %q, want ok", resp["status"])
			}

			// Schema created for the right collection
			if len(repo.created) != 1 || repo.created[0] != tc.wantCollection {
				t.Errorf("CreateSchema called with %v, want [%q]", repo.created, tc.wantCollection)
			}

			// Document stored with fields preserved
			docs := repo.insertedDocs[tc.wantCollection]
			if len(docs) != 1 {
				t.Fatalf("inserted %d docs into %q, want 1", len(docs), tc.wantCollection)
			}
			wantDoc := repository.Document{
				Summary:  tc.req.Summary,
				Content:  tc.req.Content,
				Kind:     tc.req.Kind,
				Title:    tc.req.Title,
				Tags:     tc.req.Tags,
				Source:   tc.req.Source,
				Metadata: tc.req.Metadata,
			}
			if !reflect.DeepEqual(docs[0], wantDoc) {
				t.Errorf("inserted doc = %+v, want %+v", docs[0], wantDoc)
			}

			// Summary was embedded, exactly one vector passed
			if len(mdl.embedded) != 1 || mdl.embedded[0] != tc.req.Summary {
				t.Errorf("embedded texts = %q, want [%q]", mdl.embedded, tc.req.Summary)
			}
			if len(repo.insertedVecs) != 1 || !reflect.DeepEqual(repo.insertedVecs[0], testVec) {
				t.Errorf("inserted vectors = %v, want one vector %v", repo.insertedVecs, testVec)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// QUERY /content — 20 query cases
// ---------------------------------------------------------------------------

func TestQueryContent20Queries(t *testing.T) {
	testVec := []float32{0.7, 0.8, 0.9}
	cannedResults := []repository.SearchResult{
		{Document: repository.Document{Summary: "hit 1", Title: "One"}, Score: 0.11},
		{Document: repository.Document{Summary: "hit 2", Title: "Two"}, Score: 0.42},
	}

	cases := []struct {
		name           string
		req            QueryRequest
		wantCollection string
		wantTopK       int
		wantMaxDist    float32
	}{
		{
			name:           "01_defaults",
			req:            QueryRequest{Text: "how do I restart the search service"},
			wantCollection: "",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "02_project_topk_5",
			req:            QueryRequest{ProjectName: "ops", Text: "search outage postmortem", TopK: 5},
			wantCollection: "ops",
			wantTopK:       5,
			wantMaxDist:    0.9,
		},
		{
			name:           "03_topk_1",
			req:            QueryRequest{ProjectName: "core", Text: "pagination PR", TopK: 1},
			wantCollection: "core",
			wantTopK:       1,
			wantMaxDist:    0.9,
		},
		{
			name:           "04_custom_max_distance",
			req:            QueryRequest{ProjectName: "kb", Text: "rotate database credentials", MaxDistance: 0.5},
			wantCollection: "kb",
			wantTopK:       3,
			wantMaxDist:    0.5,
		},
		{
			name:           "05_topk_and_distance",
			req:            QueryRequest{ProjectName: "perf", Text: "embedding throughput benchmark", TopK: 10, MaxDistance: 0.3},
			wantCollection: "perf",
			wantTopK:       10,
			wantMaxDist:    0.3,
		},
		{
			name:           "06_zero_topk_defaults_to_3",
			req:            QueryRequest{ProjectName: "team", Text: "sprint planning notes", TopK: 0},
			wantCollection: "team",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "07_negative_topk_defaults_to_3",
			req:            QueryRequest{ProjectName: "team", Text: "onboarding guide", TopK: -7},
			wantCollection: "team",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "08_negative_max_distance_defaults",
			req:            QueryRequest{ProjectName: "arch", Text: "vector store decision", MaxDistance: -1},
			wantCollection: "arch",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "09_unicode_query",
			req:            QueryRequest{ProjectName: "infra", Text: "网络策略配置", TopK: 2},
			wantCollection: "infra",
			wantTopK:       2,
			wantMaxDist:    0.9,
		},
		{
			name: "10_long_natural_query",
			req: QueryRequest{
				ProjectName: "core",
				Text:        "I remember there was a change that added pagination to the search results endpoint and it was reviewed last week, can you find it?",
				TopK:        4,
				MaxDistance: 0.8,
			},
			wantCollection: "core",
			wantTopK:       4,
			wantMaxDist:    0.8,
		},
		{
			name:           "11_single_word",
			req:            QueryRequest{Text: "runbook"},
			wantCollection: "",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "12_tight_distance",
			req:            QueryRequest{ProjectName: "ops", Text: "restart steps", MaxDistance: 0.05},
			wantCollection: "ops",
			wantTopK:       3,
			wantMaxDist:    0.05,
		},
		{
			name:           "13_release_notes_lookup",
			req:            QueryRequest{ProjectName: "core", Text: "CtxHive v0.2 release notes", TopK: 6, MaxDistance: 0.7},
			wantCollection: "core",
			wantTopK:       6,
			wantMaxDist:    0.7,
		},
		{
			name:           "14_incident_lookup",
			req:            QueryRequest{ProjectName: "ops", Text: "sev1 incident 2026-08-20", TopK: 8},
			wantCollection: "ops",
			wantTopK:       8,
			wantMaxDist:    0.9,
		},
		{
			name:           "15_empty_project_query",
			req:            QueryRequest{Text: "migration duplicate key error", TopK: 3, MaxDistance: 0.6},
			wantCollection: "",
			wantTopK:       3,
			wantMaxDist:    0.6,
		},
		{
			name:           "16_jira_lookup",
			req:            QueryRequest{ProjectName: "infra", Text: "CTX-221 memory leak", TopK: 2, MaxDistance: 0.4},
			wantCollection: "infra",
			wantTopK:       2,
			wantMaxDist:    0.4,
		},
		{
			name:           "17_code_snippet_lookup",
			req:            QueryRequest{ProjectName: "snippets", Text: "batch insert example", TopK: 3},
			wantCollection: "snippets",
			wantTopK:       3,
			wantMaxDist:    0.9,
		},
		{
			name:           "18_meeting_notes_lookup",
			req:            QueryRequest{ProjectName: "team", Text: "what was decided in sprint planning", TopK: 9, MaxDistance: 0.55},
			wantCollection: "team",
			wantTopK:       9,
			wantMaxDist:    0.55,
		},
		{
			name:           "19_large_topk",
			req:            QueryRequest{ProjectName: "kb", Text: "credentials rotation", TopK: 100},
			wantCollection: "kb",
			wantTopK:       100,
			wantMaxDist:    0.9,
		},
		{
			name:           "20_max_distance_one",
			req:            QueryRequest{ProjectName: "misc", Text: "anything", MaxDistance: 1.0},
			wantCollection: "misc",
			wantTopK:       3,
			wantMaxDist:    1.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.searchResults = cannedResults
			mdl := &fakeModel{vec: testVec}
			s := newTestServer(repo, mdl)

			rr := queryJSON(t, s, tc.req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
			}

			var resp struct {
				Status  string                    `json:"status"`
				Results []repository.SearchResult `json:"results"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Status != "ok" {
				t.Errorf("status field = %q, want ok", resp.Status)
			}
			if !reflect.DeepEqual(resp.Results, cannedResults) {
				t.Errorf("results = %+v, want %+v", resp.Results, cannedResults)
			}

			// Query text was embedded
			if len(mdl.embedded) != 1 || mdl.embedded[0] != tc.req.Text {
				t.Errorf("embedded texts = %q, want [%q]", mdl.embedded, tc.req.Text)
			}

			// Search called with defaults applied
			if len(repo.searchCalls) != 1 {
				t.Fatalf("Search called %d times, want 1", len(repo.searchCalls))
			}
			call := repo.searchCalls[0]
			if call.collection != tc.wantCollection {
				t.Errorf("Search collection = %q, want %q", call.collection, tc.wantCollection)
			}
			if call.topK != tc.wantTopK {
				t.Errorf("Search topK = %d, want %d", call.topK, tc.wantTopK)
			}
			if call.maxDistance != tc.wantMaxDist {
				t.Errorf("Search maxDistance = %v, want %v", call.maxDistance, tc.wantMaxDist)
			}
			if !reflect.DeepEqual(call.queryVector, testVec) {
				t.Errorf("Search queryVector = %v, want %v", call.queryVector, testVec)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation and error paths
// ---------------------------------------------------------------------------

func TestPostContentValidation(t *testing.T) {
	cases := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{"wrong_content_type", http.MethodPost, "text/plain", `{"summary":"x"}`, http.StatusBadRequest},
		{"missing_content_type", http.MethodPost, "", `{"summary":"x"}`, http.StatusBadRequest},
		{"invalid_json", http.MethodPost, "application/json", `{not json`, http.StatusBadRequest},
		{"empty_summary", http.MethodPost, "application/json", `{"summary":""}`, http.StatusBadRequest},
		{"whitespace_summary", http.MethodPost, "application/json", `{"summary":"   "}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

			rr := doRequest(t, s, tc.method, "/content", []byte(tc.body), tc.contentType)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			// No repository calls should have happened for rejected input
			if len(repo.created) != 0 || len(repo.insertedDocs) != 0 {
				t.Errorf("repository was called for rejected input: created=%v docs=%v", repo.created, repo.insertedDocs)
			}
		})
	}
}

// The mux itself matches "POST /content" by method, so the handler's internal
// method guard is only reachable when invoked directly.
func TestPostContentWrongMethodDirect(t *testing.T) {
	repo := newFakeRepository()
	s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

	req := httptest.NewRequest(http.MethodGet, "/content", bytes.NewReader([]byte(`{"summary":"x"}`)))
	rr := httptest.NewRecorder()
	s.handlePostContent(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestQueryContentValidation(t *testing.T) {
	cases := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{"empty_text", "QUERY", "application/json", `{"text":""}`, http.StatusBadRequest},
		{"missing_text", "QUERY", "application/json", `{"projectName":"ops"}`, http.StatusBadRequest},
		{"invalid_json", "QUERY", "application/json", `{not json`, http.StatusBadRequest},
		{"wrong_content_type", "QUERY", "text/plain", `{"text":"x"}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

			rr := doRequest(t, s, tc.method, "/content", []byte(tc.body), tc.contentType)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if len(repo.searchCalls) != 0 {
				t.Errorf("Search called for rejected input: %+v", repo.searchCalls)
			}
		})
	}
}

// The mux itself matches "QUERY /content" by method, so the handler's internal
// method guard is only reachable when invoked directly.
func TestQueryContentWrongMethodDirect(t *testing.T) {
	repo := newFakeRepository()
	s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

	req := httptest.NewRequest(http.MethodPost, "/content", bytes.NewReader([]byte(`{"text":"x"}`)))
	rr := httptest.NewRecorder()
	s.handleQueryContent(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestPostContentDependencyErrors(t *testing.T) {
	req := ContentRequest{Summary: "valid summary", ProjectName: "ops"}

	t.Run("create_schema_error", func(t *testing.T) {
		repo := newFakeRepository()
		repo.createErr = context.Canceled
		s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

		if rr := postJSON(t, s, req); rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
	})

	t.Run("embed_error", func(t *testing.T) {
		repo := newFakeRepository()
		mdl := &fakeModel{vec: []float32{0.1}, err: context.Canceled}
		s := newTestServer(repo, mdl)

		if rr := postJSON(t, s, req); rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
	})

	t.Run("insert_error", func(t *testing.T) {
		repo := newFakeRepository()
		repo.insertErr = context.Canceled
		s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

		if rr := postJSON(t, s, req); rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
	})
}

func TestQueryContentDependencyErrors(t *testing.T) {
	req := QueryRequest{ProjectName: "ops", Text: "search"}

	t.Run("embed_error", func(t *testing.T) {
		repo := newFakeRepository()
		mdl := &fakeModel{vec: []float32{0.1}, err: context.Canceled}
		s := newTestServer(repo, mdl)

		if rr := queryJSON(t, s, req); rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
	})

	t.Run("search_error", func(t *testing.T) {
		repo := newFakeRepository()
		repo.searchErr = context.Canceled
		s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

		if rr := queryJSON(t, s, req); rr.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rr.Code)
		}
	})
}

func TestServeUI(t *testing.T) {
	repo := newFakeRepository()
	s := newTestServer(repo, &fakeModel{vec: []float32{0.1}})

	rr := doRequest(t, s, http.MethodGet, "/", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Error("frontend HTML body is empty")
	}
}
