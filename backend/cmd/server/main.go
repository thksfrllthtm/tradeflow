package main

import (
	"context"
	"log"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	httpapi "github.com/east-trans-logistic/import-calculator/backend/internal/http"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	database, err := repository.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	server := httpapi.NewServer(database, cfg)
	if err := server.RecoverInterrupted(ctx); err != nil {
		log.Printf("recover interrupted agent sessions: %v", err)
	}
	log.Printf("backend listening on :%s", cfg.Port)
	if err := server.Router().Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
