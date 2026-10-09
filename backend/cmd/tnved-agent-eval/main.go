// Command tnved-agent-eval проверяет точность ИИ-агента подбора кода ТН ВЭД на проверенных
// записях базы решений: агент подбирает код, не видя самой записи, итог сравнивается с
// проверенным кодом. Ничего не сохраняет; расходует токены модели (DeepSeek Flash — около
// $0.005 за запись).
//
//	go run ./cmd/tnved-agent-eval -limit 20
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
)

func main() {
	limit := flag.Int("limit", 20, "сколько последних проверенных записей проверить")
	baseline := flag.Bool("baseline", false, "как раньше: без интернета и без самопроверки — для сравнения")
	effort := flag.String("effort", "", "уровень размышлений модели: low, high, max (пусто — из TNVED_AI_EFFORT)")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if *effort != "" {
		cfg.TNVEDAIEffort = *effort
	}
	database, err := repository.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	model := tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
		APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Vision: cfg.TNVEDAIVision, Effort: cfg.TNVEDAIEffort,
		PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
	})
	settings := tnvedagent.AgentSettings{DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule, WebSearchKey: cfg.TavilyAPIKey, SelfCheck: true}
	if *baseline {
		settings.WebSearchKey, settings.SelfCheck = "", false
	}
	agent := tnvedagent.NewTNVEDAgentService(database, model, decisions.NewCodeDecisionService(database), settings)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	fmt.Printf("Модель %s, записей до %d, интернет: %v, самопроверка: %v, размышления: %q\n", cfg.TNVEDAIModel, *limit, settings.WebSearchKey != "", settings.SelfCheck, cfg.TNVEDAIEffort)
	summary, err := agent.Evaluate(ctx, *limit, func(line string) { fmt.Println(line) })
	if err != nil {
		log.Fatal(ai.AIErrorMessage(err))
	}
	if summary.Total == 0 {
		fmt.Println("Проверенных записей нет: импортируйте старый инвойс или проверьте решения в базе.")
		return
	}
	percent := func(value int) float64 { return float64(value) * 100 / float64(summary.Total) }
	fmt.Printf("\nВсего %d · лучший код верный: %d (%.0f%%) · верный среди вариантов: %d (%.0f%%) · без итога: %d · ошибок: %d\n",
		summary.Total, summary.BestCorrect, percent(summary.BestCorrect), summary.CandidateCorrect, percent(summary.CandidateCorrect), summary.NoResult, summary.Errors)
	fmt.Printf("Стоимость $%.4f (в среднем $%.4f), время %s (в среднем %s)\n", summary.CostUSD, summary.CostUSD/float64(summary.Total),
		summary.Duration.Round(time.Second), (summary.Duration / time.Duration(summary.Total)).Round(time.Second))
}
