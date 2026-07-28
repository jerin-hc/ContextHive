package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jerin-stack/CtxHive/internal/repository"
)

const maxDistance = 0.9

//go:embed frontend/index.html
var frontendFS embed.FS

type Route struct {
	Pattern string
}

func requireJSONContentType(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		next(w, r)
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) []Route {
	var routes []Route

	register := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, handler)
		routes = append(routes, Route{Pattern: pattern})
	}

	register("GET /", s.handleServeUI)
	register("POST /content", requireJSONContentType(s.handlePostContent))
	register("QUERY /content", requireJSONContentType(s.handleQueryContent))

	return routes
}

// summarisePrompt instructs the LLM to produce a dense, keyword-rich summary
// that captures all essential details from the combined Jira, PR, and message input.
const summarisePrompt = `You are a technical knowledge-base curator. Given the combined input below —
which may include Jira issue details, GitHub pull-request information, and a general message —
produce a dense, keyword-rich summary. Follow these rules strictly:

- Extract and preserve every KEYWORD, technical term, API name, class name, function name,
  file path, error code, stack trace snippet, configuration key, metric name, and acronym
  present in the input. These keywords are critical for vector search recall.
- Write in compact, information-dense prose. Prefer short, declarative sentences packed
  with terminology. No filler.
- Organise the summary under these Markdown headings (omit a heading if its source data
  is absent): ## Jira, ## Pull Request, ## Message.
- Under each heading, include relevant sub-details: issue key, status, assignee, PR number,
  branch, files changed, reviewers, key discussion points, decisions made, blockers, and
  action items.
- Preserve ALL code snippets, shell commands, diffs, and configuration blocks EXACTLY.
  Wrap them in fenced code blocks with the appropriate language tag.
- The output must stand alone as a self-contained reference document suitable for
  similarity search retrieval. Output ONLY the summary — no preamble, no meta-commentary.

Input:
%s`

func (s *Server) handlePostContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[WARN] handlePostContent called with wrong method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[WARN] Invalid JSON body in POST /content: %v", err)
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Build a combined input from all provided fields for the LLM to summarise.
	input := buildSummaryInput(req)
	if input == "" {
		log.Printf("[WARN] POST /content — all fields empty, nothing to store")
		http.Error(w, "At least one field must be provided", http.StatusBadRequest)
		return
	}

	// Use the LLM to generate a dense, keyword-rich summary of the combined
	// Jira, PR, and message data.
	prompt := fmt.Sprintf(summarisePrompt, input)
	summary, err := s.model.Generate(prompt)
	if err != nil {
		log.Printf("[ERROR] Failed to generate summary in POST /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] POST /content — embedding content (len=%d)", len(summary))
	vec, err := s.model.Embed(summary)
	if err != nil {
		log.Printf("[ERROR] Failed to embed content in POST /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] POST /content — inserting document into collection %q", s.documentName)

	doc := repository.Document{
		Content:     summary,

		PRTitle:       req.PRTitle,
		PRDescription: req.PRDescription,
		PRDiff:        req.PRDiff,
		PRComments:    req.PRComments,

		JiraIssueKey:    req.JiraIssueKey,
		JiraSummary:     req.JiraSummary,
		JiraDescription: req.JiraDescription,
		JiraComments:    req.JiraComments,

		Message: req.Message,
	}

	if err := s.repository.Insert(s.ctx, s.documentName, []repository.Document{doc}, vec); err != nil {
		log.Printf("[ERROR] Failed to insert document in POST /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] POST /content — document stored successfully (collection=%q, content_len=%d)", s.documentName, len(summary))

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// buildSummaryInput concatenates all non-empty fields from the request into a
// single labelled block of text to feed into the LLM summarisation prompt.
func buildSummaryInput(req ContentRequest) string {
	var b strings.Builder

	if req.JiraIssueKey != "" {
		b.WriteString("=== JIRA ISSUE ===\n")
		fmt.Fprintf(&b, "Key: %s\n", req.JiraIssueKey)
	}
	if req.JiraSummary != "" {
		fmt.Fprintf(&b, "Summary: %s\n", req.JiraSummary)
	}
	if req.JiraDescription != "" {
		fmt.Fprintf(&b, "Description:\n%s\n", req.JiraDescription)
	}
	if req.JiraComments != "" {
		fmt.Fprintf(&b, "Comments:\n%s\n", req.JiraComments)
	}

	if req.PRTitle != "" || req.PRDescription != "" || req.PRDiff != "" || req.PRComments != "" {
		b.WriteString("\n=== PULL REQUEST ===\n")
	}
	if req.PRTitle != "" {
		fmt.Fprintf(&b, "Title: %s\n", req.PRTitle)
	}
	if req.PRDescription != "" {
		fmt.Fprintf(&b, "Description:\n%s\n", req.PRDescription)
	}
	if req.PRDiff != "" {
		fmt.Fprintf(&b, "Diff:\n%s\n", req.PRDiff)
	}
	if req.PRComments != "" {
		fmt.Fprintf(&b, "Comments:\n%s\n", req.PRComments)
	}

	if req.Message != "" {
		b.WriteString("\n=== MESSAGE ===\n")
		b.WriteString(req.Message)
		b.WriteString("\n")
	}

	return b.String()
}

func (s *Server) handleQueryContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "QUERY" {
		log.Printf("[WARN] handleQueryContent called with wrong method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[WARN] Invalid JSON body in QUERY /content: %v", err)
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		log.Printf("[WARN] QUERY /content received with empty text")
		http.Error(w, "Text is required for search", http.StatusBadRequest)
		return
	}

	log.Printf("[INFO] QUERY /content — embedding query text (len=%d)", len(req.Text))
	vec, err := s.model.Embed(req.Text)
	if err != nil {
		log.Printf("[ERROR] Failed to embed query in QUERY /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 3
	}
	maxDist := req.MaxDistance
	if maxDist <= 0 {
		maxDist = maxDistance
	}

	log.Printf("[INFO] QUERY /content — searching collection %q (topK=%d, maxDistance=%.4f)", s.documentName, topK, maxDist)
	results, err := s.repository.Search(s.ctx, s.documentName, vec[0], topK, maxDist)
	if err != nil {
		log.Printf("[ERROR] Failed to search in QUERY /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] QUERY /content — search returned %d result(s)", len(results))

	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"results": results,
	})
}

func (s *Server) handleServeUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("[WARN] handleServeUI called with wrong method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	html, err := frontendFS.ReadFile("frontend/index.html")
	if err != nil {
		log.Printf("[ERROR] Failed to read frontend HTML: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

	// formatPrompt instructs the LLM to convert content into well-structured markdown
	// while preserving all original information — nothing is dropped, nothing is summarized.
	const formatPrompt = "Reformat the following content into clean, well-structured Markdown. " +
		"Follow these rules strictly:\n" +
		"- Preserve ALL original information, details, facts, and nuance. Do NOT drop, shorten, or summarize anything.\n" +
		"- Preserve ALL code snippets, shell commands, configuration blocks, git diffs, and technical syntax EXACTLY as-is. " +
		"Wrap them in fenced code blocks with the appropriate language tag (e.g. ```sh, ```python, ```diff, ```json, ```yaml).\n" +
		"- Git diffs are particularly important — wrap every diff (unified, side-by-side, or raw patch format) in a ```diff fenced code block. " +
		"Keep every hunk header, every +/-, every context line intact.\n" +
		"- Use proper Markdown formatting: headings (##, ###), **bold** for emphasis, `backticks` for inline code, " +
		"fenced code blocks (```) for multi-line code, bullet lists for itemization, numbered lists for sequences, " +
		"blockquotes for quoted material, and [link text](URL) for hyperlinks.\n" +
		"- Organize the content logically: add section headings where appropriate, group related ideas under common headings, " +
		"and ensure the document flows naturally from top to bottom.\n" +
		"- Write in clear, complete prose paragraphs for narrative sections. Use lists only where they genuinely " +
		"improve readability.\n" +
		"- The output should be roughly the same length as the input — you are formatting, not summarizing.\n" +
		"- The output must stand alone as a complete, self-contained reference document.\n\nContent:\n%s"
