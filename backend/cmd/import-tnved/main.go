// Command import-tnved validates and imports an approved classifier dataset.
// It intentionally does not download or scrape web pages. Download the
// official document manually, verify its licence/checksum, convert it to the
// documented JSON/CSV contract, then run preview followed by apply.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func main() {
	inputPath := flag.String("input", "", "JSON или компактный CSV файл")
	mode := flag.String("mode", "preview", "preview или apply")
	flag.Parse()
	if *inputPath == "" {
		fatal("укажите -input")
	}
	f, err := os.Open(*inputPath)
	if err != nil {
		fatal(err.Error())
	}
	defer f.Close()
	var data service.CatalogImportRequest
	if len(*inputPath) >= 4 && (*inputPath)[len(*inputPath)-4:] == ".csv" {
		data, err = service.ParseCatalogCSV(f)
	} else {
		err = json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&data)
	}
	if err != nil {
		fatal("разбор входного файла: " + err.Error())
	}
	checker := service.NewCatalogService(nil)
	report, _ := checker.Validate(data)
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))
	if !report.Valid || *mode == "preview" {
		if !report.Valid {
			os.Exit(2)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err.Error())
	}
	db, err := repository.Open(cfg.DatabaseURL)
	if err != nil {
		fatal(err.Error())
	}
	saved, _, err := service.NewCatalogService(db).Apply(context.Background(), data)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("Импортирована версия %s (%s)\n", saved.VersionKey, saved.DatasetKind)
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
