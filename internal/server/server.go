package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/jerin-stack/CtxHive/internal/model"
	"github.com/jerin-stack/CtxHive/internal/repository"
)

type Server struct {
	ctx          context.Context
	documentName string
	repository   repository.Repository
	model        model.Model
	port         string
}

func New(ctx context.Context, port string, repository repository.Repository, model model.Model) *Server {
	name := "document1122"
	log.Printf("[INFO] Creating schema for collection %q", name)
	if err := repository.CreateSchema(ctx, name); err != nil {
		log.Printf("[ERROR] Failed to create schema for collection %q: %v", name, err)
		log.Panic(err)
	}
	log.Printf("[INFO] Schema ready for collection %q", name)

	return &Server{
		ctx:          ctx,
		documentName: name,
		repository:   repository,
		model:        model,
		port:         port,
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	routes := s.RegisterRoutes(mux)

	// List all registered API endpoints
	fmt.Println("Registered API Endpoints:")
	for _, route := range routes {
		fmt.Printf("- %s\n", route.Pattern)
	}
	log.Printf("[INFO] Starting HTTP server on :%s", s.port)
	if err := http.ListenAndServe(":"+s.port, mux); err != nil {
		return fmt.Errorf("error serving %w", err)
	}
	return nil
}
