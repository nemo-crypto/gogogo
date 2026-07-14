package trading

import (
	"context"
	"time"

	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/risk"
)

type Mode string

const (
	ModeBacktest Mode = "backtest"
	ModePaper    Mode = "paper"
	ModeLive     Mode = "live"
)

type MarketDataProvider interface {
	Candles(ctx context.Context, query marketdata.CandleQuery) ([]marketdata.Candle, error)
	LatestMarkPrice(ctx context.Context, exchange string, symbol string) (marketdata.MarkPrice, error)
}

type Broker interface {
	SubmitOrder(ctx context.Context, order execution.OrderRecord) (execution.OrderRecord, error)
	CancelOrder(ctx context.Context, accountID string, symbol string, clientOrderID string) (execution.OrderRecord, error)
}

type RiskManager interface {
	EvaluateOrder(ctx context.Context, account risk.AccountSnapshot, order risk.OrderIntent) (risk.Result, error)
}

type Strategy interface {
	ID() string
	GenerateSignal(ctx context.Context, input StrategyInput) (Signal, error)
}

type StrategyInput struct {
	Mode       Mode
	Exchange   string
	MarketType marketdata.MarketType
	Symbol     string
	Interval   string
	Time       time.Time
	Candles    []marketdata.Candle
	Account    risk.AccountSnapshot
}

type Signal struct {
	Action     string
	Side       risk.Side
	Price      float64
	Quantity   float64
	StopPrice  float64
	Confidence float64
	Reason     string
}
