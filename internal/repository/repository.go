package repository

import "context"

// ContentType classifies the kind of content stored in a document.
type ContentType string

const (
	ContentTypeMessage ContentType = "message"
	ContentTypeGitPR   ContentType = "git_pr"
	ContentTypeJira    ContentType = "jira"
)

// Document represents a structured document stored in the vector database.
// The Summary field holds the description of the record — it is the text that
// gets embedded into the vector used for semantic search. The remaining fields
// are preserved alongside the embedding for retrieval.
type Document struct {
	ID int64 `json:"id"`
	Summary string `json:"summary"` // the record's description; embedded for semantic search
	Content string `json:"content"` // the full markdown text of the record

	Kind     string            `json:"kind"`     // the kind of content, e.g. "discovery"
	Title    string            `json:"title"`    // short human-readable title
	Tags     []string          `json:"tags"`     // free-form tags for filtering
	Source   string            `json:"source"`   // where the content came from
	Metadata map[string]string `json:"metadata"` // extra context, e.g. branch or ticket id
}

// Field names used by the Milvus schema. They mirror the JSON tags on
// Document, so stored fields map directly onto the struct.
const (
	FieldID        = "id"      // primary key, auto-generated
	FieldSummary   = "summary" // the description that gets embedded
	FieldContent   = "content" // the full record text
	FieldKind      = "kind"
	FieldTitle     = "title"
	FieldTags      = "tags"
	FieldSource    = "source"
	FieldMetadata  = "metadata"
	FieldEmbedding = "embedding" // the float vector
)

// SearchResult represents a single result from a vector similarity search.
type SearchResult struct {
	Document Document `json:"document"` // the full structured document
	Score    float64  `json:"score"`    // the distance score from the query vector (lower = more similar)
}

type Repository interface {
	Insert(ctx context.Context, name string, docs []Document, vectors [][]float32) error
	CreateSchema(ctx context.Context, name string) error
	Search(ctx context.Context, collectionName string, queryVector []float32, topK int, maxDistance float32) ([]SearchResult, error)

	GetMaxCappacity() int64
}
