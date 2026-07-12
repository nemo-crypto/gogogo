package portfolio

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSQLiteRepositorySnapshots(t *testing.T) {
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
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	if _, err := repo.SaveBalanceSnapshot(ctx, BalanceSnapshot{
		AccountID:    "research",
		Exchange:     "Binance",
		Asset:        "usdt",
		Free:         900,
		Locked:       100,
		SnapshotTime: now,
	}); err != nil {
		t.Fatalf("save balance: %v", err)
	}
	if _, err := repo.SavePositionSnapshot(ctx, PositionSnapshot{
		AccountID:        "research",
		Exchange:         "binance",
		MarketType:       "perpetual",
		Symbol:           "btcusdt",
		PositionSide:     "long",
		Quantity:         0.01,
		EntryPrice:       60000,
		MarkPrice:        61000,
		LiquidationPrice: 50000,
		Leverage:         2,
		MarginMode:       "isolated",
		SnapshotTime:     now,
	}); err != nil {
		t.Fatalf("save position: %v", err)
	}
	if _, err := repo.SaveMarginSnapshot(ctx, MarginSnapshot{
		AccountID:        "research",
		Exchange:         "binance",
		MarketType:       "perpetual",
		Equity:           1000,
		MarginBalance:    1000,
		AvailableBalance: 800,
		SnapshotTime:     now,
	}); err != nil {
		t.Fatalf("save margin: %v", err)
	}
}
