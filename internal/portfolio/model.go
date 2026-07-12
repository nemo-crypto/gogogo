package portfolio

import "time"

type BalanceSnapshot struct {
	ID           int64
	AccountID    string
	Exchange     string
	Asset        string
	Free         float64
	Locked       float64
	Total        float64
	USDValue     float64
	SnapshotTime time.Time
	CreatedAt    time.Time
}

type PositionSnapshot struct {
	ID                  int64
	AccountID           string
	Exchange            string
	MarketType          string
	Symbol              string
	PositionSide        string
	Quantity            float64
	EntryPrice          float64
	MarkPrice           float64
	LiquidationPrice    float64
	Leverage            float64
	MarginMode          string
	UnrealizedPnL       float64
	Notional            float64
	LiquidationDistance float64
	SnapshotTime        time.Time
	CreatedAt           time.Time
}

type MarginSnapshot struct {
	ID                int64
	AccountID         string
	Exchange          string
	MarketType        string
	Equity            float64
	MarginBalance     float64
	InitialMargin     float64
	MaintenanceMargin float64
	MarginRatio       float64
	AvailableBalance  float64
	SnapshotTime      time.Time
	CreatedAt         time.Time
}

type ContractSpec struct {
	ID           int64
	Exchange     string
	Symbol       string
	ContractType string
	BaseAsset    string
	QuoteAsset   string
	TickSize     string
	StepSize     string
	MinQty       string
	MinNotional  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
