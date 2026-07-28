package model

// Model defines the interface for AI model operations.
type Model interface {
	// Generate produces a response based on the provided messages.
	Generate(message string) (string, error)
	// Embed converts content into vector embeddings.
	Embed(content string) ([][]float32, error)
}
