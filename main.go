package main

import (
	"log"
	"net/http"
	"os"

	"github.com/contextforge/contextforge-broker/broker"
	"github.com/contextforge/contextforge-broker/config"
)

func main() {
	configPath := os.Getenv("BROKER_CONFIG_PATH")
	if configPath == "" {
		configPath = "./broker-config.yml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	b, err := broker.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create broker: %v", err)
	}

	router := b.Router()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("ContextForge MCP Gateway Broker starting on %s", addr)
	log.Printf("Catalog: %d services available", len(cfg.Catalog.Services))

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
