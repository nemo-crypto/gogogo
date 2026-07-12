package marketdata

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSQLiteRepositoryUpsertAndListCandles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)
	openTime := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)

	candle := Candle{
		Exchange:    "Binance",
		MarketType:  MarketTypePerpetual,
		Symbol:      "btcusdt",
		Interval:    "1h",
		OpenTime:    openTime,
		CloseTime:   openTime.Add(time.Hour),
		Open:        "60000.10",
		High:        "61000.00",
		Low:         "59500.00",
		Close:       "60800.00",
		Volume:      "12.5",
		QuoteVolume: "760000.00",
		TradeCount:  120,
	}
	if err := repo.UpsertCandle(ctx, candle); err != nil {
		t.Fatalf("upsert candle: %v", err)
	}

	candle.Close = "60900.00"
	candle.TradeCount = 130
	if err := repo.UpsertCandle(ctx, candle); err != nil {
		t.Fatalf("upsert duplicate candle: %v", err)
	}

	candles, err := repo.ListCandles(ctx, CandleQuery{
		Exchange:   "binance",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "1h",
		Start:      openTime.Add(-time.Hour),
		End:        openTime.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("list candles: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("candles length = %d, want 1", len(candles))
	}
	if candles[0].Close != "60900.00" {
		t.Fatalf("close = %q, want updated close", candles[0].Close)
	}
	if candles[0].Exchange != "binance" {
		t.Fatalf("exchange = %q, want binance", candles[0].Exchange)
	}
	if candles[0].Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", candles[0].Symbol)
	}
}

func TestSQLiteRepositoryFundingRatesAndMarkPrices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)
	eventTime := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)

	if err := repo.UpsertFundingRate(ctx, FundingRate{
		Exchange:    "binance",
		Symbol:      "BTCUSDT",
		FundingTime: eventTime,
		FundingRate: "0.0001",
		MarkPrice:   "61000.00",
		IndexPrice:  "60990.00",
	}); err != nil {
		t.Fatalf("upsert funding rate: %v", err)
	}

	rates, err := repo.ListFundingRates(ctx, FundingRateQuery{
		Exchange: "binance",
		Symbol:   "BTCUSDT",
		Start:    eventTime.Add(-time.Hour),
		End:      eventTime.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("list funding rates: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("funding rates length = %d, want 1", len(rates))
	}
	if rates[0].FundingRate != "0.0001" {
		t.Fatalf("funding rate = %q, want 0.0001", rates[0].FundingRate)
	}

	nextFunding := eventTime.Add(8 * time.Hour)
	if err := repo.UpsertMarkPrice(ctx, MarkPrice{
		Exchange:             "binance",
		Symbol:               "btcusdt",
		EventTime:            eventTime,
		MarkPrice:            "61010.00",
		IndexPrice:           "60995.00",
		EstimatedSettlePrice: "61000.00",
		NextFundingTime:      nextFunding,
	}); err != nil {
		t.Fatalf("upsert mark price: %v", err)
	}

	latest, err := repo.LatestMarkPrice(ctx, "BINANCE", "BTCUSDT")
	if err != nil {
		t.Fatalf("latest mark price: %v", err)
	}
	if latest.MarkPrice != "61010.00" {
		t.Fatalf("mark price = %q, want 61010.00", latest.MarkPrice)
	}
	if !latest.NextFundingTime.Equal(nextFunding) {
		t.Fatalf("next funding time = %s, want %s", latest.NextFundingTime, nextFunding)
	}
}

func TestSQLiteRepositoryLatestMarkPriceNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)

	_, err := repo.LatestMarkPrice(ctx, "binance", "BTCUSDT")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteRepositoryCreateCandleSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)
	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 2; i++ {
		if err := repo.UpsertCandle(ctx, Candle{
			Exchange:    "binance",
			MarketType:  MarketTypeSpot,
			Symbol:      "BTCUSDT",
			Interval:    "1h",
			OpenTime:    start.Add(time.Duration(i) * time.Hour),
			CloseTime:   start.Add(time.Duration(i+1) * time.Hour),
			Open:        "100.00",
			High:        "110.00",
			Low:         "90.00",
			Close:       "105.00",
			Volume:      "10.00",
			QuoteVolume: "1050.00",
			TradeCount:  100,
			Source:      "binance",
		}); err != nil {
			t.Fatalf("upsert candle %d: %v", i, err)
		}
	}

	snapshot, coverage, err := repo.CreateCandleSnapshot(ctx, CandleSnapshotRequest{
		Name: "unit-test",
		Query: CandleQuery{
			Exchange:   "binance",
			MarketType: MarketTypeSpot,
			Symbol:     "BTCUSDT",
			Interval:   "1h",
			Start:      start,
			End:        start.Add(2 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	if snapshot.ID <= 0 {
		t.Fatalf("snapshot id = %d, want positive", snapshot.ID)
	}
	if snapshot.CandleCount != 2 || snapshot.ExpectedCount != 2 || snapshot.MissingCount != 0 {
		t.Fatalf("snapshot counts = candles:%d expected:%d missing:%d, want 2/2/0", snapshot.CandleCount, snapshot.ExpectedCount, snapshot.MissingCount)
	}
	if snapshot.DataHash == "" {
		t.Fatal("snapshot data hash is empty")
	}
	if !coverage.Complete() {
		t.Fatal("coverage complete = false, want true")
	}
}

func newTestRepository(t *testing.T, ctx context.Context) *SQLiteRepository {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	db.SetMaxOpenConns(1)

	if err := InitSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	return NewSQLiteRepository(db)
}
