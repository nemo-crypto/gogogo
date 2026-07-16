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
		PositionModel:    "AGGREGATION",
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
	var positionModel string
	if err := db.QueryRowContext(ctx, `SELECT position_model FROM positions WHERE account_id = 'research' AND symbol = 'BTCUSDT';`).Scan(&positionModel); err != nil {
		t.Fatalf("query position model: %v", err)
	}
	if positionModel != "AGGREGATION" {
		t.Fatalf("position model = %q, want AGGREGATION", positionModel)
	}
}

func TestLatestLiveBalanceSnapshotIgnoresPaperRows(t *testing.T) {
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
	liveTime := time.Date(2026, 7, 15, 11, 34, 0, 0, time.UTC)
	if _, err := repo.SaveBalanceSnapshot(ctx, BalanceSnapshot{
		AccountID:        "live-main",
		Exchange:         "onebullex",
		Asset:            "USDT",
		Free:             26.5,
		Total:            26.5,
		AvailableBalance: "26.5",
		WalletBalance:    "26.5",
		SnapshotTime:     liveTime,
	}); err != nil {
		t.Fatalf("save live balance: %v", err)
	}
	if _, err := repo.SaveBalanceSnapshot(ctx, BalanceSnapshot{
		AccountID:    "live-main",
		Exchange:     "onebullex",
		Asset:        "USDT",
		Free:         1000,
		Total:        1000,
		SnapshotTime: liveTime.Add(time.Minute),
	}); err != nil {
		t.Fatalf("save paper balance: %v", err)
	}

	got, err := repo.LatestLiveBalanceSnapshot(ctx, "live-main", "onebullex", "usdt")
	if err != nil {
		t.Fatalf("latest live balance: %v", err)
	}
	if got.Total != 26.5 || got.AvailableBalance != "26.5" {
		t.Fatalf("latest live balance = %+v, want real 26.5 row", got)
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
	if position.InitialStopLoss != 59700 {
		t.Fatalf("initial stop = %.4f, want 59700", position.InitialStopLoss)
	}

	if err := repo.UpdatePaperPositionMark(ctx, id, 60400); err != nil {
		t.Fatalf("update mark: %v", err)
	}
	if err := repo.UpdatePaperPositionStopLoss(ctx, id, 60100); err != nil {
		t.Fatalf("update stop loss: %v", err)
	}
	position, err = repo.LatestOpenPaperPosition(ctx, "paper", "scalp-tpsl-paper", "onebullex", "spot", "BTCUSDT")
	if err != nil {
		t.Fatalf("reload latest open position: %v", err)
	}
	if position.StopLossPrice != 60100 {
		t.Fatalf("stop loss = %.4f, want 60100", position.StopLossPrice)
	}
	if position.InitialStopLoss != 59700 {
		t.Fatalf("initial stop after update = %.4f, want 59700", position.InitialStopLoss)
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

	closed, err := repo.ClosePaperPositionWithRealizedPnLAndReason(ctx, id, 59800, openedAt.Add(time.Minute), 1.25, "take_profit")
	if err != nil {
		t.Fatalf("close paper position with realized pnl: %v", err)
	}
	if closed.Status != PaperPositionClosed {
		t.Fatalf("status = %q, want closed", closed.Status)
	}
	if closed.RealizedPnL != 1.25 {
		t.Fatalf("realized pnl = %.4f, want 1.25", closed.RealizedPnL)
	}
	if closed.CloseReason != "take_profit" {
		t.Fatalf("close reason = %q, want take_profit", closed.CloseReason)
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

func TestSQLiteRepositoryPaperPositionRiskStats(t *testing.T) {
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
	dayStart := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	for i, realizedPnL := range []float64{1.00, -0.75, -1.25} {
		openedAt := dayStart.Add(time.Duration(i) * time.Hour)
		id, err := repo.OpenPaperPosition(ctx, PaperPositionRecord{
			AccountID:    "paper-v8",
			StrategyID:   "perp-trend-scalp-v2-paper",
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
			t.Fatalf("open paper position %d: %v", i, err)
		}
		reason := "take_profit"
		if realizedPnL < 0 {
			reason = "stop_loss"
		}
		if _, err := repo.ClosePaperPositionWithRealizedPnLAndReason(ctx, id, 59900, openedAt.Add(10*time.Minute), realizedPnL, reason); err != nil {
			t.Fatalf("close paper position %d: %v", i, err)
		}
	}

	daily, err := repo.SumClosedPaperPositionRealizedPnLSince(ctx, "paper-v8", "perp-trend-scalp-v2-paper", "onebullex", "perpetual", "BTCUSDT", dayStart)
	if err != nil {
		t.Fatalf("sum daily pnl: %v", err)
	}
	if daily != -1.0 {
		t.Fatalf("daily pnl = %.4f, want -1.0", daily)
	}
	losses, err := repo.CountConsecutiveClosedPaperPositionLosses(ctx, "paper-v8", "perp-trend-scalp-v2-paper", "onebullex", "perpetual", "BTCUSDT")
	if err != nil {
		t.Fatalf("count consecutive losses: %v", err)
	}
	if losses != 2 {
		t.Fatalf("consecutive losses = %d, want 2", losses)
	}
}

func TestSQLiteRepositoryContractMetadata(t *testing.T) {
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
	if _, err := repo.SaveContractSpec(ctx, ContractSpec{
		Exchange:            "onebullex",
		Symbol:              "btcusdt",
		ContractType:        "PERPETUAL",
		UnderlyingType:      "USDT",
		ContractSize:        "1",
		TradeSwitch:         true,
		BaseAsset:           "btc",
		QuoteAsset:          "usdt",
		MinPrice:            "0.1",
		MinQty:              "0.001",
		MinNotional:         "5",
		MaxNotional:         "100000",
		MinStepPrice:        "0.1",
		SupportOrderType:    "LIMIT,MARKET",
		SupportTimeInForce:  "GTC,IOC",
		SupportPositionType: "LONG,SHORT",
		LabelsJSON:          `["perp"]`,
		RawJSON:             `{"symbol":"btc_usdt"}`,
	}); err != nil {
		t.Fatalf("save contract spec: %v", err)
	}
	if _, err := repo.SaveLeverageBracket(ctx, LeverageBracket{
		Exchange:        "onebullex",
		Symbol:          "btcusdt",
		Bracket:         1,
		MaxNominalValue: "100000",
		MaintMarginRate: "0.005",
		StartMarginRate: "0.01",
		MaxLeverage:     "100",
		MinLeverage:     "1",
		RawJSON:         `{"bracket":1}`,
	}); err != nil {
		t.Fatalf("save leverage bracket: %v", err)
	}
	if _, err := repo.SavePositionConfig(ctx, PositionConfig{
		AccountID:     "research",
		Exchange:      "onebullex",
		Symbol:        "btcusdt",
		PositionType:  "crossed",
		PositionSide:  "both",
		PositionModel: "LONG_SHORT",
		AutoMargin:    true,
		Leverage:      10,
		RawJSON:       `{"positionModel":"LONG_SHORT"}`,
		SnapshotTime:  time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save position config: %v", err)
	}

	var minStepPrice string
	if err := db.QueryRowContext(ctx, `SELECT min_step_price FROM contract_specs WHERE symbol = 'BTCUSDT';`).Scan(&minStepPrice); err != nil {
		t.Fatalf("query contract spec: %v", err)
	}
	if minStepPrice != "0.1" {
		t.Fatalf("min step price = %q, want 0.1", minStepPrice)
	}
	var maxLeverage string
	if err := db.QueryRowContext(ctx, `SELECT max_leverage FROM leverage_brackets WHERE symbol = 'BTCUSDT' AND bracket = 1;`).Scan(&maxLeverage); err != nil {
		t.Fatalf("query leverage bracket: %v", err)
	}
	if maxLeverage != "100" {
		t.Fatalf("max leverage = %q, want 100", maxLeverage)
	}
	var positionModel string
	if err := db.QueryRowContext(ctx, `SELECT position_model FROM position_configs WHERE symbol = 'BTCUSDT' AND position_side = 'both';`).Scan(&positionModel); err != nil {
		t.Fatalf("query position config: %v", err)
	}
	if positionModel != "DISAGGREGATION" {
		t.Fatalf("position model = %q, want DISAGGREGATION", positionModel)
	}
}
