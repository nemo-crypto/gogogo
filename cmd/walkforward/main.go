package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
)

func main() {
	var (
		dsn         = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange    = flag.String("exchange", "binance", "exchange name")
		marketType  = flag.String("market", "spot", "market type: spot or perpetual")
		symbols     = flag.String("symbol", "BTCUSDT", "symbol or comma-separated symbols")
		interval    = flag.String("interval", "1h", "kline interval")
		start       = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end         = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		fast        = flag.String("fast", "6,12,24", "fast SMA windows")
		slow        = flag.String("slow", "24,48,96", "slow SMA windows")
		feeRate     = flag.Float64("fee-rate", 0.001, "fee rate per entry/exit")
		trainWindow = flag.Int("train-window", 240, "training window in candles")
		testWindow  = flag.Int("test-window", 120, "test window in candles")
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

	parsedMarketType, err := parseMarketType(*marketType)
	if err != nil {
		log.Fatal(err)
	}
	parsedSymbols := parseSymbols(*symbols)
	if len(parsedSymbols) == 0 {
		log.Fatal("at least one symbol is required")
	}
	fastWindows, err := parseInts(*fast)
	if err != nil {
		log.Fatalf("parse fast: %v", err)
	}
	slowWindows, err := parseInts(*slow)
	if err != nil {
		log.Fatalf("parse slow: %v", err)
	}
	configs := buildConfigs(fastWindows, slowWindows, *feeRate)
	if len(configs) == 0 {
		log.Fatal("no valid SMA configs; fast windows must be less than slow windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	repo := marketdata.NewSQLiteRepository(db)
	fmt.Printf("%-8s %-5s %-12s %-12s %-10s %-10s\n", "symbol", "steps", "avg_test%", "avg_excess%", "win_steps%", "last_cfg")
	for _, symbol := range parsedSymbols {
		candles, err := repo.ListCandles(ctx, marketdata.CandleQuery{
			Exchange:   *exchange,
			MarketType: parsedMarketType,
			Symbol:     symbol,
			Interval:   *interval,
			Start:      startTime,
			End:        endTime,
			Limit:      10000,
		})
		if err != nil {
			log.Fatalf("list candles %s: %v", symbol, err)
		}

		result, err := backtest.RunWalkForward(candles, backtest.WalkForwardConfig{
			TrainWindow: *trainWindow,
			TestWindow:  *testWindow,
			Configs:     configs,
		})
		if err != nil {
			log.Printf("skip %s: %v", symbol, err)
			continue
		}

		last := result.Steps[len(result.Steps)-1].Config
		fmt.Printf("%-8s %-5d %-12.2f %-12.2f %-10.2f %d/%d\n",
			result.Symbol,
			len(result.Steps),
			result.AverageTestReturn,
			result.AverageExcessReturn,
			result.WinningStepsPct,
			last.FastWindow,
			last.SlowWindow,
		)
	}
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

func parseInts(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	values := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var parsed int
		if _, err := fmt.Sscanf(part, "%d", &parsed); err != nil {
			return nil, fmt.Errorf("invalid integer %q", part)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("window must be positive: %d", parsed)
		}
		if _, ok := seen[parsed]; ok {
			continue
		}
		seen[parsed] = struct{}{}
		values = append(values, parsed)
	}
	return values, nil
}

func buildConfigs(fastWindows []int, slowWindows []int, feeRate float64) []backtest.SMAConfig {
	configs := make([]backtest.SMAConfig, 0, len(fastWindows)*len(slowWindows))
	for _, fast := range fastWindows {
		for _, slow := range slowWindows {
			if fast >= slow {
				continue
			}
			configs = append(configs, backtest.SMAConfig{
				FastWindow: fast,
				SlowWindow: slow,
				FeeRate:    feeRate,
			})
		}
	}
	return configs
}
