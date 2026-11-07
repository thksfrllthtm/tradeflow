// legal-acts-collect — сбор актов из реестра docs/acts/acts_registry.json: по каждому коду adilet (основному и
// extra_codes) скачивает сводную редакцию (API adilet, при сбое — zakon.uchet.kz), сверяет дату и номер с
// названием в реестре, кладёт текст в <out>/<код>/<дата редакции>.html и пишет JSON-отчёт. Базу не трогает.
//
//	go run ./cmd/legal-acts-collect -registry ../docs/acts/acts_registry.json -out ../data/acts -report collection.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/legalacts"
)

type registry struct {
	Acts []struct {
		ID         string   `json:"id"`
		Group      string   `json:"group"`
		Kind       string   `json:"kind"`
		Title      string   `json:"title"`
		AdiletCode string   `json:"adilet_code"`
		ExtraCodes []string `json:"extra_codes"`
	} `json:"acts"`
}

type row struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Role         string `json:"role"`
	Group        string `json:"group"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Source       string `json:"source,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	OfficialName string `json:"official_name,omitempty"`
	Title        string `json:"title,omitempty"`
	ActStatus    string `json:"act_status,omitempty"`
	RevisionDate string `json:"revision_date,omitempty"`
	File         string `json:"file,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`
	Check        string `json:"check,omitempty"`
	APIProblem   string `json:"api_problem,omitempty"`
	Note         string `json:"note,omitempty"`
}

func main() {
	registryPath := flag.String("registry", "../docs/acts/acts_registry.json", "реестр актов")
	out := flag.String("out", envOr("LEGAL_ACTS_DIR", "../data/acts"), "куда класть тексты актов")
	reportPath := flag.String("report", "", "JSON-отчёт (пусто — в stdout)")
	parallel := flag.Int("parallel", 4, "запросов одновременно")
	flag.Parse()

	data, err := os.ReadFile(*registryPath)
	if err != nil {
		log.Fatal(err)
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil {
		log.Fatalf("реестр: %v", err)
	}
	var rows []row
	for _, act := range reg.Acts {
		base := row{ID: act.ID, Group: act.Group, Kind: act.Kind}
		if act.AdiletCode == "" {
			base.Status, base.Note = "not_found", "в реестре нет кода adilet"
			rows = append(rows, base)
			continue
		}
		main := base
		main.Code, main.Role = act.AdiletCode, "main"
		rows = append(rows, main)
		for _, code := range act.ExtraCodes {
			extra := base
			extra.Code, extra.Role = code, "extra"
			rows = append(rows, extra)
		}
	}
	titles := map[string]string{}
	for _, act := range reg.Acts {
		titles[act.ID] = act.Title
	}

	source := legalacts.Source{}
	ctx := context.Background()
	slots := make(chan struct{}, *parallel)
	var wg sync.WaitGroup
	for index := range rows {
		if rows[index].Code == "" {
			continue
		}
		wg.Add(1)
		go func(item *row) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			collect(ctx, source, item, titles[item.ID], *out)
			log.Printf("%-28s %-12s %s %s", item.ID, item.Code, item.Status, item.Note)
		}(&rows[index])
	}
	wg.Wait()

	report, _ := json.MarshalIndent(rows, "", "  ")
	if *reportPath == "" {
		fmt.Println(string(report))
		return
	}
	if err := os.WriteFile(*reportPath, report, 0o644); err != nil {
		log.Fatal(err)
	}
}

// collect — один код: скачать, сверить с реестром, сохранить файл.
func collect(ctx context.Context, source legalacts.Source, item *row, registryTitle, out string) {
	revision, err := fetchWithRetry(ctx, source, item.Code)
	if errors.Is(err, legalacts.ErrNotFound) {
		item.Status, item.Note = "not_found", "кода нет ни в API adilet, ни на zakon.uchet.kz"
		return
	}
	if err != nil {
		item.Status, item.Note = "needs_review", "не скачан: "+err.Error()
		return
	}
	item.Source, item.SourceURL, item.OfficialName, item.Title = revision.Source, revision.SourceURL, revision.OfficialName, revision.Title
	item.ActStatus, item.SHA256, item.Bytes, item.APIProblem = revision.ActStatus, revision.SHA256, len(revision.Content), revision.APIProblem
	notes := []string{}
	date := "undated"
	if revision.RevisionDate.IsZero() {
		notes = append(notes, "дата редакции не указана")
	} else {
		date = revision.RevisionDate.Format("2006-01-02")
		item.RevisionDate = date
	}
	switch revision.ActStatus {
	case "yts":
		notes = append(notes, "в adilet — утратил силу")
	case "err":
		notes = append(notes, "в adilet статус err — проверить текст")
	}
	if revision.APIProblem != "" {
		notes = append(notes, "текст с zakon.uchet.kz: API adilet — "+revision.APIProblem)
	}
	if revision.OfficialName != "" {
		check := legalacts.CheckTitle(registryTitle, revision.OfficialName)
		item.Check = fmt.Sprintf("дата %s, номер %s (adilet: %s)", mark(check.Date), mark(check.Number), check.Official)
		if !check.OK() {
			notes = append(notes, "дата или номер не совпали с реестром")
		}
	} else {
		notes = append(notes, "у zakon.uchet.kz нет official_name — сверить вручную")
	}
	item.File = filepath.ToSlash(filepath.Join(item.Code, date+".html"))
	path := filepath.Join(out, item.Code, date+".html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		notes = append(notes, "файл не записан: "+err.Error())
	} else if err := os.WriteFile(path, revision.Content, 0o644); err != nil {
		notes = append(notes, "файл не записан: "+err.Error())
	}
	item.Note = strings.Join(notes, "; ")
	item.Status = "ok"
	if len(notes) > 0 {
		item.Status = "needs_review"
	}
}

// fetchWithRetry — сайты иногда отвечают 5xx на первый запрос: ещё две попытки с паузой.
func fetchWithRetry(ctx context.Context, source legalacts.Source, code string) (legalacts.Revision, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*5) * time.Second)
		}
		revision, err := source.Fetch(ctx, code)
		if err == nil || errors.Is(err, legalacts.ErrNotFound) {
			return revision, err
		}
		lastErr = err
	}
	return legalacts.Revision{}, lastErr
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
