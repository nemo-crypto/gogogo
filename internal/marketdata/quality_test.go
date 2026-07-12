package marketdata

import (
	"testing"
	"time"
)

func TestCheckCandleCoverageFindsGaps(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	candles := []Candle{
		testCandleAt(start),
		testCandleAt(start.Add(time.Hour)),
		testCandleAt(start.Add(3 * time.Hour)),
	}

	coverage, err := CheckCandleCoverage(candles, CandleQuery{
		Exchange:   "binance",
		MarketType: MarketTypeSpot,
		Symbol:     "BTCUSDT",
		Interval:   "1h",
		Start:      start,
		End:        start.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("check coverage: %v", err)
	}

	if coverage.Complete() {
		t.Fatal("coverage complete = true, want false")
	}
	if coverage.ExpectedCount != 4 {
		t.Fatalf("expected count = %d, want 4", coverage.ExpectedCount)
	}
	if coverage.CandleCount != 3 {
		t.Fatalf("candle count = %d, want 3", coverage.CandleCount)
	}
	if coverage.MissingCount != 1 {
		t.Fatalf("missing count = %d, want 1", coverage.MissingCount)
	}
	if len(coverage.Gaps) != 1 {
		t.Fatalf("gaps length = %d, want 1", len(coverage.Gaps))
	}
	if !coverage.Gaps[0].Start.Equal(start.Add(2 * time.Hour)) {
		t.Fatalf("gap start = %s, want %s", coverage.Gaps[0].Start, start.Add(2*time.Hour))
	}
	if !coverage.Gaps[0].End.Equal(start.Add(3 * time.Hour)) {
		t.Fatalf("gap end = %s, want %s", coverage.Gaps[0].End, start.Add(3*time.Hour))
	}
}

func TestCandleDataHashIsDeterministic(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	first := []Candle{
		testCandleAt(start.Add(time.Hour)),
		testCandleAt(start),
	}
	second := []Candle{
		testCandleAt(start),
		testCandleAt(start.Add(time.Hour)),
	}

	firstHash := CandleDataHash(first)
	secondHash := CandleDataHash(second)
	if firstHash == "" {
		t.Fatal("hash is empty")
	}
	if firstHash != secondHash {
		t.Fatalf("hash mismatch: %s != %s", firstHash, secondHash)
	}
}

func testCandleAt(openTime time.Time) Candle {
	return Candle{
		Exchange:    "binance",
		MarketType:  MarketTypeSpot,
		Symbol:      "BTCUSDT",
		Interval:    "1h",
		OpenTime:    openTime,
		CloseTime:   openTime.Add(time.Hour),
		Open:        "100.00",
		High:        "110.00",
		Low:         "90.00",
		Close:       "105.00",
		Volume:      "10.00",
		QuoteVolume: "1050.00",
		TradeCount:  100,
		Source:      "binance",
	}
}
