// Command tnved-agent-eval проверяет точность ИИ-агента подбора кода ТН ВЭД. Ничего не сохраняет;
// расходует токены моделей и запросы Tavily.
//
// По базе решений: последние проверенные записи, агент не видит самой записи.
//
//	go run ./cmd/tnved-agent-eval -limit 20          // модели из TNVED_AI_* / TNVED_AI_VISION_*
//	go run ./cmd/tnved-agent-eval -limit 20 -single  // всё на модели для фото (Flash)
//
// По файлу поставщика: фото и наименования — из Excel, правильные коды — из TSV «строка<TAB>код<TAB>наименование[<TAB>спорный]»;
// схема моделей — -scheme (A–E и P — как на проде, см. schemes), подробный итог — в -out (JSON).
//
//	go run ./cmd/tnved-agent-eval -file goods.xlsx -answers answers.tsv -rows 3,4,5 -scheme E -out e.json
//
// Несколько схем через запятую идут параллельно на одних и тех же товарах (у провайдеров свои лимиты),
// в конце — сравнение по товарам. Запросы Tavily расходуются каждой схемой отдельно.
//
//	go run ./cmd/tnved-agent-eval -limit 20 -scheme P,E -out compare.json
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
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
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
// Схема P — модели как на проде (TNVED_AI_* / TNVED_AI_VISION_*), задаётся в schemeModels.
var schemes = map[string][2]string{
	"A": {"", "flash"},
	"B": {"flash", "pro"},
	"C": {"claude-opus", "flash"},
	"D": {"claude-opus", "pro"},
	"E": {"claude-opus", "claude-sonnet"},
}

const schemeProd = "P"

// schemeRun — прогон одной схемы: модели, сводка и записи по товарам.
type schemeRun struct {
	Scheme  string                       `json:"scheme"`
	Vision  string                       `json:"vision"`
	Model   string                       `json:"model"`
	Summary tnvedagent.AgentEvalSummary  `json:"summary"`
	Records []tnvedagent.AgentEvalRecord `json:"records"`
}

func main() {
	limit := flag.Int("limit", 20, "сколько последних проверенных записей проверить")
	baseline := flag.Bool("baseline", false, "как раньше: без интернета и без самопроверки — для сравнения")
	effort := flag.String("effort", "", "уровень размышлений модели: low, high, max (пусто — из TNVED_AI_EFFORT)")
	single := flag.Bool("single", false, "прежняя схема: одна модель для фото (TNVED_AI_VISION_MODEL) на всё, без размышлений")
	file := flag.String("file", "", "Excel поставщика: прогон по его строкам вместо базы решений")
	answers := flag.String("answers", "", "TSV с правильными кодами: строка, код, наименование, «спорный»")
	rows := flag.String("rows", "", "строки Excel через запятую (пусто — все из -answers)")
	scheme := flag.String("scheme", "", "схемы моделей через запятую: A–E, P (как на проде); несколько — параллельно, со сравнением")
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
	settings := tnvedagent.AgentSettings{DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule, WebSearchKey: cfg.TavilyAPIKey}
	if *baseline {
		settings.WebSearchKey, settings.SelfCheck = "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	if *file != "" || *scheme != "" {
		runSchemes(ctx, cfg, database, settings, strings.Split(*scheme, ","), *file, *answers, *rows, *limit, *out)
		return
	}
	model, vision := envModels(cfg)
	switch {
	case *single && cfg.TNVEDAIVisionModel == "":
		log.Fatal("-single: задайте TNVED_AI_VISION_MODEL — модель прежней схемы")
	case *single:
		model, vision = tnvedagent.NewAgentModelClient(visionConfig(cfg)), nil
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

// runSchemes — прогон схем на одних и тех же товарах: параллельно (у провайдеров отдельные лимиты), по ходу
// строки с меткой схемы, в конце сводка каждой схемы и сравнение по товарам.
func runSchemes(ctx context.Context, cfg config.Config, database *repository.Repository, settings tnvedagent.AgentSettings, names []string, file, answers, rows string, limit int, out string) {
	var items []tnvedagent.AgentEvalItem
	if file != "" {
		var err error
		if items, err = loadItems(ctx, file, answers, rows); err != nil {
			log.Fatal(err)
		}
	}
	runs := make([]schemeRun, len(names))
	agents := make([]*tnvedagent.TNVEDAgentService, len(names))
	for index, name := range names {
		name = strings.ToUpper(strings.TrimSpace(name))
		model, vision := schemeModels(cfg, name)
		agents[index] = tnvedagent.NewTNVEDAgentService(database, model, vision, decisions.NewCodeDecisionService(database), settings)
		runs[index] = schemeRun{Scheme: name, Vision: visionName(vision), Model: model.Name()}
		fmt.Printf("Схема %s: фото — %s, подбор — %s, интернет: %v\n", name, runs[index].Vision, runs[index].Model, settings.WebSearchKey != "")
	}
	var wg sync.WaitGroup
	for index := range runs {
		wg.Add(1)
		go func(run *schemeRun, agent *tnvedagent.TNVEDAgentService) {
			defer wg.Done()
			report := func(line string) { fmt.Printf("[%s] %s\n", run.Scheme, line) }
			var err error
			if file != "" {
				run.Summary, run.Records, err = agent.EvaluateItems(ctx, items, report)
			} else {
				run.Summary, run.Records, err = agent.EvaluateDecisions(ctx, limit, report)
			}
			if err != nil {
				log.Printf("[%s] прогон прерван: %s", run.Scheme, ai.AIErrorMessage(err))
			}
		}(&runs[index], agents[index])
	}
	wg.Wait()
	for _, run := range runs {
		fmt.Printf("\nСхема %s (фото — %s, подбор — %s):", run.Scheme, run.Vision, run.Model)
		printSummary(run.Summary)
	}
	if len(runs) > 1 {
		printComparison(os.Stdout, runs)
	}
	if out != "" {
		data, _ := json.MarshalIndent(runs, "", "  ")
		if err := os.WriteFile(out, data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// printComparison — товары по строкам, под каждым — итог каждой схемы.
func printComparison(w io.Writer, runs []schemeRun) {
	fmt.Fprintln(w, "\nСравнение по товарам:")
	for index, record := range runs[0].Records {
		fmt.Fprintf(w, "%3d. %s → %s\n", index+1, service.TruncateRunes(record.Name, 60), tnvedagent.FormatTNVEDCode(record.Code))
		for _, run := range runs {
			if index >= len(run.Records) {
				fmt.Fprintf(w, "     %s: прогон прерван\n", run.Scheme)
				continue
			}
			fmt.Fprintf(w, "     %s: %s\n", run.Scheme, run.Records[index].Outcome())
		}
	}
}

// schemeModels — модели схемы (A–E, P): модель подбора и, если фото смотрит отдельная модель, она.
func schemeModels(cfg config.Config, scheme string) (*tnvedagent.AgentModelClient, *tnvedagent.AgentModelClient) {
	var model, vision *tnvedagent.AgentModelClient
	if scheme == schemeProd {
		model, vision = envModels(cfg)
	} else {
		pair, ok := schemes[scheme]
		if !ok {
			log.Fatal("-scheme: A, B, C, D, E или P")
		}
		configs := models(cfg)
		model = tnvedagent.NewAgentModelClient(configs[pair[1]])
		if pair[0] != "" {
			reasoning := configs[pair[1]]
			reasoning.Vision = false
			model = tnvedagent.NewAgentModelClient(reasoning)
			vision = tnvedagent.NewAgentModelClient(configs[pair[0]])
		}
	}
	if !model.Configured() || (vision != nil && !vision.Configured()) {
		log.Fatalf("схема %s: модель не настроена — нужны DEEPSEEK_API_KEY (или TNVED_AI_API_KEY) и для Claude — ANTHROPIC_API_KEY", scheme)
	}
	return model, vision
}

// envModels — модели как на проде: подбор из TNVED_AI_*, фото из TNVED_AI_VISION_* (nil — одна модель на всё).
func envModels(cfg config.Config) (*tnvedagent.AgentModelClient, *tnvedagent.AgentModelClient) {
	model := tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
		APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Vision: cfg.TNVEDAIVision && cfg.TNVEDAIVisionModel == "", Effort: cfg.TNVEDAIEffort,
		PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
	})
	var vision *tnvedagent.AgentModelClient
	if cfg.TNVEDAIVisionModel != "" {
		vision = tnvedagent.NewAgentModelClient(visionConfig(cfg))
	}
	return model, vision
}

func visionConfig(cfg config.Config) tnvedagent.AgentModelConfig {
	return tnvedagent.AgentModelConfig{
		APIKey: cfg.TNVEDAIVisionAPIKey, BaseURL: cfg.TNVEDAIVisionBaseURL, Model: cfg.TNVEDAIVisionModel, Vision: cfg.TNVEDAIVision,
		PriceInput: cfg.TNVEDAIVisionPriceInput, PriceCached: cfg.TNVEDAIVisionPriceCached, PriceOutput: cfg.TNVEDAIVisionPriceOutput,
	}
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
