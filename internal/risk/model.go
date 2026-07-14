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
	MaxInitialMarginPct       float64
	MaxAvailableBalanceUsePct float64
	MaxLeverage               float64
	MaxDailyLossPct           float64
	MaxConsecutiveLosses      int
	MinLiquidationDistancePct float64
	MaintenanceMarginRatePct  float64
	MaxAbsFundingRatePct      float64
	MinQuantity               float64
	QuantityStep              float64
	PriceTickSize             float64
}

func DefaultConfig() Config {
	return Config{
		MaxOrderRiskPct:           1,
		MaxSymbolExposurePct:      30,
		MaxTotalExposurePct:       100,
		MaxInitialMarginPct:       35,
		MaxAvailableBalanceUsePct: 80,
		MaxLeverage:               3,
		MaxDailyLossPct:           2,
		MaxConsecutiveLosses:      3,
		MinLiquidationDistancePct: 10,
		MaintenanceMarginRatePct:  0.5,
		MaxAbsFundingRatePct:      0.05,
		MinQuantity:               0,
		QuantityStep:              0,
		PriceTickSize:             0,
	}
}

type AccountSnapshot struct {
	AccountID             string
	Equity                float64
	AvailableBalance      float64
	DailyRealizedPnL      float64
	ConsecutiveLosses     int
	CurrentTotalExposure  float64
	CurrentSymbolExposure float64
	CurrentInitialMargin  float64
	CurrentMaintMargin    float64
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
	Decision           Decision
	Events             []Event
	OrderNotional      float64
	OrderRisk          float64
	OrderInitialMargin float64
	TotalExposure      float64
	SymbolExposure     float64
	TotalInitialMargin float64
	AvailableBalance   float64
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
		Decision:           DecisionAllow,
		OrderNotional:      order.Notional(),
		OrderRisk:          orderRisk(order),
		OrderInitialMargin: orderInitialMargin(order),
		TotalExposure:      account.CurrentTotalExposure,
		SymbolExposure:     account.CurrentSymbolExposure,
		TotalInitialMargin: account.CurrentInitialMargin,
		AvailableBalance:   effectiveAvailableBalance(account),
		Events:             make([]Event, 0),
	}

	if !order.ReduceOnly {
		if account.DailyRealizedPnL < 0 {
			lossPct := math.Abs(account.DailyRealizedPnL) / account.Equity * 100
			if lossPct >= config.MaxDailyLossPct {
				result.add(DecisionHalt, SeverityCritical, "daily_loss_halt", fmt.Sprintf("daily loss %.2f%% reached limit %.2f%%", lossPct, config.MaxDailyLossPct))
			}
		}
		if config.MaxConsecutiveLosses > 0 && account.ConsecutiveLosses >= config.MaxConsecutiveLosses {
			result.add(DecisionHalt, SeverityCritical, "consecutive_loss_halt", fmt.Sprintf("consecutive losses %d reached limit %d", account.ConsecutiveLosses, config.MaxConsecutiveLosses))
		}

		if config.MinQuantity > 0 && order.Quantity+1e-12 < config.MinQuantity {
			result.add(DecisionReject, SeverityCritical, "min_quantity_limit", fmt.Sprintf("quantity %.8f below minimum %.8f", order.Quantity, config.MinQuantity))
		}
		if config.QuantityStep > 0 && !isStepAligned(order.Quantity, config.QuantityStep) {
			result.add(DecisionReject, SeverityCritical, "quantity_step_limit", fmt.Sprintf("quantity %.8f is not aligned to step %.8f", order.Quantity, config.QuantityStep))
		}
		if config.PriceTickSize > 0 && !isStepAligned(order.Price, config.PriceTickSize) {
			result.add(DecisionReject, SeverityCritical, "price_tick_limit", fmt.Sprintf("price %.8f is not aligned to tick %.8f", order.Price, config.PriceTickSize))
		}
		if config.PriceTickSize > 0 && order.StopPrice > 0 && !isStepAligned(order.StopPrice, config.PriceTickSize) {
			result.add(DecisionReject, SeverityCritical, "stop_price_tick_limit", fmt.Sprintf("stop price %.8f is not aligned to tick %.8f", order.StopPrice, config.PriceTickSize))
		}
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

		if order.MarketType == MarketTypeSpot && order.Side == SideBuy && result.OrderNotional > result.AvailableBalance {
			result.add(DecisionReject, SeverityCritical, "available_balance_limit", fmt.Sprintf("order notional %.4f exceeds available balance %.4f", result.OrderNotional, result.AvailableBalance))
		}
	}

	if order.MarketType == MarketTypePerpetual && !order.ReduceOnly {
		if order.Leverage > config.MaxLeverage {
			result.add(DecisionReject, SeverityCritical, "leverage_limit", fmt.Sprintf("leverage %.2fx exceeds limit %.2fx", order.Leverage, config.MaxLeverage))
		}
		result.TotalInitialMargin = account.CurrentInitialMargin + result.OrderInitialMargin
		if result.OrderInitialMargin > result.AvailableBalance {
			result.add(DecisionReject, SeverityCritical, "available_balance_limit", fmt.Sprintf("initial margin %.4f exceeds available balance %.4f", result.OrderInitialMargin, result.AvailableBalance))
		}
		if config.MaxAvailableBalanceUsePct > 0 && result.AvailableBalance > 0 {
			availableUsePct := result.OrderInitialMargin / result.AvailableBalance * 100
			if availableUsePct > config.MaxAvailableBalanceUsePct {
				result.add(DecisionReject, SeverityCritical, "available_balance_use_limit", fmt.Sprintf("order uses %.2f%% of available balance, limit %.2f%%", availableUsePct, config.MaxAvailableBalanceUsePct))
			}
		}
		if marginPct(result.TotalInitialMargin, account.Equity) > config.MaxInitialMarginPct {
			result.add(DecisionReject, SeverityCritical, "initial_margin_limit", fmt.Sprintf("initial margin %.2f%% exceeds limit %.2f%%", marginPct(result.TotalInitialMargin, account.Equity), config.MaxInitialMarginPct))
		}
		liquidationPrice := order.LiquidationPrice
		if liquidationPrice <= 0 {
			liquidationPrice = EstimateLiquidationPrice(order.Price, order.Side, order.Leverage, config.MaintenanceMarginRatePct)
		}
		if liquidationPrice > 0 {
			distance := liquidationDistancePct(order.Price, liquidationPrice)
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
	if config.MaxInitialMarginPct <= 0 {
		return errors.New("max initial margin pct must be positive")
	}
	if config.MaxAvailableBalanceUsePct < 0 {
		return errors.New("max available balance use pct cannot be negative")
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
	if config.MaintenanceMarginRatePct < 0 {
		return errors.New("maintenance margin rate pct cannot be negative")
	}
	if config.MaxAbsFundingRatePct < 0 {
		return errors.New("max abs funding rate pct cannot be negative")
	}
	if config.MinQuantity < 0 {
		return errors.New("min quantity cannot be negative")
	}
	if config.QuantityStep < 0 {
		return errors.New("quantity step cannot be negative")
	}
	if config.PriceTickSize < 0 {
		return errors.New("price tick size cannot be negative")
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
	if account.AvailableBalance < 0 {
		return errors.New("available balance cannot be negative")
	}
	if account.CurrentInitialMargin < 0 {
		return errors.New("current initial margin cannot be negative")
	}
	if account.CurrentMaintMargin < 0 {
		return errors.New("current maintenance margin cannot be negative")
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

func isStepAligned(value float64, step float64) bool {
	if step <= 0 {
		return true
	}
	ratio := value / step
	return math.Abs(ratio-math.Round(ratio)) < 1e-9
}

func orderInitialMargin(order OrderIntent) float64 {
	if order.MarketType != MarketTypePerpetual || order.Leverage <= 0 {
		return 0
	}
	return order.Notional() / order.Leverage
}

func effectiveAvailableBalance(account AccountSnapshot) float64 {
	if account.AvailableBalance > 0 {
		return account.AvailableBalance
	}
	available := account.Equity - account.CurrentInitialMargin
	if available < 0 {
		return 0
	}
	return available
}

func exposurePct(exposure float64, equity float64) float64 {
	return exposure / equity * 100
}

func marginPct(margin float64, equity float64) float64 {
	return margin / equity * 100
}

func riskPct(risk float64, equity float64) float64 {
	return risk / equity * 100
}

func liquidationDistancePct(price float64, liquidationPrice float64) float64 {
	return math.Abs(price-liquidationPrice) / price * 100
}

func EstimateLiquidationPrice(entryPrice float64, side Side, leverage float64, maintenanceMarginRatePct float64) float64 {
	if entryPrice <= 0 || leverage <= 0 {
		return 0
	}
	maintenanceRate := math.Max(maintenanceMarginRatePct, 0) / 100
	marginRate := 1 / leverage
	if side == SideSell {
		liquidation := entryPrice * (1 + marginRate - maintenanceRate)
		if liquidation <= 0 || math.IsNaN(liquidation) || math.IsInf(liquidation, 0) {
			return 0
		}
		return liquidation
	}
	liquidation := entryPrice * (1 - marginRate + maintenanceRate)
	if liquidation <= 0 || math.IsNaN(liquidation) || math.IsInf(liquidation, 0) {
		return 0
	}
	return liquidation
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
