// Command tnved-agent-eval проверяет точность ИИ-агента подбора кода ТН ВЭД. Ничего не сохраняет;
// расходует токены моделей и запросы Tavily.
//
// По базе решений: последние проверенные записи, агент не видит самой записи.
//
//	go run ./cmd/tnved-agent-eval -limit 20          // модели из TNVED_AI_* / TNVED_AI_VISION_*
//	go run ./cmd/tnved-agent-eval -limit 20 -single  // всё на модели для фото (Flash)
//
// По файлу поставщика: фото и наименования — из Excel, правильные коды — из TSV «строка<TAB>код<TAB>наименование[<TAB>спорный]»;
// схема моделей — -scheme (A–E, см. schemes), подробный итог — в -out (JSON).
//
//	go run ./cmd/tnved-agent-eval -file goods.xlsx -answers answers.tsv -rows 3,4,5 -scheme E -out e.json
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/goods"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
	"github.com/shopspring/decimal"
	"golang.org/x/image/draw"
)

// Модели прогона по файлу. Цены — USD за 1 млн токенов (ввод, ввод из кэша, вывод): DeepSeek — часы пик,
// Claude — прайс Anthropic (2026-10).
func models(cfg config.Config) map[string]tnvedagent.AgentModelConfig {
	price := decimal.RequireFromString
	claudeKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	return map[string]tnvedagent.AgentModelConfig{
		"flash": {APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: "deepseek-flash", Vision: true,
			PriceInput: price("0.30"), PriceCached: price("0.006"), PriceOutput: price("1.20")},
		"pro": {APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: "deepseek-v4-pro", Effort: "high",
			PriceInput: price("1.32"), PriceCached: price("0.044"), PriceOutput: price("3.96")},
		"claude-opus": {Provider: "anthropic", APIKey: claudeKey, Model: "claude-opus-5-5", Vision: true, Effort: "medium",
			PriceInput: price("4"), PriceCached: price("0.20"), PriceOutput: price("20")},
		"claude-sonnet": {Provider: "anthropic", APIKey: claudeKey, Model: "claude-sonnet-5-5", Effort: "high",
			PriceInput: price("2"), PriceCached: price("0.20"), PriceOutput: price("10")},
	}
}

// schemes — схема: модель для фото и модель подбора ("" для фото — одна модель видит фото сама).
var schemes = map[string][2]string{
	"A": {"", "flash"},
	"B": {"flash", "pro"},
	"C": {"claude-opus", "flash"},
	"D": {"claude-opus", "pro"},
	"E": {"claude-opus", "claude-sonnet"},
}

func main() {
	limit := flag.Int("limit", 20, "сколько последних проверенных записей проверить")
	baseline := flag.Bool("baseline", false, "как раньше: без интернета и без самопроверки — для сравнения")
	effort := flag.String("effort", "", "уровень размышлений модели: low, high, max (пусто — из TNVED_AI_EFFORT)")
	single := flag.Bool("single", false, "прежняя схема: одна модель для фото (TNVED_AI_VISION_MODEL) на всё, без размышлений")
	file := flag.String("file", "", "Excel поставщика: прогон по его строкам вместо базы решений")
	answers := flag.String("answers", "", "TSV с правильными кодами: строка, код, наименование, «спорный»")
	rows := flag.String("rows", "", "строки Excel через запятую (пусто — все из -answers)")
	scheme := flag.String("scheme", "", "схема моделей для прогона по файлу: A–E")
	out := flag.String("out", "", "куда записать подробный итог (JSON)")
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
	settings := tnvedagent.AgentSettings{DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule, WebSearchKey: cfg.TavilyAPIKey, SelfCheck: true}
	if *baseline {
		settings.WebSearchKey, settings.SelfCheck = "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	if *file != "" || *scheme != "" {
		model, vision := schemeModels(cfg, *scheme)
		agent := tnvedagent.NewTNVEDAgentService(database, model, vision, decisions.NewCodeDecisionService(database), settings)
		fmt.Printf("Схема %s: фото — %s, подбор — %s, интернет: %v\n", strings.ToUpper(*scheme), visionName(vision), model.Name(), settings.WebSearchKey != "")
		var summary tnvedagent.AgentEvalSummary
		var records []tnvedagent.AgentEvalRecord
		if *file != "" {
			items, err := loadItems(ctx, *file, *answers, *rows)
			if err != nil {
				log.Fatal(err)
			}
			summary, records, err = agent.EvaluateItems(ctx, items, func(line string) { fmt.Println(line) })
			if err != nil {
				log.Fatal(ai.AIErrorMessage(err))
			}
		} else {
			summary, records, err = agent.EvaluateDecisions(ctx, *limit, func(line string) { fmt.Println(line) })
			if err != nil {
				log.Fatal(ai.AIErrorMessage(err))
			}
		}
		printSummary(summary)
		if *out != "" {
			data, _ := json.MarshalIndent(map[string]any{"scheme": strings.ToUpper(*scheme), "vision": visionName(vision), "model": model.Name(),
				"summary": summary, "records": records}, "", "  ")
			if err := os.WriteFile(*out, data, 0o644); err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	visionConfig := tnvedagent.AgentModelConfig{
		APIKey: cfg.TNVEDAIVisionAPIKey, BaseURL: cfg.TNVEDAIVisionBaseURL, Model: cfg.TNVEDAIVisionModel, Vision: cfg.TNVEDAIVision,
		PriceInput: cfg.TNVEDAIVisionPriceInput, PriceCached: cfg.TNVEDAIVisionPriceCached, PriceOutput: cfg.TNVEDAIVisionPriceOutput,
	}
	model := tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
		APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Vision: cfg.TNVEDAIVision && cfg.TNVEDAIVisionModel == "", Effort: cfg.TNVEDAIEffort,
		PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
	})
	var vision *tnvedagent.AgentModelClient
	switch {
	case *single && cfg.TNVEDAIVisionModel == "":
		log.Fatal("-single: задайте TNVED_AI_VISION_MODEL — модель прежней схемы")
	case *single:
		model = tnvedagent.NewAgentModelClient(visionConfig)
	case cfg.TNVEDAIVisionModel != "":
		vision = tnvedagent.NewAgentModelClient(visionConfig)
	}
	agent := tnvedagent.NewTNVEDAgentService(database, model, vision, decisions.NewCodeDecisionService(database), settings)
	fmt.Printf("Модель %s, фото: %s, записей до %d, интернет: %v, самопроверка: %v\n", model.Name(), visionName(vision), *limit, settings.WebSearchKey != "", settings.SelfCheck)
	summary, err := agent.Evaluate(ctx, *limit, func(line string) { fmt.Println(line) })
	if err != nil {
		log.Fatal(ai.AIErrorMessage(err))
	}
	if summary.Total == 0 {
		fmt.Println("Проверенных записей нет: импортируйте старый инвойс или проверьте решения в базе.")
		return
	}
	printSummary(summary)
}

// schemeModels — модели схемы (A–E): модель подбора и, если фото смотрит отдельная модель, она.
func schemeModels(cfg config.Config, scheme string) (*tnvedagent.AgentModelClient, *tnvedagent.AgentModelClient) {
	pair, ok := schemes[strings.ToUpper(scheme)]
	if !ok {
		log.Fatal("-scheme: A, B, C, D или E")
	}
	configs := models(cfg)
	model := tnvedagent.NewAgentModelClient(configs[pair[1]])
	var vision *tnvedagent.AgentModelClient
	if pair[0] != "" {
		reasoning := configs[pair[1]]
		reasoning.Vision = false
		model = tnvedagent.NewAgentModelClient(reasoning)
		vision = tnvedagent.NewAgentModelClient(configs[pair[0]])
	}
	if !model.Configured() || (vision != nil && !vision.Configured()) {
		log.Fatal("модель не настроена: нужны DEEPSEEK_API_KEY (или TNVED_AI_API_KEY) и для Claude — ANTHROPIC_API_KEY")
	}
	return model, vision
}

// loadItems — строки Excel с фото и наименованиями и правильные коды из TSV.
func loadItems(ctx context.Context, file, answersPath, rowsList string) ([]tnvedagent.AgentEvalItem, error) {
	type answer struct {
		code, name string
		disputed   bool
	}
	answers := map[int]answer{}
	order := []int{}
	source, err := os.Open(answersPath)
	if err != nil {
		return nil, fmt.Errorf("-answers: %w", err)
	}
	defer source.Close()
	scanner := bufio.NewScanner(source)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		row, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || len(fields) < 2 {
			continue
		}
		item := answer{code: strings.TrimSpace(fields[1])}
		if len(fields) > 2 {
			item.name = strings.TrimSpace(fields[2])
		}
		item.disputed = len(fields) > 3 && strings.TrimSpace(fields[3]) != ""
		answers[row] = item
		order = append(order, row)
	}
	if rowsList != "" {
		order = order[:0]
		for _, value := range strings.Split(rowsList, ",") {
			if row, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				order = append(order, row)
			}
		}
	}
	workbook, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer workbook.Close()
	info, err := workbook.Stat()
	if err != nil {
		return nil, err
	}
	parsed, err := goods.NewGoodsService().ParseXlsx(ctx, workbook, info.Size(), info.Name(), "", 0, 0)
	if err != nil {
		return nil, err
	}
	values := map[int]map[string]string{}
	for _, row := range parsed.Rows {
		values[row.SourceRow] = row.Values
	}
	photos := map[int][]string{}
	for _, picture := range parsed.Images {
		if photo := photoDataURL(parsed.ImageFiles[picture.DataIndex]); photo != "" && len(photos[picture.SourceRow]) < 3 {
			photos[picture.SourceRow] = append(photos[picture.SourceRow], photo)
		}
	}
	items := []tnvedagent.AgentEvalItem{}
	for _, row := range order {
		expected, ok := answers[row]
		if !ok {
			return nil, fmt.Errorf("строки %d нет в -answers", row)
		}
		// Колонки файла поставщика: русское и китайское наименование, материал.
		cells := values[row]
		item := tnvedagent.AgentItemInput{Name: cells["俄文品名"], SupplierName: cells["品名"], Quantity: "1", Unit: "шт"}
		if material := cells["材质 (2)"]; material != "" {
			item.OtherNames = []string{"Материал: " + material}
		}
		items = append(items, tnvedagent.AgentEvalItem{Label: "строка " + strconv.Itoa(row) + " · " + expected.name, Item: item, Photos: photos[row], Code: expected.code, Disputed: expected.disputed})
	}
	return items, nil
}

// photoDataURL — фото как в программе: браузер уменьшает до 1024 px по длинной стороне, JPEG 85%.
func photoDataURL(data []byte) string {
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	bounds := picture.Bounds()
	scale := min(1, 1024/float64(max(bounds.Dx(), bounds.Dy())))
	target := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(bounds.Dx())*scale)), max(1, int(float64(bounds.Dy())*scale))))
	draw.Draw(target, target.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(target, target.Bounds(), picture, bounds, draw.Over, nil)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, target, &jpeg.Options{Quality: 85}); err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func printSummary(summary tnvedagent.AgentEvalSummary) {
	if summary.Total == 0 {
		return
	}
	percent := func(value int) float64 { return float64(value) * 100 / float64(summary.Total) }
	fmt.Printf("\nВсего %d · лучший код верный: %d (%.0f%%) · верный среди вариантов: %d (%.0f%%) · без итога: %d · ошибок: %d\n",
		summary.Total, summary.BestCorrect, percent(summary.BestCorrect), summary.CandidateCorrect, percent(summary.CandidateCorrect), summary.NoResult, summary.Errors)
	fmt.Printf("Стоимость $%s (в среднем $%s), время %s (в среднем %s)\n", summary.CostUSD.StringFixed(4), summary.CostUSD.Div(decimal.NewFromInt(int64(summary.Total))).StringFixed(4),
		summary.Duration.Round(time.Second), (summary.Duration / time.Duration(summary.Total)).Round(time.Second))
}

// visionName — имя модели для фото или «та же модель».
func visionName(vision *tnvedagent.AgentModelClient) string {
	if vision == nil {
		return "та же модель"
	}
	return vision.Name()
}
