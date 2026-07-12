package strategy

import "time"

type RunStatus string

const (
	RunStatusStarted  RunStatus = "started"
	RunStatusFinished RunStatus = "finished"
	RunStatusFailed   RunStatus = "failed"
)

type SignalAction string

const (
	SignalBuy   SignalAction = "buy"
	SignalSell  SignalAction = "sell"
	SignalShort SignalAction = "short"
	SignalCover SignalAction = "cover"
	SignalHold  SignalAction = "hold"
)

type RunRecord struct {
	ID          int64
	StrategyID  string
	Mode        string
	Status      RunStatus
	StartedAt   time.Time
	FinishedAt  time.Time
	ConfigJSON  string
	SummaryJSON string
	CreatedAt   time.Time
}

type SignalRecord struct {
	ID              int64
	StrategyID      string
	RunID           int64
	Exchange        string
	MarketType      string
	Symbol          string
	SignalTime      time.Time
	Action          SignalAction
	Confidence      float64
	Reason          string
	RawFeaturesJSON string
	CreatedAt       time.Time
}

type PerformanceSnapshot struct {
	ID           int64
	StrategyID   string
	RunID        int64
	SnapshotTime time.Time
	Equity       float64
	PnL          float64
	DrawdownPct  float64
	Exposure     float64
	MetricsJSON  string
	CreatedAt    time.Time
}
