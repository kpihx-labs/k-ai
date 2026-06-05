package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kpihx-labs/k-ai/internal/config"
	"github.com/kpihx-labs/k-ai/internal/server"
	"github.com/kpihx-labs/k-ai/internal/store"
)

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	if err := st.BootstrapFromConfig(cfg); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	srv := server.New(cfg, st)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		os.Exit(0)
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
