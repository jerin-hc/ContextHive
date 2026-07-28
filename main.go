package main

import (
	"context"
	"log"

	"github.com/jerin-stack/CtxHive/internal/milvus"
	"github.com/jerin-stack/CtxHive/internal/ollama"
	"github.com/jerin-stack/CtxHive/internal/server"
)

const (
	milvusAddress = "http://localhost:19530"
	ollamaAddress = "http://localhost:11434"
	serverPort    = "8080"

	generateModel = "qwen2.5-coder:7b"
	embedModel    = "nomic-embed-text:v1.5"

	cappacity = 65535 
	dim       = 768
)

func main() {
	log.Printf("[INFO] Starting CtxHive (milvus=%s, ollama=%s, port=%s, generate_model=%s, embed_model=%s, dim=%d, capacity=%d)",
		milvusAddress, ollamaAddress, serverPort, generateModel, embedModel, dim, cappacity)

	m, err := milvus.NewMilvusClient(context.Background(), milvusAddress, cappacity, dim)
	if err != nil {
		log.Printf("[ERROR] Failed to create Milvus client: %v", err)
		log.Panic(err)
	}

	o, err := ollama.NewOllamaClient(context.Background(), ollamaAddress, embedModel, generateModel)
	if err != nil {
		log.Printf("[ERROR] Failed to create Ollama client: %v", err)
		log.Panic(err)
	}

	log.Printf("[INFO] Pulling generate model %q", generateModel)
	if err := o.Pull(generateModel); err != nil {
		log.Printf("[ERROR] Failed to pull generate model %q: %v", generateModel, err)
		log.Panic(err)
	}
	log.Printf("[INFO] Pulling embed model %q", embedModel)
	if err := o.Pull(embedModel); err != nil {
		log.Printf("[ERROR] Failed to pull embed model %q: %v", embedModel, err)
		log.Panic(err)
	}

	log.Printf("[INFO] Starting HTTP server on :%s", serverPort)
	s := server.New(context.Background(), serverPort, m, o)
	if err := s.ListenAndServe(); err != nil {
		log.Printf("[ERROR] Server stopped unexpectedly: %v", err)
		log.Panic(err)
	}
}
