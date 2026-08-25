package ollama

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"
)

type Ollama struct {
	ctx           context.Context
	generateModel string
	embedModel    string
	client        *api.Client
}

func NewOllamaClient(ctx context.Context, addr string, embedModel string) (*Ollama, error) {
	if embedModel == "" {
		log.Printf("[ERROR] Invalid Ollama model names: embed=%q", embedModel)
		return nil, errors.New("error, invalid model name")
	}
	u, err := url.Parse(addr)
	if err != nil {
		log.Printf("[ERROR] Failed to parse Ollama address %q: %v", addr, err)
		return nil, fmt.Errorf("error, invalid ollama address %v", err)
	}
	log.Printf("[INFO] Ollama client created (addr=%s, embed_model=%q)", addr, embedModel)
	o := &Ollama{
		ctx:        ctx,
		embedModel: embedModel,
		client:     api.NewClient(u, http.DefaultClient),
	}

	log.Printf("[INFO] Pulling embed model %q", embedModel)
	if err := o.pull(embedModel); err != nil {
		log.Printf("[ERROR] Failed to pull embed model %q: %v", embedModel, err)
		log.Panic(err)
	}
	return o, nil
}

func (o *Ollama) Embed(content string) ([][]float32, error) {
	log.Printf("[INFO] Generating embedding (model=%q, content_len=%d)", o.embedModel, len(content))

	req := &api.EmbedRequest{
		Model: o.embedModel,
		Input: []string{content},
	}

	resp, err := o.client.Embed(o.ctx, req)
	if err != nil {
		log.Printf("[ERROR] Embedding failed (model=%q): %v", o.embedModel, err)
		return nil, fmt.Errorf("embedding error: %v", err)
	}

	log.Printf("[INFO] Embedding generated successfully (model=%q, num_vectors=%d)", o.embedModel, len(resp.Embeddings))
	return resp.Embeddings, nil
}

func (o *Ollama) pull(model string) error {
	log.Printf("[INFO] Pulling model %q", model)
	req := &api.PullRequest{
		Model: model,
	}
	pullRespFuc := func(resp api.ProgressResponse) error {
		return nil
	}
	err := o.client.Pull(o.ctx, req, pullRespFuc)
	if err != nil {
		log.Printf("[ERROR] Failed to pull model %q: %v", model, err)
		return fmt.Errorf("error pulling model: %v", err)
	}
	log.Printf("[INFO] Model %q pulled successfully", model)
	return nil
}
