package milvus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/milvus-io/milvus/client/v3/column"
	"github.com/milvus-io/milvus/client/v3/entity"
	"github.com/milvus-io/milvus/client/v3/index"
	"github.com/milvus-io/milvus/client/v3/milvusclient"

	"github.com/jerin-stack/CtxHive/internal/repository"
)

// Field length constants for Milvus VarChar columns.
const (
	maxKind    = 50
	maxTitle   = 1024
	maxSource  = 1024
	maxSummary = 4096
)

// fieldNames lists every user-defined scalar column in the schema (excluding id and embedding).
// Used by both Insert (to build column slices) and Search (to request output fields).
var fieldNames = []string{
	repository.FieldID,
	repository.FieldSummary,
	repository.FieldContent,
	repository.FieldKind,
	repository.FieldTitle,
	repository.FieldTags,
	repository.FieldSource,
	repository.FieldMetadata,
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

// normalizeCollectionName converts a user-supplied project name into a valid
// Milvus collection name. Milvus only allows letters, numbers, and underscores,
// and the name must not start with a digit. Special characters are replaced
// with underscores (rather than dropped) so distinct names like "my-project"
// and "myproject" cannot collapse into the same collection.
func normalizeCollectionName(name string) string {
	const fallback = "default"

	if name == "" {
		return fallback
	}

	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	out := b.String()
	if out == "" {
		return fallback
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

func (m *milvus) CreateSchema(ctx context.Context, documentName string) error {
	documentName = normalizeCollectionName(documentName)
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
					WithName(repository.FieldID).
					WithDataType(entity.FieldTypeInt64).
					WithIsPrimaryKey(true).
					WithIsAutoID(true),
			).
			// --- Description embedded for semantic search ---
			WithField(
				entity.NewField().
					WithName(repository.FieldSummary).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxSummary),
			).
			// --- Full record text ---
			WithField(
				entity.NewField().
					WithName(repository.FieldContent).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(m.cappacity),
			).
			// --- Content metadata ---
			WithField(
				entity.NewField().
					WithName(repository.FieldKind).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxKind),
			).
			WithField(
				entity.NewField().
					WithName(repository.FieldTitle).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxTitle),
			).
			WithField(
				entity.NewField().
					WithName(repository.FieldSource).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(maxSource),
			).
			WithField(
				entity.NewField().
					WithName(repository.FieldTags).
					WithDataType(entity.FieldTypeJSON),
			).
			WithField(
				entity.NewField().
					WithName(repository.FieldMetadata).
					WithDataType(entity.FieldTypeJSON),
			).
			// --- Embedding vector ---
			WithField(
				entity.NewField().
					WithName(repository.FieldEmbedding).
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
		indexOption := milvusclient.NewCreateIndexOption(documentName, repository.FieldEmbedding, idx).WithIndexName("embedding_idx")
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

// docFields extracts the string field values from a Document into individual slices
// that can be passed to WithVarcharColumn. The slices are indexed in the same order
// as the fieldNames package variable.
func docFields(docs []repository.Document) map[string][]string {
	n := len(docs)
	return map[string][]string{
		repository.FieldSummary: makeStrSlice(n, func(i int) string { return docs[i].Summary }),
		repository.FieldContent: makeStrSlice(n, func(i int) string { return docs[i].Content }),
		repository.FieldKind:    makeStrSlice(n, func(i int) string { return docs[i].Kind }),
		repository.FieldTitle:   makeStrSlice(n, func(i int) string { return docs[i].Title }),
		repository.FieldSource:  makeStrSlice(n, func(i int) string { return docs[i].Source }),
	}
}

// jsonColumn marshals one JSON value per document (used for the tags and metadata
// columns, which are stored as Milvus JSON fields).
func jsonColumn[T any](docs []repository.Document, fn func(repository.Document) T) ([][]byte, error) {
	col := make([][]byte, len(docs))
	for i, d := range docs {
		b, err := json.Marshal(fn(d))
		if err != nil {
			return nil, err
		}
		col[i] = b
	}
	return col, nil
}

func makeStrSlice(n int, fn func(int) string) []string {
	s := make([]string, n)
	for i := range n {
		s[i] = fn(i)
	}
	return s
}

func (m *milvus) Insert(ctx context.Context, collectionName string, docs []repository.Document, vectors [][]float32) error {
	collectionName = normalizeCollectionName(collectionName)

	if len(docs) == 0 {
		log.Printf("[WARN] Insert called on collection %q with 0 documents — nothing to insert", collectionName)
		return nil
	}

	log.Printf("[INFO] Inserting %d document(s) into collection %q", len(docs), collectionName)

	fields := docFields(docs)

	tags, err := jsonColumn(docs, func(d repository.Document) []string {
		if d.Tags == nil {
			return []string{}
		}
		return d.Tags
	})
	if err != nil {
		return fmt.Errorf("error marshalling tags: %w", err)
	}
	metadata, err := jsonColumn(docs, func(d repository.Document) map[string]string {
		if d.Metadata == nil {
			return map[string]string{}
		}
		return d.Metadata
	})
	if err != nil {
		return fmt.Errorf("error marshalling metadata: %w", err)
	}

	opt := milvusclient.NewColumnBasedInsertOption(collectionName).
		WithVarcharColumn(repository.FieldSummary, fields[repository.FieldSummary]).
		WithVarcharColumn(repository.FieldContent, fields[repository.FieldContent]).
		WithVarcharColumn(repository.FieldKind, fields[repository.FieldKind]).
		WithVarcharColumn(repository.FieldTitle, fields[repository.FieldTitle]).
		WithVarcharColumn(repository.FieldSource, fields[repository.FieldSource]).
		WithColumns(
			column.NewColumnJSONBytes(repository.FieldTags, tags),
			column.NewColumnJSONBytes(repository.FieldMetadata, metadata),
		).
		WithFloatVectorColumn(repository.FieldEmbedding, int(m.dim), vectors)

	_, err = m.client.Insert(ctx, opt)
	if err != nil {
		log.Printf("[ERROR] Failed to insert %d document(s) into collection %q: %v", len(docs), collectionName, err)
		return fmt.Errorf("error inserting data %w", err)
	}

	log.Printf("[INFO] Successfully inserted %d document(s) into collection %q", len(docs), collectionName)
	return nil
}

func (m *milvus) Search(ctx context.Context, collectionName string, queryVector []float32, topK int, maxDistance float32) ([]repository.SearchResult, error) {
	collectionName = normalizeCollectionName(collectionName)

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
	// getJSON returns the raw JSON value of a JSON column (e.g. tags, metadata).
	getJSON := func(colName string) ([]byte, error) {
		col := rs.GetColumn(colName)
		if col == nil {
			return nil, nil // column may not exist in older schemas
		}
		v, err := col.Get(i)
		if err != nil {
			return nil, err
		}
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("column %q is not a JSON column", colName)
		}
		return b, nil
	}

	getInt64 := func(colName string) (int64, error){
		col := rs.GetColumn(colName)
		if col == nil {
			return 0, nil // column may not exist in older schemas
		}
		return col.GetAsInt64(i)
	}

	id, err := getInt64(repository.FieldID)
	if err != nil {
		return repository.Document{}, err
	}
	summary, err := getStr(repository.FieldSummary)
	if err != nil {
		return repository.Document{}, err
	}
	content, err := getStr(repository.FieldContent)
	if err != nil {
		return repository.Document{}, err
	}
	kind, err := getStr(repository.FieldKind)
	if err != nil {
		return repository.Document{}, err
	}
	title, err := getStr(repository.FieldTitle)
	if err != nil {
		return repository.Document{}, err
	}
	source, err := getStr(repository.FieldSource)
	if err != nil {
		return repository.Document{}, err
	}
	tagsRaw, err := getJSON(repository.FieldTags)
	if err != nil {
		return repository.Document{}, err
	}
	metadataRaw, err := getJSON(repository.FieldMetadata)
	if err != nil {
		return repository.Document{}, err
	}

	doc := repository.Document{
		ID: id,
		Summary: summary,
		Content: content,
		Kind:    kind,
		Title:   title,
		Source:  source,
	}
	if len(tagsRaw) > 0 {
		if err := json.Unmarshal(tagsRaw, &doc.Tags); err != nil {
			return repository.Document{}, fmt.Errorf("failed to unmarshal tags for row %d: %w", i, err)
		}
	}
	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &doc.Metadata); err != nil {
			return repository.Document{}, fmt.Errorf("failed to unmarshal metadata for row %d: %w", i, err)
		}
	}
	return doc, nil
}
