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
	ctx        context.Context
	repository repository.Repository
	model      model.Model
	port       string
}

func New(ctx context.Context, port string, repository repository.Repository, model model.Model) *Server {
	return &Server{
		ctx:        ctx,
		repository: repository,
		model:      model,
		port:       port,
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
