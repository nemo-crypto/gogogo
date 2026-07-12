package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"gogogo/internal/risk"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		market               = flag.String("market", "spot", "market type: spot or perpetual")
		symbol               = flag.String("symbol", "BTCUSDT", "symbol")
		side                 = flag.String("side", "buy", "side: buy or sell")
		price                = flag.Float64("price", 0, "planned order price")
		quantity             = flag.Float64("quantity", 0, "planned order quantity")
		stopPrice            = flag.Float64("stop-price", 0, "planned stop price; 0 disables single-order risk check")
		leverage             = flag.Float64("leverage", 1, "planned leverage, required for perpetual")
		reduceOnly           = flag.Bool("reduce-only", false, "whether order only reduces existing position")
		liquidationPrice     = flag.Float64("liquidation-price", 0, "estimated liquidation price for perpetual")
		fundingRatePct       = flag.Float64("funding-rate-pct", 0, "latest funding rate percentage, e.g. 0.01 means 0.01%")
		equity               = flag.Float64("equity", 0, "account equity")
		dailyPnL             = flag.Float64("daily-pnl", 0, "current daily realized PnL")
		consecutiveLosses    = flag.Int("consecutive-losses", 0, "current consecutive losing trades")
		totalExposure        = flag.Float64("total-exposure", 0, "current total notional exposure")
		symbolExposure       = flag.Float64("symbol-exposure", 0, "current symbol notional exposure")
		maxOrderRiskPct      = flag.Float64("max-order-risk-pct", risk.DefaultConfig().MaxOrderRiskPct, "max risk per order as pct of equity")
		maxSymbolExposurePct = flag.Float64("max-symbol-exposure-pct", risk.DefaultConfig().MaxSymbolExposurePct, "max symbol exposure pct of equity")
		maxTotalExposurePct  = flag.Float64("max-total-exposure-pct", risk.DefaultConfig().MaxTotalExposurePct, "max total exposure pct of equity")
		maxLeverage          = flag.Float64("max-leverage", risk.DefaultConfig().MaxLeverage, "max allowed leverage")
		maxDailyLossPct      = flag.Float64("max-daily-loss-pct", risk.DefaultConfig().MaxDailyLossPct, "daily loss halt pct")
		maxLosses            = flag.Int("max-consecutive-losses", risk.DefaultConfig().MaxConsecutiveLosses, "consecutive loss halt threshold")
		minLiqDistancePct    = flag.Float64("min-liquidation-distance-pct", risk.DefaultConfig().MinLiquidationDistancePct, "minimum liquidation distance pct")
		maxFundingPct        = flag.Float64("max-abs-funding-rate-pct", risk.DefaultConfig().MaxAbsFundingRatePct, "max absolute funding rate pct")
	)
	flag.Parse()

	result, err := risk.EvaluateOrder(risk.Config{
		MaxOrderRiskPct:           *maxOrderRiskPct,
		MaxSymbolExposurePct:      *maxSymbolExposurePct,
		MaxTotalExposurePct:       *maxTotalExposurePct,
		MaxLeverage:               *maxLeverage,
		MaxDailyLossPct:           *maxDailyLossPct,
		MaxConsecutiveLosses:      *maxLosses,
		MinLiquidationDistancePct: *minLiqDistancePct,
		MaxAbsFundingRatePct:      *maxFundingPct,
	}, risk.AccountSnapshot{
		Equity:                *equity,
		DailyRealizedPnL:      *dailyPnL,
		ConsecutiveLosses:     *consecutiveLosses,
		CurrentTotalExposure:  *totalExposure,
		CurrentSymbolExposure: *symbolExposure,
		SnapshotTime:          time.Now().UTC(),
	}, risk.OrderIntent{
		Exchange:             "binance",
		MarketType:           parseMarketType(*market),
		Symbol:               *symbol,
		Side:                 parseSide(*side),
		Price:                *price,
		Quantity:             *quantity,
		StopPrice:            *stopPrice,
		Leverage:             *leverage,
		ReduceOnly:           *reduceOnly,
		LiquidationPrice:     *liquidationPrice,
		LatestFundingRatePct: *fundingRatePct,
	})
	if err != nil {
		return err
	}

	fmt.Printf("decision=%s\n", result.Decision)
	fmt.Printf("order_notional=%.4f order_risk=%.4f total_exposure=%.4f symbol_exposure=%.4f\n",
		result.OrderNotional,
		result.OrderRisk,
		result.TotalExposure,
		result.SymbolExposure,
	)
	for _, event := range result.Events {
		fmt.Printf("event severity=%s type=%s message=%q\n", event.Severity, event.Type, event.Message)
	}
	return nil
}

func parseMarketType(value string) risk.MarketType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "perpetual", "futures", "future":
		return risk.MarketTypePerpetual
	default:
		return risk.MarketTypeSpot
	}
}

func parseSide(value string) risk.Side {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sell":
		return risk.SideSell
	default:
		return risk.SideBuy
	}
}
