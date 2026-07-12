package backtest

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSQLiteRepositorySaveRun(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if err := InitSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	repo := NewSQLiteRepository(db)
	id, err := repo.SaveRun(ctx, SaveRunRequest{
		Exchange:   "binance",
		MarketType: "spot",
		Config: SMAConfig{
			FastWindow: 2,
			SlowWindow: 3,
			FeeRate:    0.001,
		},
		Result: Result{
			StrategyName:   "sma_crossover_2_3",
			Symbol:         "BTCUSDT",
			Interval:       "1h",
			Start:          time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			End:            time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			InitialEquity:  1,
			FinalEquity:    1.1,
			TotalReturnPct: 10,
			MaxDrawdownPct: 2,
			Trades:         []Trade{{ReturnPct: 10}},
			WinRatePct:     100,
		},
	})
	if err != nil {
		t.Fatalf("save run: %v", err)
	}
	if id == 0 {
		t.Fatal("id = 0, want inserted id")
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backtest_runs;`).Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	records, err := repo.ListRuns(ctx, ListRunsQuery{
		MarketType: "spot",
		Symbol:     "BTCUSDT",
		Interval:   "1h",
		SortBy:     "total",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", records[0].Symbol)
	}
}
