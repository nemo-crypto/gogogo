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

	"gogogo/internal/marketdata"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dsn       = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange  = flag.String("exchange", "binance", "exchange name")
		market    = flag.String("market", "spot", "market type: spot or perpetual")
		symbols   = flag.String("symbols", "BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT", "comma-separated symbols")
		interval  = flag.String("interval", "1h", "kline interval")
		start     = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end       = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		name      = flag.String("name", "", "snapshot name; defaults to exchange-market-interval-start-end")
		allowGaps = flag.Bool("allow-gaps", false, "allow missing candles without failing the command")
		maxGaps   = flag.Int("max-gaps", 5, "max gaps to print per symbol")
	)
	flag.Parse()

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, *end)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}
	if !endTime.After(startTime) {
		return errors.New("end must be after start")
	}

	marketType, err := parseMarketType(*market)
	if err != nil {
		return err
	}
	parsedSymbols := parseSymbols(*symbols)
	if len(parsedSymbols) == 0 {
		return errors.New("at least one symbol is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()

	repo := marketdata.NewSQLiteRepository(db)
	snapshotName := strings.TrimSpace(*name)
	if snapshotName == "" {
		snapshotName = defaultSnapshotName(*exchange, string(marketType), *interval, startTime, endTime)
	}

	hasGaps := false
	for _, symbol := range parsedSymbols {
		snapshot, coverage, err := repo.CreateCandleSnapshot(ctx, marketdata.CandleSnapshotRequest{
			Name:            snapshotName,
			RequireComplete: !*allowGaps,
			Query: marketdata.CandleQuery{
				Exchange:   *exchange,
				MarketType: marketType,
				Symbol:     symbol,
				Interval:   *interval,
				Start:      startTime,
				End:        endTime,
			},
		})
		if err != nil {
			if coverage.ExpectedCount > 0 {
				printCoverage(coverage, *maxGaps)
			}
			return fmt.Errorf("create candle snapshot %s: %w", symbol, err)
		}

		printSnapshot(snapshot, coverage, *maxGaps)
		if !coverage.Complete() {
			hasGaps = true
		}
	}

	if hasGaps && !*allowGaps {
		return errors.New("data gaps detected; fill missing candles or rerun with -allow-gaps=true")
	}

	return nil
}

func printCoverage(coverage marketdata.CandleCoverage, maxGaps int) {
	fmt.Printf("market=%s symbol=%s interval=%s range=%s -> %s\n",
		coverage.MarketType,
		coverage.Symbol,
		coverage.Interval,
		coverage.Start.Format(time.RFC3339),
		coverage.End.Format(time.RFC3339),
	)
	fmt.Printf("candles=%d expected=%d missing=%d gaps=%d\n",
		coverage.CandleCount,
		coverage.ExpectedCount,
		coverage.MissingCount,
		len(coverage.Gaps),
	)
	printGaps(coverage, maxGaps)
	fmt.Println()
}

func printSnapshot(snapshot marketdata.CandleSnapshot, coverage marketdata.CandleCoverage, maxGaps int) {
	fmt.Printf("snapshot_id=%d name=%s\n", snapshot.ID, snapshot.Name)
	fmt.Printf("market=%s symbol=%s interval=%s range=%s -> %s\n",
		snapshot.MarketType,
		snapshot.Symbol,
		snapshot.Interval,
		snapshot.Start.Format(time.RFC3339),
		snapshot.End.Format(time.RFC3339),
	)
	fmt.Printf("candles=%d expected=%d missing=%d gaps=%d hash=%s\n",
		snapshot.CandleCount,
		snapshot.ExpectedCount,
		snapshot.MissingCount,
		snapshot.GapCount,
		snapshot.DataHash,
	)
	printGaps(coverage, maxGaps)
	fmt.Println()
}

func printGaps(coverage marketdata.CandleCoverage, maxGaps int) {
	if len(coverage.Gaps) > 0 {
		limit := maxGaps
		if limit <= 0 || limit > len(coverage.Gaps) {
			limit = len(coverage.Gaps)
		}
		for i := 0; i < limit; i++ {
			gap := coverage.Gaps[i]
			fmt.Printf("gap start=%s end=%s missing=%d\n",
				gap.Start.Format(time.RFC3339),
				gap.End.Format(time.RFC3339),
				gap.MissingCount,
			)
		}
	}
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

func defaultSnapshotName(exchange string, market string, interval string, start time.Time, end time.Time) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		strings.ToLower(strings.TrimSpace(exchange)),
		strings.ToLower(strings.TrimSpace(market)),
		strings.TrimSpace(interval),
		start.UTC().Format("20060102T150405Z"),
		end.UTC().Format("20060102T150405Z"),
	)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
