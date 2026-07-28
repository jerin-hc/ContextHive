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
// The Content field holds the text that gets embedded; the typed fields carry
// metadata that is preserved alongside the embedding for retrieval.
type Document struct {
	Content     string      `json:"content"` // the full markdown text that was embedded

	// Git PR fields
	PRTitle       string `json:"pr_title,omitempty"`
	PRDescription string `json:"pr_description,omitempty"`
	PRDiff        string `json:"pr_diff,omitempty"`
	PRComments    string `json:"pr_comments,omitempty"`

	// Jira fields
	JiraIssueKey    string `json:"jira_issue_key,omitempty"`
	JiraSummary     string `json:"jira_summary,omitempty"`
	JiraDescription string `json:"jira_description,omitempty"`
	JiraComments    string `json:"jira_comments,omitempty"`

	// General message (used when ContentType is "message")
	Message string `json:"message,omitempty"`
}

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
