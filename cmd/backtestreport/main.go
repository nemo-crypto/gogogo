package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
)

func main() {
	var (
		dsn      = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		market   = flag.String("market", "", "optional market filter: spot or perpetual")
		symbol   = flag.String("symbol", "", "optional symbol filter")
		interval = flag.String("interval", "1h", "optional interval filter")
		sortBy   = flag.String("sort", "excess", "sort: excess, total, drawdown, win-rate, trades")
		limit    = flag.Int("limit", 20, "max rows")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()
	if err := backtest.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init backtest schema: %v", err)
	}

	repo := backtest.NewSQLiteRepository(db)
	records, err := repo.ListRuns(ctx, backtest.ListRunsQuery{
		MarketType: *market,
		Symbol:     *symbol,
		Interval:   *interval,
		SortBy:     *sortBy,
		Limit:      *limit,
	})
	if err != nil {
		log.Fatalf("list runs: %v", err)
	}

	printRecords(records)
}

func printRecords(records []backtest.RunRecord) {
	fmt.Printf("%-5s %-18s %-8s %-8s %10s %10s %10s %10s %8s %8s\n",
		"id", "strategy", "symbol", "interval", "return%", "buyhold%", "excess%", "dd%", "trades", "win%")
	for _, record := range records {
		fmt.Printf("%-5d %-18s %-8s %-8s %10.2f %10.2f %10.2f %10.2f %8d %8.2f\n",
			record.ID,
			record.StrategyName,
			record.Symbol,
			record.Interval,
			record.TotalReturnPct,
			record.BuyHoldReturnPct,
			record.ExcessReturnPct,
			record.MaxDrawdownPct,
			record.TradeCount,
			record.WinRatePct,
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
