package main

import (
	"context"
	"errors"
	"testing"
	"time"

	exchangemodel "gogogo/internal/exchange"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
)

func TestSyncKlinesHandlesDescendingPages(t *testing.T) {
	ctx := context.Background()
	db, err := marketdata.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	start := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	client := &fakeMarketClient{
		klinePages: [][]marketdata.Candle{
			{
				testCandle(start.Add(10*time.Minute), "3"),
				testCandle(start.Add(5*time.Minute), "2"),
				testCandle(start, "1"),
			},
			{
				// OneBullEx can return the latest bucket again for the next page.
				testCandle(start.Add(10*time.Minute), "3"),
			},
		},
	}

	count, err := syncKlines(ctx, syncRequest{
		repo:       marketdata.NewSQLiteRepository(db),
		client:     client,
		exchange:   "onebullex",
		marketType: marketdata.MarketTypePerpetual,
		marketName: "perpetual",
		symbol:     "BTCUSDT",
		interval:   "5m",
		start:      start,
		end:        start.Add(15 * time.Minute),
		limit:      3,
	})
	if err != nil {
		t.Fatalf("sync klines: %v", err)
	}
	if count != 3 {
		t.Fatalf("synced count = %d, want 3", count)
	}

	candles, err := marketdata.NewSQLiteRepository(db).ListCandles(ctx, marketdata.CandleQuery{
		Exchange:   "onebullex",
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "5m",
		Start:      start,
		End:        start.Add(20 * time.Minute),
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("list candles: %v", err)
	}
	if len(candles) != 3 {
		t.Fatalf("stored candles = %d, want 3", len(candles))
	}
	for i, candle := range candles {
		want := start.Add(time.Duration(i) * 5 * time.Minute)
		if !candle.OpenTime.Equal(want) {
			t.Fatalf("candle %d open time = %s, want %s", i, candle.OpenTime, want)
		}
	}
}

func TestSyncKlinesIncrementalUsesLatestLocalCandle(t *testing.T) {
	ctx := context.Background()
	db, err := marketdata.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	repo := marketdata.NewSQLiteRepository(db)
	start := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	latestLocal := start.Add(30 * time.Minute)
	if err := repo.UpsertCandle(ctx, testCandle(latestLocal, "7")); err != nil {
		t.Fatalf("seed latest candle: %v", err)
	}

	client := &fakeMarketClient{
		klinePages: [][]marketdata.Candle{
			{
				testCandle(latestLocal.Add(-10*time.Minute), "5"),
				testCandle(latestLocal.Add(-5*time.Minute), "6"),
				testCandle(latestLocal, "7"),
				testCandle(latestLocal.Add(5*time.Minute), "8"),
			},
		},
	}

	count, err := syncKlines(ctx, syncRequest{
		repo:         repo,
		client:       client,
		exchange:     "onebullex",
		marketType:   marketdata.MarketTypePerpetual,
		marketName:   "perpetual",
		symbol:       "BTCUSDT",
		interval:     "5m",
		start:        start,
		end:          latestLocal.Add(10 * time.Minute),
		limit:        120,
		incremental:  true,
		klineOverlap: 2,
	})
	if err != nil {
		t.Fatalf("sync incremental klines: %v", err)
	}
	if count != 4 {
		t.Fatalf("synced count = %d, want 4", count)
	}
	if len(client.klineRequests) != 1 {
		t.Fatalf("kline requests = %d, want 1", len(client.klineRequests))
	}
	wantStart := latestLocal.Add(-10 * time.Minute)
	if !client.klineRequests[0].StartTime.Equal(wantStart) {
		t.Fatalf("request start = %s, want %s", client.klineRequests[0].StartTime, wantStart)
	}
}

func testCandle(openTime time.Time, close string) marketdata.Candle {
	return marketdata.Candle{
		Exchange:    "onebullex",
		MarketType:  marketdata.MarketTypePerpetual,
		Symbol:      "BTCUSDT",
		Interval:    "5m",
		OpenTime:    openTime,
		CloseTime:   openTime.Add(5 * time.Minute),
		Open:        close,
		High:        close,
		Low:         close,
		Close:       close,
		Volume:      "1",
		QuoteVolume: "1",
		Source:      "onebullex",
	}
}

type fakeMarketClient struct {
	klinePages    [][]marketdata.Candle
	klineCalls    int
	klineRequests []exchangemodel.KlineRequest
}

func (f *fakeMarketClient) Klines(_ context.Context, request exchangemodel.KlineRequest) ([]marketdata.Candle, error) {
	f.klineRequests = append(f.klineRequests, request)
	if f.klineCalls >= len(f.klinePages) {
		return nil, nil
	}
	page := f.klinePages[f.klineCalls]
	f.klineCalls++
	return page, nil
}

func (f *fakeMarketClient) FundingRates(context.Context, exchangemodel.FundingRateRequest) ([]marketdata.FundingRate, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMarketClient) LatestMarkPrice(context.Context, string) (marketdata.MarkPrice, error) {
	return marketdata.MarkPrice{}, errors.New("not implemented")
}

func (f *fakeMarketClient) IndexPrices(context.Context, string) ([]marketdata.IndexPrice, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMarketClient) RecentTrades(context.Context, string, int) ([]marketdata.Trade, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMarketClient) OrderBook(context.Context, string, int) (marketdata.OrderBook, error) {
	return marketdata.OrderBook{}, errors.New("not implemented")
}

func (f *fakeMarketClient) SymbolSpecs(context.Context) ([]portfolio.ContractSpec, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMarketClient) LeverageBrackets(context.Context, string) ([]portfolio.LeverageBracket, error) {
	return nil, errors.New("not implemented")
}
