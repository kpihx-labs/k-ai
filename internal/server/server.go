package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/kpihx-labs/k-ai/internal/admin"
	"github.com/kpihx-labs/k-ai/internal/auth"
	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/proxy"
	"github.com/kpihx-labs/k-ai/internal/store"
	"github.com/kpihx-labs/k-ai/internal/web"
)

type Server struct {
	cfg     *config.Config
	store   *store.Store
	gateway *proxy.Gateway
	http    *http.Server
}

func New(cfg *config.Config, st *store.Store) *Server {
	gw := proxy.NewGateway(st)
	s := &Server{cfg: cfg, store: st, gateway: gw}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Internal mock upstream (used by mock provider in config).
	mux.HandleFunc("GET /mock/v1/models", gw.MockListModels)
	mux.HandleFunc("POST /mock/v1/chat/completions", gw.MockChatCompletions)

	// User authentication routes (no API key required)
	jwtMgr, err := auth.NewJWTManager(cfg.Server.JWTSecret, cfg.Server.JWTExpiryDays)
	if err != nil {
		log.Fatalf("jwt: %v", err)
	}
	userHandler := &auth.UserHandler{
		Store:               st,
		JWT:                 jwtMgr,
		RegistrationEnabled: cfg.IsRegistrationEnabled,
	}
	userHandler.Register(mux)

	keyAuth := auth.NewMiddleware(proxy.KeyValidator{Store: st})
	mux.Handle("GET /v1/models", keyAuth.RequireScope("models", http.HandlerFunc(gw.ListModels)))
	mux.Handle("POST /v1/chat/completions", keyAuth.RequireScope("chat", http.HandlerFunc(gw.ChatCompletions)))

	adminHandler := admin.NewHandler(st, jwtMgr)
	adminMux := http.NewServeMux()
	adminHandler.Register(adminMux)
	mux.Handle("/admin/", auth.AdminToken(cfg.Server.AdminToken, adminMux))

	mux.Handle("/", web.Handler())

	s.http = &http.Server{
		Addr:    cfg.ListenAddr(),
		Handler: loggingMiddleware(mux),
	}
	return s
}

func (s *Server) Start() error {
	// Warm up upstream model cache in background for smart routing
	go func() {
		ctx := context.Background()
		registry, err := s.store.BuildRegistry(ctx, s.gateway.ModelCache())
		if err != nil {
			log.Printf("[warmup] failed to build registry: %v", err)
			return
		}
		s.gateway.WarmUpModelCache(ctx, registry)
	}()
	log.Printf("k-ai listening on %s", s.cfg.ListenAddr())
	return s.http.ListenAndServe()
}

func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

func (s *Server) Addr() string {
	return s.cfg.ListenAddr()
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func ListenAndServe(cfg *config.Config, st *store.Store) error {
	s := New(cfg, st)
	if err := s.Start(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}
