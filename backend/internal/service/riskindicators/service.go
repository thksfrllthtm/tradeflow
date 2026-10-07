package riskindicators

// Стоимостные индикаторы риска (СУР) КГД РК — сколько долларов США стоит 1 кг товара по коду
// ТН ВЭД для страны происхождения. Администратор загружает приложения к профилям риска
// («24_прил CN», «25_прил CN», «26_прил CN»); смета в режиме «по СУР» берёт индикатор кода
// для страны сметы (по умолчанию CN).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

// DefaultCountry — страна, для которой загружены файлы «…_прил CN».
const DefaultCountry = "CN"

type Service struct{ repository *repository.Repository }

func NewService(r *repository.Repository) *Service { return &Service{repository: r} }

// Profile — загруженный профиль риска для страницы номенклатуры.
type Profile struct {
	ID             int64    `json:"id"`
	Number         string   `json:"number"`
	FileName       string   `json:"fileName"`
	CodeCount      int      `json:"codeCount"`
	Groups         []string `json:"groups"`
	ImportedAt     string   `json:"importedAt"`
	ImportedByName string   `json:"importedByName"`
}

// ImportResult: с Errors ничего не сохранено. DefaultCountryCodes — сколько кодов с индикатором для CN.
type ImportResult struct {
	Profile             *Profile   `json:"profile,omitempty"`
	Replaced            bool       `json:"replaced"`
	DefaultCountryCodes int        `json:"defaultCountryCodes"`
	Errors              []RowError `json:"errors"`
}

// Indicator — индикатор кода для страны: USD за 1 кг и профиль, из которого он взят.
type Indicator struct {
	Code     string
	Country  string
	Profile  string
	PriceUSD decimal.Decimal
}

// Import разбирает файл и сохраняет профиль; профиль с тем же номером заменяется.
func (s *Service) Import(ctx context.Context, userID int64, fileName string, file io.Reader, size int64) (ImportResult, error) {
	result := ImportResult{Errors: []RowError{}}
	book, rowErrors, err := Parse(file, size, fileName)
	if err != nil {
		return result, err
	}
	if len(rowErrors) > 0 {
		result.Errors = rowErrors
		return result, nil
	}
	existing, err := s.repository.RiskIndicatorProfiles(ctx)
	if err != nil {
		return result, err
	}
	for _, profile := range existing {
		result.Replaced = result.Replaced || profile.Number == book.Number
	}
	groups, err := json.Marshal(book.Groups)
	if err != nil {
		return result, err
	}
	rows := make([]repository.RiskIndicator, 0, len(book.Rows))
	group := groupIndex(book.Groups, DefaultCountry)
	for _, row := range book.Rows {
		prices := make([]*string, len(row.Prices))
		for index := range row.Prices {
			if row.Prices[index] != "" {
				prices[index] = &row.Prices[index]
			}
		}
		encoded, err := json.Marshal(prices)
		if err != nil {
			return result, err
		}
		rows = append(rows, repository.RiskIndicator{Code: row.Code, Prices: repository.JSON(encoded)})
		if group >= 0 && row.Prices[group] != "" {
			result.DefaultCountryCodes++
		}
	}
	saved, err := s.repository.ReplaceRiskIndicatorProfile(ctx, repository.RiskIndicatorProfile{
		Number: service.TruncateRunes(book.Number, 60), FileName: service.TruncateRunes(fileName, 255),
		Groups: repository.JSON(groups), CodeCount: len(rows), ImportedBy: &userID,
	}, rows)
	if err != nil {
		return result, err
	}
	profile := profileInfo(saved)
	result.Profile = &profile
	return result, nil
}

// Profiles — загруженные профили.
func (s *Service) Profiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.repository.RiskIndicatorProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, profileInfo(row))
	}
	return profiles, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repository.DeleteRiskIndicatorProfile(ctx, id)
}

// Lookup — индикаторы кодов для страны. loaded=false — ни одного профиля не загружено.
func (s *Service) Lookup(ctx context.Context, codes []string, country string) (map[string]Indicator, bool, error) {
	profiles, err := s.repository.RiskIndicatorProfiles(ctx)
	if err != nil || len(profiles) == 0 {
		return map[string]Indicator{}, false, err
	}
	rows, err := s.repository.RiskIndicatorsByCodes(ctx, codes)
	if err != nil {
		return nil, true, err
	}
	return Pick(rows, country), true, nil
}

// Pick выбирает индикатор каждого кода для страны. Страна в нескольких группах профиля — берётся
// самая узкая группа (отдельная колонка «CN» точнее общего списка). Код в нескольких профилях —
// берётся больший индикатор: таможня сверяет стоимость с каждым действующим профилем.
func Pick(rows []repository.RiskIndicatorRow, country string) map[string]Indicator {
	country = NormalizeCountry(country)
	result := map[string]Indicator{}
	for _, row := range rows {
		var groups [][]string
		var prices []*string
		if json.Unmarshal(row.Groups, &groups) != nil || json.Unmarshal(row.Prices, &prices) != nil {
			continue
		}
		group := groupIndex(groups, country)
		if group < 0 || group >= len(prices) || prices[group] == nil {
			continue
		}
		price, err := decimal.NewFromString(*prices[group])
		if err != nil || !price.IsPositive() {
			continue
		}
		if current, exists := result[row.Code]; exists && !price.GreaterThan(current.PriceUSD) {
			continue
		}
		result[row.Code] = Indicator{Code: row.Code, Country: country, Profile: row.ProfileNumber, PriceUSD: price}
	}
	return result
}

// NormalizeCountry — код страны для поиска: «cn » → «CN», пусто → CN.
func NormalizeCountry(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return DefaultCountry
	}
	return value
}

// ValidCountry — двухбуквенный код ISO или «00» (страна неизвестна), как в файлах профилей.
func ValidCountry(value string) bool {
	return countryToken.MatchString(NormalizeCountry(value))
}

func groupIndex(groups [][]string, country string) int {
	best, size := -1, 0
	for index, group := range groups {
		for _, value := range group {
			if value == country && (best < 0 || len(group) < size) {
				best, size = index, len(group)
			}
		}
	}
	return best
}

func profileInfo(row repository.RiskIndicatorProfile) Profile {
	var groups [][]string
	_ = json.Unmarshal(row.Groups, &groups)
	labels := make([]string, 0, len(groups))
	for _, group := range groups {
		labels = append(labels, groupLabel(group))
	}
	return Profile{
		ID: row.ID, Number: row.Number, FileName: row.FileName, CodeCount: row.CodeCount, Groups: labels,
		ImportedAt: row.ImportedAt.UTC().Format(time.RFC3339), ImportedByName: row.ImportedByName,
	}
}

// IsNotFound — профиля для удаления нет.
func IsNotFound(err error) bool { return errors.Is(err, repository.ErrNotFound) }

// SortedCodes — коды без повторов (для запроса индикаторов сметы).
func SortedCodes(codes []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, code := range codes {
		if code != "" && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	sort.Strings(result)
	return result
}
