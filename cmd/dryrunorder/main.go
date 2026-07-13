package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/risk"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	defaultRisk := risk.DefaultConfig()
	var (
		dsn                  = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID            = flag.String("account", "research", "account id")
		strategyID           = flag.String("strategy", "manual-dry-run", "strategy id")
		clientOrderID        = flag.String("client-order-id", "", "unique client order id; auto-generated if empty")
		market               = flag.String("market", "spot", "market type: spot or perpetual")
		symbol               = flag.String("symbol", "BTCUSDT", "symbol")
		side                 = flag.String("side", "buy", "side: buy or sell")
		orderType            = flag.String("order-type", "limit", "order type")
		timeInForce          = flag.String("time-in-force", "GTC", "time in force")
		price                = flag.Float64("price", 0, "planned order price")
		quantity             = flag.Float64("quantity", 0, "planned order quantity")
		stopPrice            = flag.Float64("stop-price", 0, "planned stop price; 0 disables single-order risk check")
		takeProfitPrice      = flag.Float64("take-profit-price", 0, "planned take profit price; 0 means unset")
		leverage             = flag.Float64("leverage", 1, "planned leverage, required for perpetual")
		reduceOnly           = flag.Bool("reduce-only", false, "whether order only reduces existing position")
		liquidationPrice     = flag.Float64("liquidation-price", 0, "estimated liquidation price for perpetual")
		fundingRatePct       = flag.Float64("funding-rate-pct", 0, "latest funding rate percentage, e.g. 0.01 means 0.01%")
		equity               = flag.Float64("equity", 0, "account equity")
		dailyPnL             = flag.Float64("daily-pnl", 0, "current daily realized PnL")
		consecutiveLosses    = flag.Int("consecutive-losses", 0, "current consecutive losing trades")
		totalExposure        = flag.Float64("total-exposure", 0, "current total notional exposure")
		symbolExposure       = flag.Float64("symbol-exposure", 0, "current symbol notional exposure")
		maxOrderRiskPct      = flag.Float64("max-order-risk-pct", defaultRisk.MaxOrderRiskPct, "max risk per order as pct of equity")
		maxSymbolExposurePct = flag.Float64("max-symbol-exposure-pct", defaultRisk.MaxSymbolExposurePct, "max symbol exposure pct of equity")
		maxTotalExposurePct  = flag.Float64("max-total-exposure-pct", defaultRisk.MaxTotalExposurePct, "max total exposure pct of equity")
		maxLeverage          = flag.Float64("max-leverage", defaultRisk.MaxLeverage, "max allowed leverage")
		maxDailyLossPct      = flag.Float64("max-daily-loss-pct", defaultRisk.MaxDailyLossPct, "daily loss halt pct")
		maxLosses            = flag.Int("max-consecutive-losses", defaultRisk.MaxConsecutiveLosses, "consecutive loss halt threshold")
		minLiqDistancePct    = flag.Float64("min-liquidation-distance-pct", defaultRisk.MinLiquidationDistancePct, "minimum liquidation distance pct")
		maxFundingPct        = flag.Float64("max-abs-funding-rate-pct", defaultRisk.MaxAbsFundingRatePct, "max absolute funding rate pct")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := execution.InitSQLiteSchema(ctx, db); err != nil {
		return fmt.Errorf("init execution schema: %w", err)
	}

	repo := execution.NewSQLiteRepository(db)
	result, err := repo.RecordDryRunOrder(ctx, execution.DryRunRequest{
		Account: risk.AccountSnapshot{
			AccountID:             *accountID,
			Equity:                *equity,
			DailyRealizedPnL:      *dailyPnL,
			ConsecutiveLosses:     *consecutiveLosses,
			CurrentTotalExposure:  *totalExposure,
			CurrentSymbolExposure: *symbolExposure,
			SnapshotTime:          time.Now().UTC(),
		},
		Order: risk.OrderIntent{
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
		},
		RiskConfig: risk.Config{
			MaxOrderRiskPct:           *maxOrderRiskPct,
			MaxSymbolExposurePct:      *maxSymbolExposurePct,
			MaxTotalExposurePct:       *maxTotalExposurePct,
			MaxLeverage:               *maxLeverage,
			MaxDailyLossPct:           *maxDailyLossPct,
			MaxConsecutiveLosses:      *maxLosses,
			MinLiquidationDistancePct: *minLiqDistancePct,
			MaxAbsFundingRatePct:      *maxFundingPct,
		},
		StrategyID:      *strategyID,
		ClientOrderID:   *clientOrderID,
		OrderType:       *orderType,
		TimeInForce:     *timeInForce,
		TakeProfitPrice: *takeProfitPrice,
	})
	if err != nil {
		return err
	}

	printResult(result)
	return nil
}

func printResult(result execution.DryRunResult) {
	fmt.Printf("order_id=%d client_order_id=%s\n", result.Order.ID, result.Order.ClientOrderID)
	fmt.Printf("status=%s risk_decision=%s risk_reason=%s\n", result.Order.Status, result.Order.RiskDecision, result.Order.RiskReason)
	fmt.Printf("market=%s symbol=%s side=%s price=%.8f quantity=%.8f stop=%.8f take_profit=%.8f reduce_only=%t\n",
		result.Order.MarketType,
		result.Order.Symbol,
		result.Order.Side,
		result.Order.Price,
		result.Order.Quantity,
		result.Order.StopPrice,
		result.Order.TakeProfitPrice,
		result.Order.ReduceOnly,
	)
	fmt.Printf("order_notional=%.4f order_risk=%.4f total_exposure=%.4f symbol_exposure=%.4f\n",
		result.RiskResult.OrderNotional,
		result.RiskResult.OrderRisk,
		result.RiskResult.TotalExposure,
		result.RiskResult.SymbolExposure,
	)
	for _, event := range result.Events {
		fmt.Printf("risk_event_id=%d severity=%s type=%s message=%q\n", event.ID, event.Severity, event.EventType, event.Message)
	}
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

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
