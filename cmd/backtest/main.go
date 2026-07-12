package main

import (
	"context"
	"errors"
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
		dsn        = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange   = flag.String("exchange", "binance", "exchange name")
		marketType = flag.String("market", "spot", "market type: spot or perpetual")
		symbols    = flag.String("symbol", "BTCUSDT", "symbol or comma-separated symbols")
		interval   = flag.String("interval", "1h", "kline interval")
		start      = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end        = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		fast       = flag.String("fast", "3", "fast SMA window or comma-separated windows")
		slow       = flag.String("slow", "6", "slow SMA window or comma-separated windows")
		feeRate    = flag.Float64("fee-rate", 0.001, "fee rate per entry/exit, e.g. 0.001 = 0.1%")
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
	if err := backtest.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init backtest schema: %v", err)
	}

	repo := marketdata.NewSQLiteRepository(db)
	backtestRepo := backtest.NewSQLiteRepository(db)

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

		for _, config := range configs {
			result, err := backtest.RunSMACrossover(candles, config)
			if err != nil {
				if errors.Is(err, backtest.ErrNotEnoughData) {
					log.Printf("skip %s fast=%d slow=%d: not enough candles got=%d", symbol, config.FastWindow, config.SlowWindow, len(candles))
					continue
				}
				log.Fatalf("run backtest %s fast=%d slow=%d: %v", symbol, config.FastWindow, config.SlowWindow, err)
			}

			runID, err := backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
				Exchange:   *exchange,
				MarketType: string(parsedMarketType),
				Config:     config,
				Result:     result,
			})
			if err != nil {
				log.Fatalf("save backtest run %s fast=%d slow=%d: %v", symbol, config.FastWindow, config.SlowWindow, err)
			}

			printResult(runID, result)
		}
	}
}

func printResult(runID int64, result backtest.Result) {
	fmt.Printf("run_id=%d\n", runID)
	fmt.Printf("strategy=%s\n", result.StrategyName)
	fmt.Printf("symbol=%s interval=%s\n", result.Symbol, result.Interval)
	fmt.Printf("range=%s -> %s\n", result.Start.Format(time.RFC3339), result.End.Format(time.RFC3339))
	fmt.Printf("initial_equity=%.4f final_equity=%.4f\n", result.InitialEquity, result.FinalEquity)
	fmt.Printf("total_return_pct=%.4f\n", result.TotalReturnPct)
	fmt.Printf("max_drawdown_pct=%.4f\n", result.MaxDrawdownPct)
	fmt.Printf("trades=%d win_rate_pct=%.2f\n", len(result.Trades), result.WinRatePct)
	fmt.Println()
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
