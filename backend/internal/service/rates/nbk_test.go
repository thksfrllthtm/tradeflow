package rates

import (
	"strings"
	"testing"
)

func TestParseNBKRateReadsUSDAndEURFromOneFeed(t *testing.T) {
	feed, err := decodeNBKFeed(strings.NewReader(`<rss><channel>
		<item><title>USD</title><pubDate>29.09.2026</pubDate><description>472.15</description><quant>1</quant></item>
		<item><title>EUR</title><pubDate>29.09.2026</pubDate><description>551,30</description><quant>1</quant></item>
		<item><title>KGS</title><pubDate>29.09.2026</pubDate><description>540.00</description><quant>100</quant></item>
	</channel></rss>`))
	if err != nil {
		t.Fatal(err)
	}
	for currency, want := range map[string]string{"USD": "472.15", "EUR": "551.3", "KGS": "5.4"} {
		rate, ok, err := parseNBKRate(feed.Channel.Items, currency, "2026-09-29", "НБРК")
		if err != nil || !ok || rate.Rate != want || rate.Currency != currency {
			t.Errorf("%s: got %+v ok=%v err=%v, want rate %s", currency, rate, ok, err, want)
		}
	}
	if _, ok, _ := parseNBKRate(feed.Channel.Items, "GBP", "2026-09-29", "НБРК"); ok {
		t.Fatal("a currency missing from the feed must not be found")
	}
}
