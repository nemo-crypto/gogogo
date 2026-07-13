package execution

import (
	"time"

	"gogogo/internal/risk"
)

type OrderStatus string

const (
	OrderStatusDryRunAccepted OrderStatus = "dry_run_accepted"
	OrderStatusRiskRejected   OrderStatus = "risk_rejected"
	OrderStatusRiskHalted     OrderStatus = "risk_halted"
	OrderStatusSubmitted      OrderStatus = "submitted"
	OrderStatusFilled         OrderStatus = "filled"
	OrderStatusCanceled       OrderStatus = "canceled"
	OrderStatusFailed         OrderStatus = "failed"
)

type DryRunRequest struct {
	Account         risk.AccountSnapshot
	Order           risk.OrderIntent
	RiskConfig      risk.Config
	StrategyID      string
	ClientOrderID   string
	OrderType       string
	TimeInForce     string
	TakeProfitPrice float64
}

type OrderRecord struct {
	ID              int64
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      risk.MarketType
	Symbol          string
	ClientOrderID   string
	ExchangeOrderID string
	ExchangeStatus  string
	Side            risk.Side
	OrderType       string
	TimeInForce     string
	ReduceOnly      bool
	Price           float64
	Quantity        float64
	StopPrice       float64
	TakeProfitPrice float64
	Leverage        float64
	Status          OrderStatus
	RiskDecision    risk.Decision
	RiskReason      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RiskEventRecord struct {
	ID            int64
	AccountID     string
	StrategyID    string
	OrderID       int64
	ClientOrderID string
	EventTime     time.Time
	Severity      risk.Severity
	EventType     string
	Symbol        string
	Decision      risk.Decision
	Message       string
	ContextJSON   string
	CreatedAt     time.Time
}

type DryRunResult struct {
	Order      OrderRecord
	RiskResult risk.Result
	Events     []RiskEventRecord
}
