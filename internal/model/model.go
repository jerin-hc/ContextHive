package model

// Model defines the interface for AI model operations.
type Model interface {
	// Embed converts content into vector embeddings.
	Embed(content string) ([][]float32, error)
}
