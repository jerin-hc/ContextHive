package server

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/jerin-stack/CtxHive/internal/repository"
)

const (
	maxDistance = 0.9
)

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

	if strings.TrimSpace(req.Summary) == "" {
		log.Printf("[WARN] POST /content received without a summary")
		http.Error(w, "Summary is required: it is the description of the record and the text embedded for semantic search", http.StatusBadRequest)
		return
	}

	name := req.ProjectName
	if name == "" {
		log.Printf("[INFO] ProjectName not provided, defaulting to %q", "default")
		name = "default"
	}
	if err := s.repository.CreateSchema(s.ctx, name); err != nil {
		log.Printf("[ERROR] Failed to create schema for collection %q: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] Schema ready for collection %q", name)

	log.Printf("[INFO] POST /content — embedding summary (len=%d)", len(req.Summary))
	vec, err := s.model.Embed(req.Summary)
	if err != nil {
		log.Printf("[ERROR] Failed to embed summary in POST /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] POST /content — inserting document into collection %q", name)

	doc := repository.Document{
		Summary:  req.Summary,
		Content:  req.Content,
		Kind:     req.Kind,
		Title:    req.Title,
		Tags:     req.Tags,
		Source:   req.Source,
		Metadata: req.Metadata,
	}

	if err := s.repository.Insert(s.ctx, name, []repository.Document{doc}, vec); err != nil {
		log.Printf("[ERROR] Failed to insert document in POST /content: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] POST /content — document stored successfully (collection=%q, summary_len=%d)", name, len(req.Summary))

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
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

	log.Printf("[INFO] QUERY /content — searching collection %q (topK=%d, maxDistance=%.4f)", req.ProjectName, topK, maxDist)
	results, err := s.repository.Search(s.ctx, req.ProjectName, vec[0], topK, maxDist)
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
