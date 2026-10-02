package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/auth"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	repository, err := repository.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := auth.NewAuthService(repository, cfg.SessionTTL).BootstrapUser(
		ctx,
		os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("employee created: %s (id=%d)", user.Email, user.ID)
}
