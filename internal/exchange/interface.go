package exchange

import (
	"context"
	"time"

	"gogogo/internal/marketdata"
)

type AccountSnapshot struct {
	AccountID    string
	Exchange     string
	Balances     []Balance
	Positions    []Position
	ServerTime   time.Time
	SnapshotTime time.Time
	ReadOnly     bool
}

type Balance struct {
	Asset     string
	Free      string
	Locked    string
	Total     string
	USDValue  string
	UpdatedAt time.Time
}

type Position struct {
	Symbol           string
	PositionSide     string
	Quantity         string
	EntryPrice       string
	MarkPrice        string
	LiquidationPrice string
	Leverage         int
	MarginMode       string
	UnrealizedPnL    string
	UpdatedAt        time.Time
}

type OrderRequest struct {
	AccountID     string
	MarketType    marketdata.MarketType
	Symbol        string
	ClientOrderID string
	Side          string
	OrderType     string
	TimeInForce   string
	ReduceOnly    bool
	Price         string
	Quantity      string
}

type OrderStatus struct {
	ClientOrderID   string
	ExchangeOrderID string
	Status          string
	ExecutedQty     string
	UpdatedAt       time.Time
}

type Client interface {
	Klines(ctx context.Context, request KlineRequest) ([]marketdata.Candle, error)
	FundingRates(ctx context.Context, request FundingRateRequest) ([]marketdata.FundingRate, error)
	LatestMarkPrice(ctx context.Context, symbol string) (marketdata.MarkPrice, error)
	ServerTime(ctx context.Context, marketType marketdata.MarketType) (time.Time, error)
	AccountSnapshot(ctx context.Context, accountID string) (AccountSnapshot, error)
	SubmitOrder(ctx context.Context, request OrderRequest) (OrderStatus, error)
	CancelOrder(ctx context.Context, accountID string, symbol string, clientOrderID string) (OrderStatus, error)
	OrderStatus(ctx context.Context, accountID string, symbol string, clientOrderID string) (OrderStatus, error)
}

type KlineRequest struct {
	MarketType marketdata.MarketType
	Symbol     string
	Interval   string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int
}

type FundingRateRequest struct {
	Symbol    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}
