package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/jerin-stack/CtxHive/internal/model/ollama"
	"github.com/jerin-stack/CtxHive/internal/repository/milvus"
	"github.com/jerin-stack/CtxHive/internal/server"
)

func main() {
	milvusAddress := getEnv("CTXHIVE_MILVUS_ADDR")
	ollamaAddress := getEnv("CTXHIVE_OLLAMA_ADDR")
	serverPort := getEnv("CTXHIVE_PORT")

	embedModel := getEnv("CTXHIVE_EMBED_MODEL") // nomic-embed-text:v1.5

	cappacity := getEnvInt64("CTXHIVE_CAPACITY") // 65535
	dim := getEnvInt64("CTXHIVE_DIM")            // 768

	log.Printf("[INFO] Starting CtxHive (milvus=%s, ollama=%s, port=%s, embed_model=%s, dim=%d, capacity=%d)",
		milvusAddress, ollamaAddress, serverPort, embedModel, dim, cappacity)

	m, err := milvus.NewMilvusClient(context.Background(), milvusAddress, cappacity, dim)
	if err != nil {
		log.Printf("[ERROR] Failed to create Milvus client: %v", err)
		log.Panic(err)
	}

	o, err := ollama.NewOllamaClient(context.Background(), ollamaAddress, embedModel)
	if err != nil {
		log.Printf("[ERROR] Failed to create Ollama client: %v", err)
		log.Panic(err)
	}

	log.Printf("[INFO] Starting HTTP server on :%s", serverPort)
	s := server.New(context.Background(), serverPort, m, o)
	if err := s.ListenAndServe(); err != nil {
		log.Printf("[ERROR] Server stopped unexpectedly: %v", err)
		log.Panic(err)
	}
}
func getEnv(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		log.Panicf("[FATAL] Required environment variable %s is not set", key)
	}
	return v
}

func getEnvInt64(key string) int64 {
	v := getEnv(key)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Panicf("[FATAL] Environment variable %s=%q is not a valid int64: %v", key, v, err)
	}
	return n
}
