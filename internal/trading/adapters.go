package trading

import (
	"context"

	"gogogo/internal/marketdata"
	"gogogo/internal/risk"
)

type SQLiteMarketDataProvider struct {
	Repository *marketdata.SQLiteRepository
}

func (p SQLiteMarketDataProvider) Candles(ctx context.Context, query marketdata.CandleQuery) ([]marketdata.Candle, error) {
	return p.Repository.ListCandles(ctx, query)
}

func (p SQLiteMarketDataProvider) LatestMarkPrice(ctx context.Context, exchange string, symbol string) (marketdata.MarkPrice, error) {
	return p.Repository.LatestMarkPrice(ctx, exchange, symbol)
}

type StaticRiskManager struct {
	Config risk.Config
}

func (m StaticRiskManager) EvaluateOrder(_ context.Context, account risk.AccountSnapshot, order risk.OrderIntent) (risk.Result, error) {
	config := m.Config
	if config == (risk.Config{}) {
		config = risk.DefaultConfig()
	}
	return risk.EvaluateOrder(config, account, order)
}
