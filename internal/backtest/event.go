package backtest

import "time"

type EventType string

const (
	EventMarket EventType = "market"
	EventSignal EventType = "signal"
	EventOrder  EventType = "order"
	EventFill   EventType = "fill"
)

type Event struct {
	Type      EventType
	Time      time.Time
	Symbol    string
	Payload   any
	CreatedAt time.Time
}

type CostModel struct {
	FeeRate           float64
	SlippageRate      float64
	MinNotional       float64
	FailureRate       float64
	FundingRate       float64
	LiquidationBuffer float64
}

func (m CostModel) EntryCost(notional float64) float64 {
	return notional * (m.FeeRate + m.SlippageRate)
}

func (m CostModel) ExitCost(notional float64) float64 {
	return notional * (m.FeeRate + m.SlippageRate)
}

func (m CostModel) FundingCost(notional float64, periods float64) float64 {
	return notional * m.FundingRate * periods
}

func (m CostModel) MeetsMinimum(notional float64) bool {
	return notional >= m.MinNotional
}
