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
		Exchange:         "onebullex",
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
		Exchange:         "onebullex",
		MarketType:       "perpetual",
		Equity:           1000,
		MarginBalance:    1000,
		AvailableBalance: 800,
		SnapshotTime:     now,
	}); err != nil {
		t.Fatalf("save margin: %v", err)
	}
}

func TestSQLiteRepositoryPositionSnapshotsKeepMarketTypesSeparate(t *testing.T) {
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
	now := time.Date(2026, 7, 13, 0, 45, 0, 0, time.UTC)
	for _, snapshot := range []PositionSnapshot{
		{
			AccountID:    "paper",
			Exchange:     "onebullex",
			MarketType:   "spot",
			Symbol:       "btcusdt",
			PositionSide: "short",
			Quantity:     0,
			MarkPrice:    63690,
			MarginMode:   "paper",
			SnapshotTime: now,
		},
		{
			AccountID:     "paper",
			Exchange:      "onebullex",
			MarketType:    "perpetual",
			Symbol:        "btcusdt",
			PositionSide:  "short",
			Quantity:      0.01,
			EntryPrice:    63713.8,
			MarkPrice:     63664.3,
			UnrealizedPnL: 0.495,
			MarginMode:    "paper",
			SnapshotTime:  now,
		},
	} {
		if _, err := repo.SavePositionSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("save %s position snapshot: %v", snapshot.MarketType, err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM positions
WHERE account_id = 'paper' AND exchange = 'onebullex' AND symbol = 'BTCUSDT'
	AND position_side = 'short' AND snapshot_time = ?;
`, now).Scan(&count); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	if count != 2 {
		t.Fatalf("position snapshot count = %d, want 2", count)
	}
}

func TestSQLiteRepositoryPaperPositionLifecycle(t *testing.T) {
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
	openedAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	id, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
		AccountID:       "paper",
		StrategyID:      "scalp-tpsl-paper",
		Exchange:        "onebullex",
		MarketType:      "spot",
		Symbol:          "btcusdt",
		PositionSide:    "long",
		Quantity:        0.01,
		EntryPrice:      60000,
		MarkPrice:       60000,
		TakeProfitPrice: 60600,
		StopLossPrice:   59700,
		OpenedAt:        openedAt,
	})
	if err != nil {
		t.Fatalf("open paper position: %v", err)
	}
	if id == 0 {
		t.Fatal("id = 0, want inserted id")
	}

	position, err := repo.LatestOpenPaperPosition(ctx, "paper", "scalp-tpsl-paper", "onebullex", "spot", "BTCUSDT")
	if err != nil {
		t.Fatalf("latest open position: %v", err)
	}
	if position.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", position.Symbol)
	}

	if err := repo.UpdatePaperPositionMark(ctx, id, 60400); err != nil {
		t.Fatalf("update mark: %v", err)
	}
	closed, err := repo.ClosePaperPosition(ctx, id, 60600, openedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("close paper position: %v", err)
	}
	if closed.Status != PaperPositionClosed {
		t.Fatalf("status = %q, want closed", closed.Status)
	}
	if closed.RealizedPnL != 6 {
		t.Fatalf("realized pnl = %.4f, want 6", closed.RealizedPnL)
	}
}

func TestSQLiteRepositoryClosePaperPositionWithRealizedPnL(t *testing.T) {
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
	openedAt := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	id, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
		AccountID:       "paper",
		StrategyID:      "scalp-tpsl-perp-paper",
		Exchange:        "onebullex",
		MarketType:      "perpetual",
		Symbol:          "BTCUSDT",
		PositionSide:    "short",
		Quantity:        0.01,
		EntryPrice:      60000,
		MarkPrice:       60000,
		TakeProfitPrice: 59800,
		StopLossPrice:   60120,
		OpenedAt:        openedAt,
	})
	if err != nil {
		t.Fatalf("open paper position: %v", err)
	}

	closed, err := repo.ClosePaperPositionWithRealizedPnL(ctx, id, 59800, openedAt.Add(time.Minute), 1.25)
	if err != nil {
		t.Fatalf("close paper position with realized pnl: %v", err)
	}
	if closed.Status != PaperPositionClosed {
		t.Fatalf("status = %q, want closed", closed.Status)
	}
	if closed.RealizedPnL != 1.25 {
		t.Fatalf("realized pnl = %.4f, want 1.25", closed.RealizedPnL)
	}
}

func TestSQLiteRepositorySumClosedPaperPositionRealizedPnL(t *testing.T) {
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
	openedAt := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	firstID, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
		AccountID:    "paper-v2",
		StrategyID:   "scalp-tpsl-perp-v2-paper",
		Exchange:     "onebullex",
		MarketType:   "perpetual",
		Symbol:       "BTCUSDT",
		PositionSide: "long",
		Quantity:     0.01,
		EntryPrice:   60000,
		MarkPrice:    60000,
		OpenedAt:     openedAt,
	})
	if err != nil {
		t.Fatalf("open first paper position: %v", err)
	}
	if _, err := repo.ClosePaperPositionWithRealizedPnL(ctx, firstID, 60100, openedAt.Add(time.Minute), 0.75); err != nil {
		t.Fatalf("close first paper position: %v", err)
	}
	secondID, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
		AccountID:    "paper-v2",
		StrategyID:   "scalp-tpsl-perp-v2-paper",
		Exchange:     "onebullex",
		MarketType:   "perpetual",
		Symbol:       "BTCUSDT",
		PositionSide: "short",
		Quantity:     0.01,
		EntryPrice:   60200,
		MarkPrice:    60200,
		OpenedAt:     openedAt.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("open second paper position: %v", err)
	}
	if _, err := repo.ClosePaperPositionWithRealizedPnL(ctx, secondID, 60300, openedAt.Add(3*time.Minute), -1.25); err != nil {
		t.Fatalf("close second paper position: %v", err)
	}
	if _, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
		AccountID:    "paper-v2",
		StrategyID:   "scalp-tpsl-perp-v2-paper",
		Exchange:     "onebullex",
		MarketType:   "perpetual",
		Symbol:       "BTCUSDT",
		PositionSide: "long",
		Quantity:     0.01,
		EntryPrice:   60400,
		MarkPrice:    60400,
		OpenedAt:     openedAt.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("open live paper position: %v", err)
	}

	total, err := repo.SumClosedPaperPositionRealizedPnL(ctx, "paper-v2", "scalp-tpsl-perp-v2-paper", "onebullex", "perpetual", "BTCUSDT")
	if err != nil {
		t.Fatalf("sum realized pnl: %v", err)
	}
	if total != -0.5 {
		t.Fatalf("total realized pnl = %.4f, want -0.5", total)
	}
}
