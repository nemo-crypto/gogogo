package strategy

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteRepositoryRecordsStrategyArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if err := InitSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	repo := NewSQLiteRepository(db)

	runID, err := repo.StartRun(ctx, RunRecord{StrategyID: "sma", Mode: "paper"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if runID <= 0 {
		t.Fatalf("run id = %d, want positive", runID)
	}
	if _, err := repo.SaveSignal(ctx, SignalRecord{
		StrategyID: "sma",
		RunID:      runID,
		Exchange:   "binance",
		MarketType: "spot",
		Symbol:     "btcusdt",
		Action:     SignalBuy,
		Confidence: 0.7,
	}); err != nil {
		t.Fatalf("save signal: %v", err)
	}
	if _, err := repo.SavePerformanceSnapshot(ctx, PerformanceSnapshot{
		StrategyID: "sma",
		RunID:      runID,
		Equity:     1000,
	}); err != nil {
		t.Fatalf("save performance: %v", err)
	}
}
