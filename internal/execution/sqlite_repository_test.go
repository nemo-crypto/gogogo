package execution

import (
	"context"
	"database/sql"
	"testing"

	"gogogo/internal/risk"
)

func TestSQLiteRepositoryRecordDryRunOrderAllowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)

	result, err := repo.RecordDryRunOrder(ctx, DryRunRequest{
		Account: risk.AccountSnapshot{
			AccountID:             "research",
			Equity:                10_000,
			CurrentTotalExposure:  2_000,
			CurrentSymbolExposure: 1_000,
		},
		Order: risk.OrderIntent{
			Exchange:   "binance",
			MarketType: risk.MarketTypeSpot,
			Symbol:     "btcusdt",
			Side:       risk.SideBuy,
			Price:      60_000,
			Quantity:   0.02,
			StopPrice:  58_000,
		},
		ClientOrderID: "dryrun-test-allow",
		StrategyID:    "unit",
	})
	if err != nil {
		t.Fatalf("record dry-run order: %v", err)
	}

	if result.Order.ID <= 0 {
		t.Fatalf("order id = %d, want positive", result.Order.ID)
	}
	if result.Order.Status != OrderStatusDryRunAccepted {
		t.Fatalf("status = %s, want accepted", result.Order.Status)
	}
	if result.Order.RiskDecision != risk.DecisionAllow {
		t.Fatalf("risk decision = %s, want allow", result.Order.RiskDecision)
	}
	if len(result.Events) != 0 {
		t.Fatalf("events length = %d, want 0", len(result.Events))
	}
}

func TestSQLiteRepositoryRecordDryRunOrderRejectedAndIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)
	request := DryRunRequest{
		Account: risk.AccountSnapshot{
			AccountID: "research",
			Equity:    10_000,
		},
		Order: risk.OrderIntent{
			Exchange:             "binance",
			MarketType:           risk.MarketTypePerpetual,
			Symbol:               "solusdt",
			Side:                 risk.SideBuy,
			Price:                150,
			Quantity:             10,
			StopPrice:            148,
			Leverage:             5,
			LiquidationPrice:     140,
			LatestFundingRatePct: 0.08,
		},
		ClientOrderID: "dryrun-test-reject",
		StrategyID:    "unit",
	}

	first, err := repo.RecordDryRunOrder(ctx, request)
	if err != nil {
		t.Fatalf("record first dry-run order: %v", err)
	}
	second, err := repo.RecordDryRunOrder(ctx, request)
	if err != nil {
		t.Fatalf("record second dry-run order: %v", err)
	}

	if first.Order.ID != second.Order.ID {
		t.Fatalf("order ids = %d and %d, want same id", first.Order.ID, second.Order.ID)
	}
	if first.Order.Status != OrderStatusRiskRejected {
		t.Fatalf("status = %s, want rejected", first.Order.Status)
	}
	if first.Order.RiskDecision != risk.DecisionReject {
		t.Fatalf("risk decision = %s, want reject", first.Order.RiskDecision)
	}
	if len(first.Events) != 3 {
		t.Fatalf("events length = %d, want 3", len(first.Events))
	}
	if len(second.Events) != 3 {
		t.Fatalf("second events length = %d, want 3 existing events", len(second.Events))
	}
}

func newTestRepository(t *testing.T, ctx context.Context) *SQLiteRepository {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	db.SetMaxOpenConns(1)

	if err := InitSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	return NewSQLiteRepository(db)
}
