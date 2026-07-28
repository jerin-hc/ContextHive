package milvus

type Document struct {
	Name           string
	Dim            int64
	MaxLen         int64
	DataName       string
	DataValue      []string
	EmbeddingName  string
	EmbeddingValue [][]float64
}
