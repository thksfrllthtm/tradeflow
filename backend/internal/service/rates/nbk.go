package rates

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

const nbkRatesURL = "https://nationalbank.kz/rss/get_rates.cfm?fdate="
const nbkLatestRatesURL = "https://nationalbank.kz/rss/rates_all.xml"

// NBKRate is the official National Bank exchange rate for one unit of a currency (USD for
// the calculation; EUR is shown for reference on the home page).
// The copy calculator uses this value as a convenient starting point, while
// the user can still edit the rate manually before calculating.
type NBKRate struct {
	Currency string `json:"currency"`
	Rate     string `json:"rate"`
	Date     string `json:"date"`
	Source   string `json:"source"`
}

type nbkRatesFeed struct {
	Channel struct {
		Items []nbkRateItem `xml:"item"`
	} `xml:"channel"`
}

type nbkRateItem struct {
	Title       string `xml:"title"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Quant       string `xml:"quant"`
}

// FetchNBKRate reads the official NBK RSS service for the selected date and currency.
// NBK documents this service with a DD.MM.YYYY date parameter and returns the
// rate in <description>, with the nominal in <quant>.
func FetchNBKRate(ctx context.Context, assessmentDate, currency string) (NBKRate, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(assessmentDate))
	if err != nil {
		return NBKRate{}, &service.ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}

	client := &http.Client{Timeout: 12 * time.Second}

	// First try the selected date itself. The documented service uses
	// DD.MM.YYYY; the other two forms cover older NBK deployments.
	if rate, ok, parseErr := fetchNBKRateForDate(ctx, client, date, currency, "НБРК"); parseErr != nil {
		return NBKRate{}, parseErr
	} else if ok {
		return rate, nil
	}

	// The public current-rate feed is the reliable source exposed by NBK. If
	// the date-specific endpoint does not return a row, use the current rate
	// explicitly marked as current rather than failing the calculation.
	if latestFeed, latestErr := fetchNBKFeed(ctx, client, nbkLatestRatesURL); latestErr == nil {
		latestDate := parseNBKDate(latestFeed.Channel.Items)
		latestParsed, parseErr := time.Parse("2006-01-02", latestDate)
		if parseErr == nil {
			source := "НБРК · текущий курс"
			if latestParsed.Equal(date) {
				source = "НБРК"
			}
			if rate, ok, rateErr := parseNBKRate(latestFeed.Channel.Items, currency, latestDate, source); rateErr != nil {
				return NBKRate{}, rateErr
			} else if ok {
				return rate, nil
			}
		}
	}

	// For weekends and dates before the latest publication, use the nearest
	// previous published business day, never a later date.
	for daysBack := 1; daysBack <= 7; daysBack++ {
		candidateDate := date.AddDate(0, 0, -daysBack)
		if rate, ok, parseErr := fetchNBKRateForDate(ctx, client, candidateDate, currency, "НБРК · предыдущий опубликованный курс"); parseErr != nil {
			return NBKRate{}, parseErr
		} else if ok {
			return rate, nil
		}
	}

	return NBKRate{}, fmt.Errorf("НБРК не вернул курс %s за %s и не предоставил последний доступный курс", currency, date.Format("02.01.2006"))
}

func fetchNBKRateForDate(ctx context.Context, client *http.Client, date time.Time, currency, source string) (NBKRate, bool, error) {
	for _, dateQuery := range []string{date.Format("02.01.2006"), date.Format("2006-01-02"), date.Format("02.01.06")} {
		feed, fetchErr := fetchNBKFeed(ctx, client, nbkRatesURL+dateQuery)
		if fetchErr != nil {
			continue
		}
		if rate, ok, parseErr := parseNBKRate(feed.Channel.Items, currency, date.Format("2006-01-02"), source); parseErr != nil {
			return NBKRate{}, false, parseErr
		} else if ok {
			return rate, true, nil
		}
	}
	return NBKRate{}, false, nil
}

func fetchNBKFeed(ctx context.Context, client *http.Client, endpoint string) (nbkRatesFeed, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nbkRatesFeed{}, fmt.Errorf("не удалось подготовить запрос к НБРК: %w", err)
	}
	request.Header.Set("Accept", "application/xml, text/xml;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "East-Trans-Logistic-Calculator/1.0")
	response, err := client.Do(request)
	if err != nil {
		return nbkRatesFeed{}, fmt.Errorf("НБРК временно недоступен: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nbkRatesFeed{}, fmt.Errorf("НБРК вернул HTTP %d", response.StatusCode)
	}
	feed, err := decodeNBKFeed(response.Body)
	if err != nil {
		return nbkRatesFeed{}, fmt.Errorf("НБРК вернул некорректный XML: %w", err)
	}
	return feed, nil
}

func decodeNBKFeed(reader io.Reader) (nbkRatesFeed, error) {
	var feed nbkRatesFeed
	decoder := xml.NewDecoder(io.LimitReader(reader, 2<<20))
	if err := decoder.Decode(&feed); err != nil {
		return nbkRatesFeed{}, err
	}
	return feed, nil
}

func parseNBKRate(items []nbkRateItem, currency, date, source string) (NBKRate, bool, error) {
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Title), currency) {
			continue
		}
		rate, err := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(item.Description, ",", ".")))
		if err != nil || !rate.GreaterThan(decimal.Zero) {
			return NBKRate{}, false, fmt.Errorf("НБРК вернул некорректный курс %s", currency)
		}
		quant := decimal.NewFromInt(1)
		if strings.TrimSpace(item.Quant) != "" {
			quant, err = decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(item.Quant, ",", ".")))
			if err != nil || !quant.GreaterThan(decimal.Zero) {
				return NBKRate{}, false, fmt.Errorf("НБРК вернул некорректный номинал %s", currency)
			}
		}
		return NBKRate{Currency: strings.ToUpper(currency), Rate: rate.Div(quant).String(), Date: date, Source: source}, true, nil
	}
	return NBKRate{}, false, nil
}

func parseNBKDate(items []nbkRateItem) string {
	for _, item := range items {
		if strings.TrimSpace(item.PubDate) == "" {
			continue
		}
		for _, layout := range []string{"02.01.2006", "02.01.06", time.RFC1123, time.RFC1123Z, time.RFC822, time.RFC822Z} {
			if parsed, err := time.Parse(layout, strings.TrimSpace(item.PubDate)); err == nil {
				return parsed.Format("2006-01-02")
			}
		}
	}
	return ""
}
