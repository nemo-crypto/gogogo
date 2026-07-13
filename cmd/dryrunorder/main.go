package main

import (
	"context"
	"flag"
	"fmt"
	"gogogo/internal/config"
	"log"
	"strings"
	"time"

	"gogogo/internal/exchange/onebullex"
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
		exchange             = flag.String("exchange", env("EXCHANGE_NAME", onebullex.ExchangeName), "exchange name")
		clientOrderID        = flag.String("client-order-id", "", "unique client order id; auto-generated if empty")
		market               = flag.String("market", "perpetual", "market type: perpetual")
		symbol               = flag.String("symbol", "BTCUSDT", "symbol")
		side                 = flag.String("side", "buy", "side: buy or sell")
		orderType            = flag.String("order-type", "limit", "order type")
		timeInForce          = flag.String("time-in-force", "GTC", "time in force")
		submitExchange       = flag.Bool("submit-exchange", false, "submit allowed order to OneBullEx; also requires ONEBULLEX_LIVE_TRADING=true")
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
		availableBalance     = flag.Float64("available-balance", 0, "current available balance; 0 derives from equity minus initial margin")
		initialMargin        = flag.Float64("initial-margin", 0, "current initial margin")
		maintenanceMargin    = flag.Float64("maintenance-margin", 0, "current maintenance margin")
		maxOrderRiskPct      = flag.Float64("max-order-risk-pct", defaultRisk.MaxOrderRiskPct, "max risk per order as pct of equity")
		maxSymbolExposurePct = flag.Float64("max-symbol-exposure-pct", defaultRisk.MaxSymbolExposurePct, "max symbol exposure pct of equity")
		maxTotalExposurePct  = flag.Float64("max-total-exposure-pct", defaultRisk.MaxTotalExposurePct, "max total exposure pct of equity")
		maxInitialMarginPct  = flag.Float64("max-initial-margin-pct", defaultRisk.MaxInitialMarginPct, "max total initial margin pct of equity")
		maxBalanceUsePct     = flag.Float64("max-balance-use-pct", defaultRisk.MaxAvailableBalanceUsePct, "max order initial margin pct of available balance")
		maxLeverage          = flag.Float64("max-leverage", defaultRisk.MaxLeverage, "max allowed leverage")
		maxDailyLossPct      = flag.Float64("max-daily-loss-pct", defaultRisk.MaxDailyLossPct, "daily loss halt pct")
		maxLosses            = flag.Int("max-consecutive-losses", defaultRisk.MaxConsecutiveLosses, "consecutive loss halt threshold")
		minLiqDistancePct    = flag.Float64("min-liquidation-distance-pct", defaultRisk.MinLiquidationDistancePct, "minimum liquidation distance pct")
		maintMarginRatePct   = flag.Float64("maintenance-margin-rate-pct", defaultRisk.MaintenanceMarginRatePct, "maintenance margin rate pct for liquidation estimate")
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
			AvailableBalance:      *availableBalance,
			DailyRealizedPnL:      *dailyPnL,
			ConsecutiveLosses:     *consecutiveLosses,
			CurrentTotalExposure:  *totalExposure,
			CurrentSymbolExposure: *symbolExposure,
			CurrentInitialMargin:  *initialMargin,
			CurrentMaintMargin:    *maintenanceMargin,
			SnapshotTime:          time.Now().UTC(),
		},
		Order: risk.OrderIntent{
			Exchange:             *exchange,
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
			MaxInitialMarginPct:       *maxInitialMarginPct,
			MaxAvailableBalanceUsePct: *maxBalanceUsePct,
			MaxLeverage:               *maxLeverage,
			MaxDailyLossPct:           *maxDailyLossPct,
			MaxConsecutiveLosses:      *maxLosses,
			MinLiquidationDistancePct: *minLiqDistancePct,
			MaintenanceMarginRatePct:  *maintMarginRatePct,
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
	if *submitExchange && result.Order.RiskDecision == risk.DecisionAllow {
		if normalizeExchangeName(result.Order.Exchange) != onebullex.ExchangeName {
			return fmt.Errorf("exchange submit currently supports %s only", onebullex.ExchangeName)
		}
		updated, err := repo.SubmitOrderToExchange(ctx, oneBullExClientFromEnv(), result.Order)
		if err != nil {
			return fmt.Errorf("submit onebullex order: %w", err)
		}
		result.Order = updated
	}

	printResult(result)
	return nil
}

func printResult(result execution.DryRunResult) {
	fmt.Printf("order_id=%d client_order_id=%s\n", result.Order.ID, result.Order.ClientOrderID)
	fmt.Printf("status=%s risk_decision=%s risk_reason=%s exchange_order_id=%s exchange_status=%s\n", result.Order.Status, result.Order.RiskDecision, result.Order.RiskReason, result.Order.ExchangeOrderID, result.Order.ExchangeStatus)
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
	fmt.Printf("order_notional=%.4f order_risk=%.4f order_initial_margin=%.4f total_exposure=%.4f symbol_exposure=%.4f total_initial_margin=%.4f available_balance=%.4f\n",
		result.RiskResult.OrderNotional,
		result.RiskResult.OrderRisk,
		result.RiskResult.OrderInitialMargin,
		result.RiskResult.TotalExposure,
		result.RiskResult.SymbolExposure,
		result.RiskResult.TotalInitialMargin,
		result.RiskResult.AvailableBalance,
	)
	for _, event := range result.Events {
		fmt.Printf("risk_event_id=%d severity=%s type=%s message=%q\n", event.ID, event.Severity, event.EventType, event.Message)
	}
}

func oneBullExClientFromEnv() *onebullex.Client {
	return onebullex.NewClient(
		onebullex.WithBaseURL(env("ONEBULLEX_BASE_URL", "")),
		onebullex.WithCredentials(env("ONEBULLEX_API_KEY", ""), env("ONEBULLEX_SECRET_KEY", "")),
		onebullex.WithTradingEnabled(envBool("ONEBULLEX_LIVE_TRADING", false)),
	)
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(env(key, "")))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func normalizeExchangeName(exchangeName string) string {
	exchangeName = strings.ToLower(strings.TrimSpace(exchangeName))
	if exchangeName == "onebull" || exchangeName == "1bullex" {
		return onebullex.ExchangeName
	}
	return exchangeName
}

func parseMarketType(value string) risk.MarketType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "perpetual", "futures", "future":
		return risk.MarketTypePerpetual
	default:
		return risk.MarketTypePerpetual
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
	return config.Env(key, fallback)
}
