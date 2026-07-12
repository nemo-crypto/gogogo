package risk

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type MarketType string

const (
	MarketTypeSpot      MarketType = "spot"
	MarketTypePerpetual MarketType = "perpetual"
)

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionReject Decision = "reject"
	DecisionHalt   Decision = "halt"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Config struct {
	MaxOrderRiskPct           float64
	MaxSymbolExposurePct      float64
	MaxTotalExposurePct       float64
	MaxLeverage               float64
	MaxDailyLossPct           float64
	MaxConsecutiveLosses      int
	MinLiquidationDistancePct float64
	MaxAbsFundingRatePct      float64
}

func DefaultConfig() Config {
	return Config{
		MaxOrderRiskPct:           1,
		MaxSymbolExposurePct:      30,
		MaxTotalExposurePct:       100,
		MaxLeverage:               3,
		MaxDailyLossPct:           2,
		MaxConsecutiveLosses:      3,
		MinLiquidationDistancePct: 10,
		MaxAbsFundingRatePct:      0.05,
	}
}

type AccountSnapshot struct {
	AccountID             string
	Equity                float64
	DailyRealizedPnL      float64
	ConsecutiveLosses     int
	CurrentTotalExposure  float64
	CurrentSymbolExposure float64
	SnapshotTime          time.Time
}

type OrderIntent struct {
	Exchange             string
	MarketType           MarketType
	Symbol               string
	Side                 Side
	Price                float64
	Quantity             float64
	StopPrice            float64
	Leverage             float64
	ReduceOnly           bool
	LiquidationPrice     float64
	LatestFundingRatePct float64
}

func (o OrderIntent) Notional() float64 {
	return o.Price * o.Quantity
}

type Event struct {
	Severity Severity
	Type     string
	Message  string
}

type Result struct {
	Decision       Decision
	Events         []Event
	OrderNotional  float64
	OrderRisk      float64
	TotalExposure  float64
	SymbolExposure float64
}

func EvaluateOrder(config Config, account AccountSnapshot, order OrderIntent) (Result, error) {
	if err := validateConfig(config); err != nil {
		return Result{}, err
	}
	if err := validateAccount(account); err != nil {
		return Result{}, err
	}
	order, err := normalizeOrder(order)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Decision:       DecisionAllow,
		OrderNotional:  order.Notional(),
		OrderRisk:      orderRisk(order),
		TotalExposure:  account.CurrentTotalExposure,
		SymbolExposure: account.CurrentSymbolExposure,
		Events:         make([]Event, 0),
	}

	if account.DailyRealizedPnL < 0 {
		lossPct := math.Abs(account.DailyRealizedPnL) / account.Equity * 100
		if lossPct >= config.MaxDailyLossPct {
			result.add(DecisionHalt, SeverityCritical, "daily_loss_halt", fmt.Sprintf("daily loss %.2f%% reached limit %.2f%%", lossPct, config.MaxDailyLossPct))
		}
	}
	if config.MaxConsecutiveLosses > 0 && account.ConsecutiveLosses >= config.MaxConsecutiveLosses {
		result.add(DecisionHalt, SeverityCritical, "consecutive_loss_halt", fmt.Sprintf("consecutive losses %d reached limit %d", account.ConsecutiveLosses, config.MaxConsecutiveLosses))
	}

	if !order.ReduceOnly {
		result.TotalExposure = account.CurrentTotalExposure + result.OrderNotional
		result.SymbolExposure = account.CurrentSymbolExposure + result.OrderNotional
		if exposurePct(result.SymbolExposure, account.Equity) > config.MaxSymbolExposurePct {
			result.add(DecisionReject, SeverityCritical, "symbol_exposure_limit", fmt.Sprintf("symbol exposure %.2f%% exceeds limit %.2f%%", exposurePct(result.SymbolExposure, account.Equity), config.MaxSymbolExposurePct))
		}
		if exposurePct(result.TotalExposure, account.Equity) > config.MaxTotalExposurePct {
			result.add(DecisionReject, SeverityCritical, "total_exposure_limit", fmt.Sprintf("total exposure %.2f%% exceeds limit %.2f%%", exposurePct(result.TotalExposure, account.Equity), config.MaxTotalExposurePct))
		}
		if riskPct(result.OrderRisk, account.Equity) > config.MaxOrderRiskPct {
			result.add(DecisionReject, SeverityCritical, "order_risk_limit", fmt.Sprintf("order risk %.2f%% exceeds limit %.2f%%", riskPct(result.OrderRisk, account.Equity), config.MaxOrderRiskPct))
		}
	}

	if order.MarketType == MarketTypePerpetual {
		if order.Leverage > config.MaxLeverage {
			result.add(DecisionReject, SeverityCritical, "leverage_limit", fmt.Sprintf("leverage %.2fx exceeds limit %.2fx", order.Leverage, config.MaxLeverage))
		}
		if order.LiquidationPrice > 0 {
			distance := liquidationDistancePct(order)
			if distance < config.MinLiquidationDistancePct {
				result.add(DecisionReject, SeverityCritical, "liquidation_distance_limit", fmt.Sprintf("liquidation distance %.2f%% below limit %.2f%%", distance, config.MinLiquidationDistancePct))
			}
		}
		if math.Abs(order.LatestFundingRatePct) > config.MaxAbsFundingRatePct {
			result.add(DecisionReject, SeverityWarning, "funding_rate_limit", fmt.Sprintf("funding rate %.4f%% exceeds limit %.4f%%", order.LatestFundingRatePct, config.MaxAbsFundingRatePct))
		}
	}

	return result, nil
}

func (r *Result) add(decision Decision, severity Severity, eventType string, message string) {
	r.Events = append(r.Events, Event{
		Severity: severity,
		Type:     eventType,
		Message:  message,
	})
	if decisionRank(decision) > decisionRank(r.Decision) {
		r.Decision = decision
	}
}

func validateConfig(config Config) error {
	if config.MaxOrderRiskPct <= 0 {
		return errors.New("max order risk pct must be positive")
	}
	if config.MaxSymbolExposurePct <= 0 {
		return errors.New("max symbol exposure pct must be positive")
	}
	if config.MaxTotalExposurePct <= 0 {
		return errors.New("max total exposure pct must be positive")
	}
	if config.MaxLeverage <= 0 {
		return errors.New("max leverage must be positive")
	}
	if config.MaxDailyLossPct <= 0 {
		return errors.New("max daily loss pct must be positive")
	}
	if config.MinLiquidationDistancePct < 0 {
		return errors.New("min liquidation distance pct cannot be negative")
	}
	if config.MaxAbsFundingRatePct < 0 {
		return errors.New("max abs funding rate pct cannot be negative")
	}
	return nil
}

func validateAccount(account AccountSnapshot) error {
	if account.Equity <= 0 || math.IsNaN(account.Equity) || math.IsInf(account.Equity, 0) {
		return errors.New("account equity must be positive")
	}
	if account.CurrentTotalExposure < 0 {
		return errors.New("current total exposure cannot be negative")
	}
	if account.CurrentSymbolExposure < 0 {
		return errors.New("current symbol exposure cannot be negative")
	}
	if account.ConsecutiveLosses < 0 {
		return errors.New("consecutive losses cannot be negative")
	}
	return nil
}

func normalizeOrder(order OrderIntent) (OrderIntent, error) {
	order.Exchange = strings.ToLower(strings.TrimSpace(order.Exchange))
	order.Symbol = strings.ToUpper(strings.TrimSpace(order.Symbol))
	order.MarketType = MarketType(strings.ToLower(strings.TrimSpace(string(order.MarketType))))
	order.Side = Side(strings.ToLower(strings.TrimSpace(string(order.Side))))

	if order.Exchange == "" {
		return OrderIntent{}, errors.New("exchange is required")
	}
	if order.Symbol == "" {
		return OrderIntent{}, errors.New("symbol is required")
	}
	if order.MarketType != MarketTypeSpot && order.MarketType != MarketTypePerpetual {
		return OrderIntent{}, fmt.Errorf("unsupported market type %q", order.MarketType)
	}
	if order.Side != SideBuy && order.Side != SideSell {
		return OrderIntent{}, fmt.Errorf("unsupported side %q", order.Side)
	}
	if order.Price <= 0 || math.IsNaN(order.Price) || math.IsInf(order.Price, 0) {
		return OrderIntent{}, errors.New("price must be positive")
	}
	if order.Quantity <= 0 || math.IsNaN(order.Quantity) || math.IsInf(order.Quantity, 0) {
		return OrderIntent{}, errors.New("quantity must be positive")
	}
	if order.StopPrice < 0 {
		return OrderIntent{}, errors.New("stop price cannot be negative")
	}
	if order.MarketType == MarketTypePerpetual && order.Leverage <= 0 {
		return OrderIntent{}, errors.New("perpetual leverage must be positive")
	}
	if order.MarketType == MarketTypeSpot && order.Leverage == 0 {
		order.Leverage = 1
	}

	return order, nil
}

func orderRisk(order OrderIntent) float64 {
	if order.StopPrice <= 0 {
		return 0
	}
	return math.Abs(order.Price-order.StopPrice) * order.Quantity
}

func exposurePct(exposure float64, equity float64) float64 {
	return exposure / equity * 100
}

func riskPct(risk float64, equity float64) float64 {
	return risk / equity * 100
}

func liquidationDistancePct(order OrderIntent) float64 {
	return math.Abs(order.Price-order.LiquidationPrice) / order.Price * 100
}

func decisionRank(decision Decision) int {
	switch decision {
	case DecisionAllow:
		return 0
	case DecisionReject:
		return 1
	case DecisionHalt:
		return 2
	default:
		return -1
	}
}
