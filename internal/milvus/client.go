package milvus

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus/client/v3/entity"
	"github.com/milvus-io/milvus/client/v3/index"
	"github.com/milvus-io/milvus/client/v3/milvusclient"

	"github.com/jerin-stack/CtxHive/internal/repository"
)

// Field length constants for Milvus VarChar columns.
const (
	maxContentType  = 50
	maxIssueKey     = 255
	maxTitleSummary = 1024
)

// fieldNames lists every user-defined scalar column in the schema (excluding id and embedding).
// Used by both Insert (to build column slices) and Search (to request output fields).
var fieldNames = []string{
	"content",
	"pr_title",
	"pr_description",
	"pr_diff",
	"pr_comments",
	"jira_issue_key",
	"jira_summary",
	"jira_description",
	"jira_comments",
	"message",
}

type milvus struct {
	client    *milvusclient.Client
	cappacity int64
	dim       int64
}

func NewMilvusClient(ctx context.Context, addr string, cappacity int64, dim int64) (*milvus, error) {
	log.Printf("[INFO] Connecting to Milvus at %s (dim=%d, max_text_len=%d)", addr, dim, cappacity)

	client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: addr,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to create Milvus client: %v", err)
		return nil, fmt.Errorf("error creating milvus client %v", err)
	}

	log.Printf("[INFO] Milvus client connected successfully")
	return &milvus{
		client:    client,
		cappacity: cappacity,
		dim:       dim,
	}, nil
}

func (m *milvus) GetMaxCappacity() int64 {
	return m.cappacity
}

func (m *milvus) CreateSchema(ctx context.Context, documentName string) error {
	log.Printf("[INFO] Creating/loading schema for collection %q", documentName)

	// Check if the collection already exists (e.g. from a previous run)
	hasCollection, err := m.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(documentName))
	if err != nil {
		log.Printf("[ERROR] Failed to check if collection %q exists: %v", documentName, err)
		return fmt.Errorf("error checking if collection exists: %w", err)
	}

	if hasCollection {
		log.Printf("[INFO] Collection %q already exists, skipping schema and index creation", documentName)
	} else {
		log.Printf("[INFO] Creating collection %q (dim=%d, max_text_len=%d, shards=2)", documentName, m.dim, m.cappacity)

		schema := entity.NewSchema().
			WithField(
				entity.NewField().
					WithName("id").
					WithDataType(entity.FieldTypeInt64).
					WithIsPrimaryKey(true).
					WithIsAutoID(true),
			).
			// --- Main embeddable text ---
			WithField(
				entity.NewField().
					WithName("content").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			// --- Git PR fields ---
			WithField(
				entity.NewField().
					WithName("pr_title").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxTitleSummary),
			).
			WithField(
				entity.NewField().
					WithName("pr_description").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			WithField(
				entity.NewField().
					WithName("pr_diff").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			WithField(
				entity.NewField().
					WithName("pr_comments").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			// --- Jira fields ---
			WithField(
				entity.NewField().
					WithName("jira_issue_key").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxIssueKey),
			).
			WithField(
				entity.NewField().
					WithName("jira_summary").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxTitleSummary),
			).
			WithField(
				entity.NewField().
					WithName("jira_description").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			WithField(
				entity.NewField().
					WithName("jira_comments").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			// --- General message ---
			WithField(
				entity.NewField().
					WithName("message").
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			// --- Embedding vector ---
			WithField(
				entity.NewField().
					WithName("embedding").
					WithDataType(entity.FieldTypeFloatVector).
					WithDim(m.dim),
			)

		option := milvusclient.
			NewCreateCollectionOption(documentName, schema).
			WithShardNum(2)

		if err := m.client.CreateCollection(ctx, option); err != nil {
			log.Printf("[ERROR] Failed to create collection %q: %v", documentName, err)
			return fmt.Errorf("error creating schema: %w", err)
		}
		log.Printf("[INFO] Collection %q created successfully", documentName)

		// Create an index on the embedding vector field (required before loading)
		log.Printf("[INFO] Creating auto-index (L2) on embedding field for collection %q", documentName)
		idx := index.NewAutoIndex(entity.L2)
		indexOption := milvusclient.NewCreateIndexOption(documentName, "embedding", idx).WithIndexName("embedding_idx")
		if _, err := m.client.CreateIndex(ctx, indexOption); err != nil {
			log.Printf("[ERROR] Failed to create index for collection %q: %v", documentName, err)
			return fmt.Errorf("error creating index: %w", err)
		}
		log.Printf("[INFO] Index created successfully for collection %q", documentName)
	}

	// Load the collection into memory so it can be searched
	log.Printf("[INFO] Loading collection %q into memory", documentName)
	loadOption := milvusclient.NewLoadCollectionOption(documentName)
	if _, err := m.client.LoadCollection(ctx, loadOption); err != nil {
		log.Printf("[ERROR] Failed to load collection %q into memory: %v", documentName, err)
		return fmt.Errorf("error loading collection: %w", err)
	}
	log.Printf("[INFO] Collection %q loaded into memory and ready for use", documentName)
	return nil
}

// docFields extracts the scalar field values from a Document into individual slices
// that can be passed to WithVarcharColumn. The slices are indexed in the same order
// as the fieldNames package variable.
func docFields(docs []repository.Document) map[string][]string {
	n := len(docs)
	return map[string][]string{
		"content":          makeStrSlice(n, func(i int) string { return docs[i].Content }),
		"pr_title":         makeStrSlice(n, func(i int) string { return docs[i].PRTitle }),
		"pr_description":   makeStrSlice(n, func(i int) string { return docs[i].PRDescription }),
		"pr_diff":          makeStrSlice(n, func(i int) string { return docs[i].PRDiff }),
		"pr_comments":      makeStrSlice(n, func(i int) string { return docs[i].PRComments }),
		"jira_issue_key":   makeStrSlice(n, func(i int) string { return docs[i].JiraIssueKey }),
		"jira_summary":     makeStrSlice(n, func(i int) string { return docs[i].JiraSummary }),
		"jira_description": makeStrSlice(n, func(i int) string { return docs[i].JiraDescription }),
		"jira_comments":    makeStrSlice(n, func(i int) string { return docs[i].JiraComments }),
		"message":          makeStrSlice(n, func(i int) string { return docs[i].Message }),
	}
}

func makeStrSlice(n int, fn func(int) string) []string {
	s := make([]string, n)
	for i := range n {
		s[i] = fn(i)
	}
	return s
}

func (m *milvus) Insert(ctx context.Context, collectionName string, docs []repository.Document, vectors [][]float32) error {
	if len(docs) == 0 {
		log.Printf("[WARN] Insert called on collection %q with 0 documents — nothing to insert", collectionName)
		return nil
	}

	log.Printf("[INFO] Inserting %d document(s) into collection %q", len(docs), collectionName)

	fields := docFields(docs)

	opt := milvusclient.NewColumnBasedInsertOption(collectionName).
		WithVarcharColumn("content", fields["content"]).
		WithVarcharColumn("pr_title", fields["pr_title"]).
		WithVarcharColumn("pr_description", fields["pr_description"]).
		WithVarcharColumn("pr_diff", fields["pr_diff"]).
		WithVarcharColumn("pr_comments", fields["pr_comments"]).
		WithVarcharColumn("jira_issue_key", fields["jira_issue_key"]).
		WithVarcharColumn("jira_summary", fields["jira_summary"]).
		WithVarcharColumn("jira_description", fields["jira_description"]).
		WithVarcharColumn("jira_comments", fields["jira_comments"]).
		WithVarcharColumn("message", fields["message"]).
		WithFloatVectorColumn("embedding", int(m.dim), vectors)

	_, err := m.client.Insert(ctx, opt)
	if err != nil {
		log.Printf("[ERROR] Failed to insert %d document(s) into collection %q: %v", len(docs), collectionName, err)
		return fmt.Errorf("error inserting data %w", err)
	}

	log.Printf("[INFO] Successfully inserted %d document(s) into collection %q", len(docs), collectionName)
	return nil
}

func (m *milvus) Search(ctx context.Context, collectionName string, queryVector []float32, topK int, maxDistance float32) ([]repository.SearchResult, error) {
	log.Printf("[INFO] Searching collection %q (topK=%d, maxDistance=%.4f)", collectionName, topK, maxDistance)

	searchResult, err := m.client.Search(ctx, milvusclient.NewSearchOption(collectionName, topK, []entity.Vector{
		entity.FloatVector(queryVector),
	}).WithOutputFields(fieldNames...))
	if err != nil {
		log.Printf("[ERROR] Semantic search failed on collection %q: %v", collectionName, err)
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}

	resultSet := searchResult[0]
	results := make([]repository.SearchResult, 0, resultSet.ResultCount)
	skipped := 0

	for i := 0; i < resultSet.ResultCount; i++ {
		if resultSet.Scores[i] > maxDistance {
			skipped++
			continue
		}

		doc, err := readDocument(resultSet, i)
		if err != nil {
			log.Printf("[ERROR] Failed to read document at row %d in search results for collection %q: %v", i, collectionName, err)
			return nil, fmt.Errorf("failed to read document at row %d: %w", i, err)
		}

		results = append(results, repository.SearchResult{
			Document: doc,
			Score:    float64(resultSet.Scores[i]),
		})
	}

	log.Printf("[INFO] Search on collection %q returned %d result(s) (%d filtered by distance)", collectionName, len(results), skipped)
	return results, nil
}

// readDocument extracts a single Document from the result set row at the given index.
func readDocument(rs milvusclient.ResultSet, i int) (repository.Document, error) {
	getStr := func(colName string) (string, error) {
		col := rs.GetColumn(colName)
		if col == nil {
			return "", nil // column may not exist in older schemas
		}
		return col.GetAsString(i)
	}
	
	content, err := getStr("content")
	if err != nil {
		return repository.Document{}, err
	}
	prTitle, err := getStr("pr_title")
	if err != nil {
		return repository.Document{}, err
	}
	prDesc, err := getStr("pr_description")
	if err != nil {
		return repository.Document{}, err
	}
	prDiff, err := getStr("pr_diff")
	if err != nil {
		return repository.Document{}, err
	}
	prComments, err := getStr("pr_comments")
	if err != nil {
		return repository.Document{}, err
	}
	jiraKey, err := getStr("jira_issue_key")
	if err != nil {
		return repository.Document{}, err
	}
	jiraSummary, err := getStr("jira_summary")
	if err != nil {
		return repository.Document{}, err
	}
	jiraDesc, err := getStr("jira_description")
	if err != nil {
		return repository.Document{}, err
	}
	jiraComments, err := getStr("jira_comments")
	if err != nil {
		return repository.Document{}, err
	}
	message, err := getStr("message")
	if err != nil {
		return repository.Document{}, err
	}

	return repository.Document{
		Content:         content,
		PRTitle:         prTitle,
		PRDescription:   prDesc,
		PRDiff:          prDiff,
		PRComments:      prComments,
		JiraIssueKey:    jiraKey,
		JiraSummary:     jiraSummary,
		JiraDescription: jiraDesc,
		JiraComments:    jiraComments,
		Message:         message,
	}, nil
}
