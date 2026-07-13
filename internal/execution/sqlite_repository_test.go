package execution

import (
	"context"
	"database/sql"
	"testing"
	"time"

	exchangemodel "gogogo/internal/exchange"
	"gogogo/internal/marketdata"
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
			Exchange:   "onebullex",
			MarketType: risk.MarketTypeSpot,
			Symbol:     "btcusdt",
			Side:       risk.SideBuy,
			Price:      60_000,
			Quantity:   0.02,
			StopPrice:  58_000,
		},
		ClientOrderID:   "dryrun-test-allow",
		StrategyID:      "unit",
		TakeProfitPrice: 62_000,
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
	if result.Order.TakeProfitPrice != 62_000 {
		t.Fatalf("take profit price = %f, want 62000", result.Order.TakeProfitPrice)
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
			Exchange:             "onebullex",
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

func TestSQLiteRepositorySubmitOrderToExchangeIncludesNativeTPSL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestRepository(t, ctx)
	result, err := repo.RecordDryRunOrder(ctx, DryRunRequest{
		Account: risk.AccountSnapshot{
			AccountID: "paper",
			Equity:    10_000,
		},
		Order: risk.OrderIntent{
			Exchange:   "onebullex",
			MarketType: risk.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Side:       risk.SideBuy,
			Price:      60_000,
			Quantity:   0.01,
			StopPrice:  59_700,
			Leverage:   1,
		},
		ClientOrderID:   "native-tpsl-test",
		StrategyID:      "unit",
		OrderType:       "market",
		TimeInForce:     "IOC",
		TakeProfitPrice: 60_900,
	})
	if err != nil {
		t.Fatalf("record dry-run order: %v", err)
	}

	client := &captureExchangeClient{}
	_, err = repo.SubmitOrderToExchange(ctx, client, result.Order)
	if err != nil {
		t.Fatalf("submit order: %v", err)
	}
	if client.request.TriggerProfitPrice != "60900" {
		t.Fatalf("trigger profit price = %q, want 60900", client.request.TriggerProfitPrice)
	}
	if client.request.TriggerStopPrice != "59700" {
		t.Fatalf("trigger stop price = %q, want 59700", client.request.TriggerStopPrice)
	}
	if client.request.ProfitOrderType != "MARKET" || client.request.StopOrderType != "MARKET" {
		t.Fatalf("protection order types = %q/%q, want MARKET/MARKET", client.request.ProfitOrderType, client.request.StopOrderType)
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

type captureExchangeClient struct {
	request exchangemodel.OrderRequest
}

func (c *captureExchangeClient) Klines(context.Context, exchangemodel.KlineRequest) ([]marketdata.Candle, error) {
	return nil, nil
}

func (c *captureExchangeClient) FundingRates(context.Context, exchangemodel.FundingRateRequest) ([]marketdata.FundingRate, error) {
	return nil, nil
}

func (c *captureExchangeClient) LatestMarkPrice(context.Context, string) (marketdata.MarkPrice, error) {
	return marketdata.MarkPrice{}, nil
}

func (c *captureExchangeClient) ServerTime(context.Context, marketdata.MarketType) (time.Time, error) {
	return time.Time{}, nil
}

func (c *captureExchangeClient) AccountSnapshot(context.Context, string) (exchangemodel.AccountSnapshot, error) {
	return exchangemodel.AccountSnapshot{}, nil
}

func (c *captureExchangeClient) SubmitOrder(_ context.Context, request exchangemodel.OrderRequest) (exchangemodel.OrderStatus, error) {
	c.request = request
	return exchangemodel.OrderStatus{
		ClientOrderID:   request.ClientOrderID,
		ExchangeOrderID: "exchange-1",
		Status:          "SUBMITTED",
		UpdatedAt:       time.Now().UTC(),
	}, nil
}

func (c *captureExchangeClient) CancelOrder(context.Context, string, string, string) (exchangemodel.OrderStatus, error) {
	return exchangemodel.OrderStatus{}, nil
}

func (c *captureExchangeClient) OrderStatus(context.Context, string, string, string) (exchangemodel.OrderStatus, error) {
	return exchangemodel.OrderStatus{}, nil
}
