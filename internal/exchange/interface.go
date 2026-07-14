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
	Asset                 string
	Free                  string
	Locked                string
	Total                 string
	USDValue              string
	WalletBalance         string
	OpenOrderMarginFrozen string
	IsolatedMargin        string
	CrossedMargin         string
	AvailableBalance      string
	Bonus                 string
	RawJSON               string
	UpdatedAt             time.Time
}

type Position struct {
	Symbol           string
	PositionID       string
	PositionSide     string
	PositionModel    string
	Quantity         string
	CloseOrderSize   string
	AvailableClose   string
	EntryPrice       string
	MarkPrice        string
	LiquidationPrice string
	Leverage         int
	MarginMode       string
	IsolatedMargin   string
	OrderMargin      string
	RealizedProfit   string
	AutoMargin       bool
	ContractSize     string
	UnrealizedPnL    string
	RawJSON          string
	UpdatedAt        time.Time
}

type OrderRequest struct {
	AccountID          string
	MarketType         marketdata.MarketType
	Symbol             string
	ClientOrderID      string
	Side               string
	OrderType          string
	PositionSide       string
	PositionModel      string
	PositionID         string
	TimeInForce        string
	ReduceOnly         bool
	Price              string
	Quantity           string
	Leverage           int
	TriggerProfitPrice string
	TriggerStopPrice   string
	ProfitOrderType    string
	StopOrderType      string
	ProfitOrderPrice   string
	StopOrderPrice     string
	MarketOrderLevel   int
}

type OrderStatus struct {
	ClientOrderID      string
	ExchangeOrderID    string
	PositionID         string
	Symbol             string
	OrderType          string
	OrderSide          string
	PositionSide       string
	TimeInForce        string
	Price              string
	OrigQty            string
	AvgPrice           string
	ExecutedQty        string
	MarginFrozen       string
	TriggerProfitPrice string
	TriggerStopPrice   string
	SourceID           string
	ForceClose         bool
	CloseProfit        string
	Status             string
	RawJSON            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
