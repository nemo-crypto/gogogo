package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gogogo/internal/exchange/binance"
	"gogogo/internal/marketdata"
)

func main() {
	var (
		dsn        = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange   = flag.String("exchange", "binance", "exchange name")
		dataset    = flag.String("dataset", "klines", "dataset: klines, funding, mark-price")
		marketType = flag.String("market", "spot", "market type: spot or perpetual")
		symbols    = flag.String("symbols", "BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT", "comma-separated symbols")
		interval   = flag.String("interval", "1h", "kline interval")
		start      = flag.String("start", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end        = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		limit      = flag.Int("limit", 1000, "max klines per symbol, capped at 1000")
	)
	flag.Parse()

	if *exchange != "binance" {
		log.Fatalf("unsupported exchange %q", *exchange)
	}

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		log.Fatalf("parse start: %v", err)
	}
	endTime, err := time.Parse(time.RFC3339, *end)
	if err != nil {
		log.Fatalf("parse end: %v", err)
	}
	if !endTime.After(startTime) {
		log.Fatal("end must be after start")
	}

	parsedMarketType, err := parseMarketType(*marketType)
	if err != nil {
		log.Fatal(err)
	}
	parsedSymbols := parseSymbols(*symbols)
	if len(parsedSymbols) == 0 {
		log.Fatal("at least one symbol is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	repo := marketdata.NewSQLiteRepository(db)
	client := binance.NewClient()

	total := 0
	for _, symbol := range parsedSymbols {
		count, err := syncSymbol(ctx, syncRequest{
			repo:       repo,
			client:     client,
			exchange:   *exchange,
			dataset:    *dataset,
			marketType: parsedMarketType,
			marketName: *marketType,
			symbol:     symbol,
			interval:   *interval,
			start:      startTime,
			end:        endTime,
			limit:      *limit,
		})
		if err != nil {
			log.Fatal(err)
		}
		total += count
	}

	log.Printf("market sync complete: dataset=%s total_rows=%d dsn=%s", *dataset, total, *dsn)
}

type syncRequest struct {
	repo       *marketdata.SQLiteRepository
	client     *binance.Client
	exchange   string
	dataset    string
	marketType marketdata.MarketType
	marketName string
	symbol     string
	interval   string
	start      time.Time
	end        time.Time
	limit      int
}

func syncSymbol(ctx context.Context, request syncRequest) (int, error) {
	switch strings.ToLower(strings.TrimSpace(request.dataset)) {
	case "klines", "candles":
		return syncKlines(ctx, request)
	case "funding", "funding-rates":
		if request.marketType != marketdata.MarketTypePerpetual {
			return 0, fmt.Errorf("funding dataset requires market=perpetual")
		}
		return syncFundingRates(ctx, request)
	case "mark-price", "mark-prices":
		if request.marketType != marketdata.MarketTypePerpetual {
			return 0, fmt.Errorf("mark-price dataset requires market=perpetual")
		}
		return syncMarkPrice(ctx, request)
	default:
		return 0, fmt.Errorf("unsupported dataset %q", request.dataset)
	}
}

func syncKlines(ctx context.Context, request syncRequest) (int, error) {
	total := 0
	pageStart := request.start
	page := 0

	for pageStart.Before(request.end) {
		page++
		if page > 10000 {
			return total, fmt.Errorf("too many kline pages for %s", request.symbol)
		}

		candles, err := request.client.Klines(ctx, binance.KlineRequest{
			MarketType: request.marketType,
			Symbol:     request.symbol,
			Interval:   request.interval,
			StartTime:  pageStart,
			EndTime:    request.end,
			Limit:      request.limit,
		})
		if err != nil {
			return total, fmt.Errorf("sync %s %s %s klines: %w", request.exchange, request.marketName, request.symbol, err)
		}
		if len(candles) == 0 {
			break
		}

		for _, candle := range candles {
			if err := request.repo.UpsertCandle(ctx, candle); err != nil {
				return total, fmt.Errorf("write candle %s %s %s: %w", request.marketName, request.symbol, candle.OpenTime.Format(time.RFC3339), err)
			}
		}

		total += len(candles)
		nextStart := candles[len(candles)-1].OpenTime.Add(time.Millisecond)
		if !nextStart.After(pageStart) {
			return total, fmt.Errorf("kline pagination did not advance for %s", request.symbol)
		}
		pageStart = nextStart

		if len(candles) < normalizeLimit(request.limit) {
			break
		}
	}

	log.Printf("synced %d candles: exchange=%s market=%s symbol=%s interval=%s pages=%d", total, request.exchange, request.marketName, request.symbol, request.interval, page)
	return total, nil
}

func syncFundingRates(ctx context.Context, request syncRequest) (int, error) {
	rates, err := request.client.FundingRates(ctx, binance.FundingRateRequest{
		Symbol:    request.symbol,
		StartTime: request.start,
		EndTime:   request.end,
		Limit:     request.limit,
	})
	if err != nil {
		return 0, fmt.Errorf("sync %s %s funding rates: %w", request.exchange, request.symbol, err)
	}

	for _, rate := range rates {
		if err := request.repo.UpsertFundingRate(ctx, rate); err != nil {
			return 0, fmt.Errorf("write funding rate %s %s: %w", request.symbol, rate.FundingTime.Format(time.RFC3339), err)
		}
	}

	log.Printf("synced %d funding rates: exchange=%s symbol=%s", len(rates), request.exchange, request.symbol)
	return len(rates), nil
}

func syncMarkPrice(ctx context.Context, request syncRequest) (int, error) {
	price, err := request.client.LatestMarkPrice(ctx, request.symbol)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s mark price: %w", request.exchange, request.symbol, err)
	}
	if err := request.repo.UpsertMarkPrice(ctx, price); err != nil {
		return 0, fmt.Errorf("write mark price %s %s: %w", request.symbol, price.EventTime.Format(time.RFC3339), err)
	}

	log.Printf("synced 1 mark price: exchange=%s symbol=%s event_time=%s", request.exchange, request.symbol, price.EventTime.Format(time.RFC3339))
	return 1, nil
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func parseMarketType(value string) (marketdata.MarketType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "spot":
		return marketdata.MarketTypeSpot, nil
	case "perpetual", "futures", "future":
		return marketdata.MarketTypePerpetual, nil
	default:
		return "", fmt.Errorf("unsupported market type %q", value)
	}
}

func parseSymbols(value string) []string {
	parts := strings.Split(value, ",")
	symbols := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	return symbols
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 1000
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}
