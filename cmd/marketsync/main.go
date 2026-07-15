package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"gogogo/internal/config"
	exchangemodel "gogogo/internal/exchange"
	"gogogo/internal/exchange/onebullex"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/storage"
)

func main() {
	var (
		dsn          = flag.String("dsn", env("DATABASE_DSN", "data.db"), "sqlite database path")
		exchange     = flag.String("exchange", env("EXCHANGE_NAME", onebullex.ExchangeName), "exchange name: onebullex")
		dataset      = flag.String("dataset", "klines", "dataset: klines, funding, mark-price, index-price, trades, order-book, contract-specs, leverage-brackets")
		marketType   = flag.String("market", "perpetual", "market type: perpetual")
		symbols      = flag.String("symbols", "BTCUSDT", "comma-separated symbols")
		interval     = flag.String("interval", "5m", "kline interval")
		start        = flag.String("start", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end          = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		limit        = flag.Int("limit", 1500, "max klines per symbol, capped at 1500")
		watch        = flag.Bool("watch", false, "keep polling public market data")
		pollEvery    = flag.Duration("poll-interval", 15*time.Second, "poll interval when -watch is enabled")
		incremental  = flag.Bool("incremental", false, "reuse local high-water marks to reduce repeated polling/writes")
		klineOverlap = flag.Int("kline-overlap", 3, "recent candles to re-sync when -incremental is enabled")
		retention    = flag.Duration("retention", 0, "delete mark prices older than this duration after each mark-price sync; 0 disables")
	)
	flag.Parse()

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

	ctx := context.Background()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init sqlite schema: %v", err)
	}

	repo := marketdata.NewSQLiteRepository(db)
	portfolioRepo := portfolio.NewSQLiteRepository(db)
	client, err := newMarketClient(*exchange)
	if err != nil {
		log.Fatal(err)
	}
	exchangeName := normalizeExchangeName(*exchange)

	request := syncBatchRequest{
		repo:         repo,
		portfolio:    portfolioRepo,
		client:       client,
		exchange:     exchangeName,
		dataset:      *dataset,
		marketType:   parsedMarketType,
		marketName:   *marketType,
		symbols:      parsedSymbols,
		interval:     *interval,
		start:        startTime,
		end:          endTime,
		limit:        *limit,
		watch:        *watch,
		pollEvery:    *pollEvery,
		incremental:  *incremental,
		klineOverlap: *klineOverlap,
		retention:    *retention,
	}

	if request.watch {
		if err := watchPublicMarketData(ctx, request); err != nil {
			log.Fatal(err)
		}
		return
	}

	total, err := syncBatch(ctx, request)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("market sync complete: dataset=%s total_rows=%d dsn=%s", *dataset, total, *dsn)
}

type syncBatchRequest struct {
	repo         *marketdata.SQLiteRepository
	portfolio    *portfolio.SQLiteRepository
	client       marketClient
	exchange     string
	dataset      string
	marketType   marketdata.MarketType
	marketName   string
	symbols      []string
	interval     string
	start        time.Time
	end          time.Time
	limit        int
	watch        bool
	pollEvery    time.Duration
	incremental  bool
	klineOverlap int
	retention    time.Duration
}

func syncBatch(ctx context.Context, request syncBatchRequest) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if isGlobalDataset(request.dataset) {
		return syncGlobalDataset(ctx, request)
	}

	total := 0
	for _, symbol := range request.symbols {
		count, err := syncSymbol(ctx, syncRequest{
			repo:         request.repo,
			portfolio:    request.portfolio,
			client:       request.client,
			exchange:     request.exchange,
			dataset:      request.dataset,
			marketType:   request.marketType,
			marketName:   request.marketName,
			symbol:       symbol,
			interval:     request.interval,
			start:        request.start,
			end:          request.end,
			limit:        request.limit,
			incremental:  request.incremental,
			klineOverlap: request.klineOverlap,
			retention:    request.retention,
		})
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

func watchPublicMarketData(ctx context.Context, request syncBatchRequest) error {
	if request.pollEvery <= 0 {
		request.pollEvery = 15 * time.Second
	}
	log.Printf("market sync watch started: dataset=%s symbols=%s poll_interval=%s", request.dataset, strings.Join(request.symbols, ","), request.pollEvery)

	for {
		current := request
		now := time.Now().UTC()
		switch {
		case isKlineDataset(current.dataset):
			current.end = now
			current.start = current.end.Add(-lookbackForInterval(current.interval, current.limit))
		case isFundingDataset(current.dataset):
			current.end = now
			current.start = current.end.Add(-lookbackForFunding(current.limit))
		}
		total, err := syncBatch(ctx, current)
		if err != nil {
			log.Printf("market sync watch error: %v", err)
		} else {
			log.Printf("market sync watch tick complete: dataset=%s rows=%d", current.dataset, total)
		}

		timer := time.NewTimer(request.pollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type syncRequest struct {
	repo         *marketdata.SQLiteRepository
	portfolio    *portfolio.SQLiteRepository
	client       marketClient
	exchange     string
	dataset      string
	marketType   marketdata.MarketType
	marketName   string
	symbol       string
	interval     string
	start        time.Time
	end          time.Time
	limit        int
	incremental  bool
	klineOverlap int
	retention    time.Duration
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
	case "index-price", "index-prices":
		if request.marketType != marketdata.MarketTypePerpetual {
			return 0, fmt.Errorf("index-price dataset requires market=perpetual")
		}
		return syncIndexPrices(ctx, request)
	case "trades", "deals":
		return syncTrades(ctx, request)
	case "order-book", "order-books", "depth":
		return syncOrderBook(ctx, request)
	case "leverage-brackets", "leverage":
		return syncLeverageBrackets(ctx, request)
	default:
		return 0, fmt.Errorf("unsupported dataset %q", request.dataset)
	}
}

func syncKlines(ctx context.Context, request syncRequest) (int, error) {
	total := 0
	syncStart, err := incrementalKlineStart(ctx, request)
	if err != nil {
		return 0, err
	}
	pageStart := syncStart
	page := 0

	for pageStart.Before(request.end) {
		page++
		if page > 10000 {
			return total, fmt.Errorf("too many kline pages for %s", request.symbol)
		}

		candles, err := request.client.Klines(ctx, exchangemodel.KlineRequest{
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
		candles = normalizeKlinePage(candles, pageStart, request.end)
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

	log.Printf("synced %d candles: exchange=%s market=%s symbol=%s interval=%s pages=%d start=%s end=%s incremental=%t", total, request.exchange, request.marketName, request.symbol, request.interval, page, syncStart.Format(time.RFC3339), request.end.Format(time.RFC3339), request.incremental)
	return total, nil
}

func incrementalKlineStart(ctx context.Context, request syncRequest) (time.Time, error) {
	if !request.incremental {
		return request.start, nil
	}
	latest, err := request.repo.LatestCandle(ctx, request.exchange, request.marketType, request.symbol, request.interval)
	if err != nil {
		if errors.Is(err, marketdata.ErrNotFound) {
			return request.start, nil
		}
		return time.Time{}, fmt.Errorf("load latest local candle %s %s: %w", request.symbol, request.interval, err)
	}
	step, err := marketdata.IntervalDuration(request.interval)
	if err != nil {
		return request.start, nil
	}
	overlap := request.klineOverlap
	if overlap < 0 {
		overlap = 0
	}
	start := latest.OpenTime.Add(-time.Duration(overlap) * step)
	if start.Before(request.start) {
		return request.start, nil
	}
	return start, nil
}

func normalizeKlinePage(candles []marketdata.Candle, start time.Time, end time.Time) []marketdata.Candle {
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})

	filtered := candles[:0]
	for _, candle := range candles {
		if candle.OpenTime.Before(start) {
			continue
		}
		if !end.IsZero() && !candle.OpenTime.Before(end) {
			continue
		}
		filtered = append(filtered, candle)
	}
	return filtered
}

func syncFundingRates(ctx context.Context, request syncRequest) (int, error) {
	rates, err := request.client.FundingRates(ctx, exchangemodel.FundingRateRequest{
		Symbol:    request.symbol,
		StartTime: request.start,
		EndTime:   request.end,
		Limit:     request.limit,
	})
	if err != nil {
		return 0, fmt.Errorf("sync %s %s funding rates: %w", request.exchange, request.symbol, err)
	}
	if request.incremental {
		rates, err = filterNewFundingRates(ctx, request, rates)
		if err != nil {
			return 0, err
		}
	}

	for _, rate := range rates {
		if err := request.repo.UpsertFundingRate(ctx, rate); err != nil {
			return 0, fmt.Errorf("write funding rate %s %s: %w", request.symbol, rate.FundingTime.Format(time.RFC3339), err)
		}
	}

	log.Printf("synced %d funding rates: exchange=%s symbol=%s", len(rates), request.exchange, request.symbol)
	return len(rates), nil
}

func filterNewFundingRates(ctx context.Context, request syncRequest, rates []marketdata.FundingRate) ([]marketdata.FundingRate, error) {
	latest, err := request.repo.LatestFundingRate(ctx, request.exchange, request.symbol)
	if err != nil {
		if errors.Is(err, marketdata.ErrNotFound) {
			return rates, nil
		}
		return nil, fmt.Errorf("load latest local funding rate %s: %w", request.symbol, err)
	}
	filtered := rates[:0]
	for _, rate := range rates {
		if rate.FundingTime.After(latest.FundingTime) {
			filtered = append(filtered, rate)
		}
	}
	return filtered, nil
}

func syncMarkPrice(ctx context.Context, request syncRequest) (int, error) {
	price, err := request.client.LatestMarkPrice(ctx, request.symbol)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s mark price: %w", request.exchange, request.symbol, err)
	}
	if err := request.repo.UpsertMarkPrice(ctx, price); err != nil {
		return 0, fmt.Errorf("write mark price %s %s: %w", request.symbol, price.EventTime.Format(time.RFC3339), err)
	}
	if request.retention > 0 {
		deleted, err := request.repo.DeleteMarkPricesBefore(ctx, request.exchange, request.symbol, price.EventTime.Add(-request.retention))
		if err != nil {
			return 0, fmt.Errorf("prune mark prices %s retention=%s: %w", request.symbol, request.retention, err)
		}
		if deleted > 0 {
			log.Printf("pruned %d mark prices: exchange=%s symbol=%s retention=%s", deleted, request.exchange, request.symbol, request.retention)
		}
	}

	log.Printf("synced 1 mark price: exchange=%s symbol=%s event_time=%s", request.exchange, request.symbol, price.EventTime.Format(time.RFC3339))
	return 1, nil
}

func syncIndexPrices(ctx context.Context, request syncRequest) (int, error) {
	prices, err := request.client.IndexPrices(ctx, request.symbol)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s index prices: %w", request.exchange, request.symbol, err)
	}
	for _, price := range prices {
		if err := request.repo.UpsertIndexPrice(ctx, price); err != nil {
			return 0, fmt.Errorf("write index price %s %s: %w", request.symbol, price.EventTime.Format(time.RFC3339), err)
		}
	}
	log.Printf("synced %d index prices: exchange=%s symbol=%s", len(prices), request.exchange, request.symbol)
	return len(prices), nil
}

func syncTrades(ctx context.Context, request syncRequest) (int, error) {
	trades, err := request.client.RecentTrades(ctx, request.symbol, request.limit)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s trades: %w", request.exchange, request.symbol, err)
	}
	for _, trade := range trades {
		if err := request.repo.UpsertTrade(ctx, trade); err != nil {
			return 0, fmt.Errorf("write trade %s %s: %w", request.symbol, trade.TradeID, err)
		}
	}
	log.Printf("synced %d trades: exchange=%s symbol=%s", len(trades), request.exchange, request.symbol)
	return len(trades), nil
}

func syncOrderBook(ctx context.Context, request syncRequest) (int, error) {
	book, err := request.client.OrderBook(ctx, request.symbol, request.limit)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s order book: %w", request.exchange, request.symbol, err)
	}
	if err := request.repo.UpsertOrderBook(ctx, book); err != nil {
		return 0, fmt.Errorf("write order book %s %s: %w", request.symbol, book.EventTime.Format(time.RFC3339), err)
	}
	log.Printf("synced 1 order book: exchange=%s symbol=%s update_id=%d", request.exchange, request.symbol, book.UpdateID)
	return 1, nil
}

func syncGlobalDataset(ctx context.Context, request syncBatchRequest) (int, error) {
	switch strings.ToLower(strings.TrimSpace(request.dataset)) {
	case "contract-specs", "symbols", "symbol-specs":
		specs, err := request.client.SymbolSpecs(ctx)
		if err != nil {
			return 0, fmt.Errorf("sync %s contract specs: %w", request.exchange, err)
		}
		for _, spec := range specs {
			if _, err := request.portfolio.SaveContractSpec(ctx, spec); err != nil {
				return 0, fmt.Errorf("write contract spec %s: %w", spec.Symbol, err)
			}
		}
		log.Printf("synced %d contract specs: exchange=%s", len(specs), request.exchange)
		return len(specs), nil
	default:
		return 0, fmt.Errorf("unsupported global dataset %q", request.dataset)
	}
}

func syncLeverageBrackets(ctx context.Context, request syncRequest) (int, error) {
	brackets, err := request.client.LeverageBrackets(ctx, request.symbol)
	if err != nil {
		return 0, fmt.Errorf("sync %s %s leverage brackets: %w", request.exchange, request.symbol, err)
	}
	for _, bracket := range brackets {
		if _, err := request.portfolio.SaveLeverageBracket(ctx, bracket); err != nil {
			return 0, fmt.Errorf("write leverage bracket %s #%d: %w", bracket.Symbol, bracket.Bracket, err)
		}
	}
	log.Printf("synced %d leverage brackets: exchange=%s symbol=%s", len(brackets), request.exchange, request.symbol)
	return len(brackets), nil
}

func env(key string, fallback string) string {
	return config.Env(key, fallback)
}

type marketClient interface {
	Klines(ctx context.Context, request exchangemodel.KlineRequest) ([]marketdata.Candle, error)
	FundingRates(ctx context.Context, request exchangemodel.FundingRateRequest) ([]marketdata.FundingRate, error)
	LatestMarkPrice(ctx context.Context, symbol string) (marketdata.MarkPrice, error)
	IndexPrices(ctx context.Context, symbol string) ([]marketdata.IndexPrice, error)
	RecentTrades(ctx context.Context, symbol string, limit int) ([]marketdata.Trade, error)
	OrderBook(ctx context.Context, symbol string, level int) (marketdata.OrderBook, error)
	SymbolSpecs(ctx context.Context) ([]portfolio.ContractSpec, error)
	LeverageBrackets(ctx context.Context, symbol string) ([]portfolio.LeverageBracket, error)
}

func newMarketClient(exchangeName string) (marketClient, error) {
	switch normalizeExchangeName(exchangeName) {
	case onebullex.ExchangeName:
		return onebullex.NewClient(onebullex.WithBaseURL(env("ONEBULLEX_BASE_URL", ""))), nil
	default:
		return nil, fmt.Errorf("unsupported exchange %q", exchangeName)
	}
}

func normalizeExchangeName(exchangeName string) string {
	exchangeName = strings.ToLower(strings.TrimSpace(exchangeName))
	if exchangeName == "onebull" || exchangeName == "1bullex" {
		return onebullex.ExchangeName
	}
	return exchangeName
}

func parseMarketType(value string) (marketdata.MarketType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "perpetual", "futures", "future":
		return marketdata.MarketTypePerpetual, nil
	default:
		return "", fmt.Errorf("unsupported market type %q: current strategy only supports perpetual", value)
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
		return 1500
	}
	if limit > 1500 {
		return 1500
	}
	return limit
}

func isKlineDataset(dataset string) bool {
	switch strings.ToLower(strings.TrimSpace(dataset)) {
	case "klines", "candles":
		return true
	default:
		return false
	}
}

func isFundingDataset(dataset string) bool {
	switch strings.ToLower(strings.TrimSpace(dataset)) {
	case "funding", "funding-rates":
		return true
	default:
		return false
	}
}

func isGlobalDataset(dataset string) bool {
	switch strings.ToLower(strings.TrimSpace(dataset)) {
	case "contract-specs", "symbols", "symbol-specs":
		return true
	default:
		return false
	}
}

func lookbackForInterval(interval string, limit int) time.Duration {
	limit = normalizeLimit(limit)
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return 24 * time.Hour
	}
	return step * time.Duration(limit)
}

func lookbackForFunding(limit int) time.Duration {
	limit = normalizeLimit(limit)
	if limit < 3 {
		limit = 3
	}
	return 8 * time.Hour * time.Duration(limit)
}
