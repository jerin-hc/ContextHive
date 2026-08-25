package server

type ContentRequest struct {
	Summary     string            `json:"summary"` // required: full description of the record; embedded for semantic search
	Content     string            `json:"content"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	ProjectName string            `json:"projectName,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Source      string            `json:"source,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type QueryRequest struct {
	ProjectName string  `json:"projectName"`
	Text        string  `json:"text"`
	TopK        int     `json:"topK,omitempty"`
	MaxDistance float32 `json:"maxDistance,omitempty"`
}
