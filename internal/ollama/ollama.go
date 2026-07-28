package ollama

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/ollama/ollama/api"
)

type Ollama struct {
	ctx           context.Context
	generateModel string
	embedModel    string
	client        *api.Client
}

func NewOllamaClient(ctx context.Context, addr string, embedModel string, generateModel string) (*Ollama, error) {
	if generateModel == "" || embedModel == "" {
		log.Printf("[ERROR] Invalid Ollama model names: generate=%q, embed=%q", generateModel, embedModel)
		return nil, errors.New("error, invalid model name")
	}
	u, err := url.Parse(addr)
	if err != nil {
		log.Printf("[ERROR] Failed to parse Ollama address %q: %v", addr, err)
		return nil, fmt.Errorf("error, invalid ollama address %v", err)
	}
	log.Printf("[INFO] Ollama client created (addr=%s, generate_model=%q, embed_model=%q)", addr, generateModel, embedModel)
	return &Ollama{
		ctx:           ctx,
		embedModel:    embedModel,
		generateModel: generateModel,
		client:        api.NewClient(u, http.DefaultClient),
	}, nil
}

func (o *Ollama) Generate(message string) (string, error) {
	log.Printf("[INFO] Generating response (model=%q, prompt_len=%d)", o.generateModel, len(message))

	req := &api.GenerateRequest{
		Model:  o.generateModel,
		Prompt: message,
	}

	var fullResponse strings.Builder
	respFunc := func(resp api.GenerateResponse) error {
		fullResponse.WriteString(resp.Response)
		return nil
	}

	err := o.client.Generate(o.ctx, req, respFunc)
	if err != nil {
		log.Printf("[ERROR] Generation failed (model=%q): %v", o.generateModel, err)
		return "", fmt.Errorf("generation error: %v", err)
	}
	log.Printf("[INFO] Generation complete (model=%q, response_len=%d)", o.generateModel, fullResponse.Len())
	return fullResponse.String(), nil
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

func (o *Ollama) Pull(model string) error {
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
