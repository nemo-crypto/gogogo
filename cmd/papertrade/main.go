package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/risk"
	"gogogo/internal/storage"
	"gogogo/internal/strategy"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

type paperProfileFlags struct {
	strategyID    *string
	market        *string
	interval      *string
	strategyType  *string
	fast          *int
	slow          *int
	takeProfitPct *float64
	stopLossPct   *float64
	cooldownBars  *int
	minSpreadPct  *float64
	confirmBars   *int
	atrWindow     *int
	minATRPct     *float64
	maxATRPct     *float64
	volumeWindow  *int
	minVolume     *float64
	maxExtension  *float64
	pullbackBars  *int
	pullbackTol   *float64
	feeRate       *float64
	slippageRate  *float64
	riskPct       *float64
	maxNotional   *float64
	maxMargin     *float64
	maxBalanceUse *float64
	minLiqDist    *float64
	maxOrderRisk  *float64
	maxLeverage   *float64
	leverage      *float64
}

func run() error {
	var (
		dsn           = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID     = flag.String("account", "paper", "paper account id")
		strategyID    = flag.String("strategy", "sma-paper", "strategy id")
		profile       = flag.String("profile", "", "paper strategy profile: aggressive or empty/manual")
		exchange      = flag.String("exchange", "binance", "exchange")
		market        = flag.String("market", "spot", "market type")
		symbol        = flag.String("symbol", "BTCUSDT", "symbol")
		interval      = flag.String("interval", "1h", "interval")
		start         = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time")
		end           = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time")
		strategyType  = flag.String("strategy-type", "sma", "strategy type: sma or scalp-tpsl")
		fast          = flag.Int("fast", 12, "fast SMA")
		slow          = flag.Int("slow", 48, "slow SMA")
		takeProfitPct = flag.Float64("take-profit-pct", 0.8, "take profit pct for scalp-tpsl")
		stopLossPct   = flag.Float64("stop-loss-pct", 0.4, "stop loss pct for scalp-tpsl")
		cooldownBars  = flag.Int("cooldown-bars", 0, "cooldown bars after scalp-tpsl exit")
		minSpreadPct  = flag.Float64("min-trend-spread-pct", 0, "minimum SMA spread pct required to enter scalp-tpsl trades")
		confirmBars   = flag.Int("confirm-bars", 1, "consecutive close direction bars required to enter scalp-tpsl trades")
		atrWindow     = flag.Int("atr-window", 0, "ATR window for scalp-tpsl volatility filter")
		minATRPct     = flag.Float64("min-atr-pct", 0, "minimum ATR pct required to enter scalp-tpsl trades")
		maxATRPct     = flag.Float64("max-atr-pct", 0, "maximum ATR pct allowed to enter scalp-tpsl trades")
		volumeWindow  = flag.Int("volume-window", 0, "volume average window for scalp-tpsl volume filter")
		minVolume     = flag.Float64("min-volume-ratio", 0, "minimum current volume / average volume required to enter scalp-tpsl trades")
		maxExtension  = flag.Float64("max-entry-extension-pct", 0, "maximum entry distance from fast SMA pct; zero disables")
		pullbackBars  = flag.Int("pullback-lookback", 0, "recent bars that must touch fast SMA zone before entry; zero disables")
		pullbackTol   = flag.Float64("pullback-tolerance-pct", 0, "pullback touch tolerance pct around fast SMA")
		feeRate       = flag.Float64("fee-rate", 0.001, "fee rate per trade side")
		slippageRate  = flag.Float64("slippage-rate", 0.0005, "slippage rate per trade side")
		equity        = flag.Float64("equity", 10000, "paper account equity")
		quantity      = flag.Float64("quantity", 0.01, "paper order quantity")
		riskPct       = flag.Float64("risk-pct", 0, "risk pct of equity per trade; overrides -quantity when positive")
		maxNotional   = flag.Float64("max-notional-pct", 100, "maximum notional as pct of equity for paper order sizing")
		maxMargin     = flag.Float64("max-margin-pct", risk.DefaultConfig().MaxInitialMarginPct, "maximum total initial margin as pct of equity")
		maxBalanceUse = flag.Float64("max-balance-use-pct", risk.DefaultConfig().MaxAvailableBalanceUsePct, "maximum order initial margin as pct of available balance")
		minLiqDist    = flag.Float64("min-liquidation-distance-pct", risk.DefaultConfig().MinLiquidationDistancePct, "minimum estimated liquidation distance pct")
		maintMargin   = flag.Float64("maintenance-margin-rate-pct", risk.DefaultConfig().MaintenanceMarginRatePct, "maintenance margin rate pct used for paper liquidation estimate")
		maxOrderRisk  = flag.Float64("max-order-risk-pct", risk.DefaultConfig().MaxOrderRiskPct, "maximum order stop loss risk as pct of equity")
		maxLeverage   = flag.Float64("max-leverage", risk.DefaultConfig().MaxLeverage, "maximum allowed paper leverage")
		leverage      = flag.Float64("leverage", 1, "paper position leverage")
		watch         = flag.Bool("watch", false, "keep running paper strategy on latest local market data")
		pollEvery     = flag.Duration("poll-interval", 15*time.Second, "poll interval when -watch is enabled")
	)
	flag.Parse()
	setFlags := visitedFlagNames()
	if err := applyPaperProfile(*profile, setFlags, paperProfileFlags{
		strategyID:    strategyID,
		market:        market,
		interval:      interval,
		strategyType:  strategyType,
		fast:          fast,
		slow:          slow,
		takeProfitPct: takeProfitPct,
		stopLossPct:   stopLossPct,
		cooldownBars:  cooldownBars,
		minSpreadPct:  minSpreadPct,
		confirmBars:   confirmBars,
		atrWindow:     atrWindow,
		minATRPct:     minATRPct,
		maxATRPct:     maxATRPct,
		volumeWindow:  volumeWindow,
		minVolume:     minVolume,
		maxExtension:  maxExtension,
		pullbackBars:  pullbackBars,
		pullbackTol:   pullbackTol,
		feeRate:       feeRate,
		slippageRate:  slippageRate,
		riskPct:       riskPct,
		maxNotional:   maxNotional,
		maxMargin:     maxMargin,
		maxBalanceUse: maxBalanceUse,
		minLiqDist:    minLiqDist,
		maxOrderRisk:  maxOrderRisk,
		maxLeverage:   maxLeverage,
		leverage:      leverage,
	}); err != nil {
		return err
	}
	actualStrategyID := *strategyID
	if actualStrategyID == "sma-paper" && strings.EqualFold(*strategyType, "scalp-tpsl") {
		actualStrategyID = "scalp-tpsl-paper"
	}

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, *end)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	config := paperRunConfig{
		AccountID:            *accountID,
		StrategyID:           actualStrategyID,
		Profile:              normalizedPaperProfile(*profile),
		Exchange:             *exchange,
		MarketType:           *market,
		Symbol:               *symbol,
		Interval:             *interval,
		Start:                startTime,
		End:                  endTime,
		StrategyType:         *strategyType,
		FastWindow:           *fast,
		SlowWindow:           *slow,
		TakeProfitPct:        *takeProfitPct,
		StopLossPct:          *stopLossPct,
		CooldownBars:         *cooldownBars,
		FeeRate:              *feeRate,
		SlippageRate:         *slippageRate,
		MinTrendSpreadPct:    *minSpreadPct,
		ConfirmBars:          *confirmBars,
		ATRWindow:            *atrWindow,
		MinATRPct:            *minATRPct,
		MaxATRPct:            *maxATRPct,
		VolumeWindow:         *volumeWindow,
		MinVolumeRatio:       *minVolume,
		MaxEntryExtensionPct: *maxExtension,
		PullbackLookback:     *pullbackBars,
		PullbackTolerancePct: *pullbackTol,
		Equity:               *equity,
		Quantity:             *quantity,
		RiskPct:              *riskPct,
		MaxNotionalPct:       *maxNotional,
		MaxMarginPct:         *maxMargin,
		MaxBalanceUsePct:     *maxBalanceUse,
		MinLiqDistancePct:    *minLiqDist,
		MaintMarginPct:       *maintMargin,
		MaxOrderRiskPct:      *maxOrderRisk,
		MaxLeverage:          *maxLeverage,
		Leverage:             *leverage,
		Watch:                *watch,
		PollInterval:         *pollEvery,
		LookbackCandles:      120,
	}

	if config.Watch {
		return watchPaperStrategy(context.Background(), db, config)
	}

	return runPaperStrategyOnce(ctx, db, config)
}

func visitedFlagNames() map[string]struct{} {
	visited := make(map[string]struct{})
	flag.Visit(func(f *flag.Flag) {
		visited[f.Name] = struct{}{}
	})
	return visited
}

func applyPaperProfile(profile string, visited map[string]struct{}, flags paperProfileFlags) error {
	switch normalizedPaperProfile(profile) {
	case "":
		return nil
	case "aggressive":
		setStringFlag(visited, "strategy", flags.strategyID, "perp-trend-scalp-aggressive-paper")
		setStringFlag(visited, "market", flags.market, "perpetual")
		setStringFlag(visited, "interval", flags.interval, "1m")
		setStringFlag(visited, "strategy-type", flags.strategyType, "scalp-tpsl")
		setIntFlag(visited, "fast", flags.fast, 3)
		setIntFlag(visited, "slow", flags.slow, 12)
		setFloatFlag(visited, "take-profit-pct", flags.takeProfitPct, 0.65)
		setFloatFlag(visited, "stop-loss-pct", flags.stopLossPct, 0.25)
		setIntFlag(visited, "cooldown-bars", flags.cooldownBars, 1)
		setFloatFlag(visited, "min-trend-spread-pct", flags.minSpreadPct, 0.03)
		setIntFlag(visited, "confirm-bars", flags.confirmBars, 1)
		setIntFlag(visited, "atr-window", flags.atrWindow, 14)
		setFloatFlag(visited, "min-atr-pct", flags.minATRPct, 0.08)
		setFloatFlag(visited, "max-atr-pct", flags.maxATRPct, 1.6)
		setIntFlag(visited, "volume-window", flags.volumeWindow, 20)
		setFloatFlag(visited, "min-volume-ratio", flags.minVolume, 1.15)
		setFloatFlag(visited, "max-entry-extension-pct", flags.maxExtension, 0.18)
		setIntFlag(visited, "pullback-lookback", flags.pullbackBars, 5)
		setFloatFlag(visited, "pullback-tolerance-pct", flags.pullbackTol, 0.06)
		setFloatFlag(visited, "fee-rate", flags.feeRate, 0.0005)
		setFloatFlag(visited, "slippage-rate", flags.slippageRate, 0.0005)
		setFloatFlag(visited, "risk-pct", flags.riskPct, 2)
		setFloatFlag(visited, "max-notional-pct", flags.maxNotional, 220)
		setFloatFlag(visited, "max-margin-pct", flags.maxMargin, 65)
		setFloatFlag(visited, "max-balance-use-pct", flags.maxBalanceUse, 90)
		setFloatFlag(visited, "min-liquidation-distance-pct", flags.minLiqDist, 15)
		setFloatFlag(visited, "max-order-risk-pct", flags.maxOrderRisk, 2.5)
		setFloatFlag(visited, "max-leverage", flags.maxLeverage, 3)
		setFloatFlag(visited, "leverage", flags.leverage, 3)
		return nil
	default:
		return fmt.Errorf("unsupported paper profile %q", profile)
	}
}

func normalizedPaperProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", "manual", "none", "default":
		return ""
	case "aggressive", "small-aggressive", "small_aggressive", "small":
		return "aggressive"
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}

func setStringFlag(visited map[string]struct{}, name string, target *string, value string) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setIntFlag(visited map[string]struct{}, name string, target *int, value int) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setFloatFlag(visited map[string]struct{}, name string, target *float64, value float64) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

type paperRunConfig struct {
	AccountID            string
	StrategyID           string
	Profile              string
	Exchange             string
	MarketType           string
	Symbol               string
	Interval             string
	Start                time.Time
	End                  time.Time
	StrategyType         string
	FastWindow           int
	SlowWindow           int
	TakeProfitPct        float64
	StopLossPct          float64
	CooldownBars         int
	FeeRate              float64
	SlippageRate         float64
	MinTrendSpreadPct    float64
	ConfirmBars          int
	ATRWindow            int
	MinATRPct            float64
	MaxATRPct            float64
	VolumeWindow         int
	MinVolumeRatio       float64
	MaxEntryExtensionPct float64
	PullbackLookback     int
	PullbackTolerancePct float64
	Equity               float64
	Quantity             float64
	RiskPct              float64
	MaxNotionalPct       float64
	MaxMarginPct         float64
	MaxBalanceUsePct     float64
	MinLiqDistancePct    float64
	MaintMarginPct       float64
	MaxOrderRiskPct      float64
	MaxLeverage          float64
	Leverage             float64
	Watch                bool
	PollInterval         time.Duration
	LookbackCandles      int
}

func watchPaperStrategy(ctx context.Context, db *sql.DB, config paperRunConfig) error {
	if config.PollInterval <= 0 {
		config.PollInterval = 15 * time.Second
	}
	log.Printf("papertrade watch started: strategy=%s symbol=%s interval=%s poll_interval=%s", config.StrategyID, config.Symbol, config.Interval, config.PollInterval)
	for {
		current := config
		current.End = time.Now().UTC()
		current.Start = current.End.Add(-paperLookbackDuration(config.Interval, config.LookbackCandles))
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := runPaperStrategyOnce(runCtx, db, current); err != nil {
			log.Printf("papertrade watch error: %v", err)
		}
		cancel()

		timer := time.NewTimer(config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func runPaperStrategyOnce(ctx context.Context, db *sql.DB, config paperRunConfig) error {
	parsedMarket := marketdata.MarketType(config.MarketType)
	mdRepo := marketdata.NewSQLiteRepository(db)
	candles, err := mdRepo.ListCandles(ctx, marketdata.CandleQuery{
		Exchange:   config.Exchange,
		MarketType: parsedMarket,
		Symbol:     config.Symbol,
		Interval:   config.Interval,
		Start:      config.Start,
		End:        config.End,
		Limit:      10000,
	})
	if err != nil {
		return fmt.Errorf("list candles: %w", err)
	}
	result, err := runPaperBacktest(candles, paperStrategyConfig{
		StrategyType:         config.StrategyType,
		MarketType:           config.MarketType,
		FastWindow:           config.FastWindow,
		SlowWindow:           config.SlowWindow,
		TakeProfitPct:        config.TakeProfitPct,
		StopLossPct:          config.StopLossPct,
		CooldownBars:         config.CooldownBars,
		FeeRate:              config.FeeRate,
		SlippageRate:         config.SlippageRate,
		MinTrendSpreadPct:    config.MinTrendSpreadPct,
		ConfirmBars:          config.ConfirmBars,
		ATRWindow:            config.ATRWindow,
		MinATRPct:            config.MinATRPct,
		MaxATRPct:            config.MaxATRPct,
		VolumeWindow:         config.VolumeWindow,
		MinVolumeRatio:       config.MinVolumeRatio,
		MaxEntryExtensionPct: config.MaxEntryExtensionPct,
		PullbackLookback:     config.PullbackLookback,
		PullbackTolerancePct: config.PullbackTolerancePct,
	})
	if err != nil {
		return fmt.Errorf("run paper strategy: %w", err)
	}

	backtestRepo := backtest.NewSQLiteRepository(db)
	backtestRunID, err := backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
		Exchange:   config.Exchange,
		MarketType: string(parsedMarket),
		Config: backtest.SMAConfig{
			FastWindow: config.FastWindow,
			SlowWindow: config.SlowWindow,
			FeeRate:    paperBacktestFeeRate(config.StrategyType, config.FeeRate, config.SlippageRate),
		},
		Result: result,
	})
	if err != nil {
		return fmt.Errorf("save paper backtest run: %w", err)
	}

	strategyRepo := strategy.NewSQLiteRepository(db)
	configJSON, err := marshalJSON(map[string]any{
		"strategy_type":           normalizedStrategyType(config.StrategyType),
		"profile":                 config.Profile,
		"fast":                    config.FastWindow,
		"slow":                    config.SlowWindow,
		"symbol":                  config.Symbol,
		"take_profit_pct":         config.TakeProfitPct,
		"stop_loss_pct":           config.StopLossPct,
		"cooldown_bars":           config.CooldownBars,
		"fee_rate":                config.FeeRate,
		"slippage_rate":           config.SlippageRate,
		"market_type":             config.MarketType,
		"min_trend_spread_pct":    config.MinTrendSpreadPct,
		"confirm_bars":            config.ConfirmBars,
		"atr_window":              config.ATRWindow,
		"min_atr_pct":             config.MinATRPct,
		"max_atr_pct":             config.MaxATRPct,
		"volume_window":           config.VolumeWindow,
		"min_volume_ratio":        config.MinVolumeRatio,
		"max_entry_extension_pct": config.MaxEntryExtensionPct,
		"pullback_lookback":       config.PullbackLookback,
		"pullback_tolerance_pct":  config.PullbackTolerancePct,
		"risk_pct":                config.RiskPct,
		"max_notional_pct":        config.MaxNotionalPct,
		"max_margin_pct":          config.MaxMarginPct,
		"max_balance_use_pct":     config.MaxBalanceUsePct,
		"min_liq_distance_pct":    config.MinLiqDistancePct,
		"maintenance_margin_pct":  config.MaintMarginPct,
		"max_order_risk_pct":      config.MaxOrderRiskPct,
		"max_leverage":            config.MaxLeverage,
		"leverage":                config.Leverage,
	})
	if err != nil {
		return fmt.Errorf("encode strategy config: %w", err)
	}
	latestPrice, latestTime, err := latestCandlePrice(candles)
	if err != nil {
		return err
	}

	portfolioRepo := portfolio.NewSQLiteRepository(db)
	paperState, err := settlePaperPosition(ctx, portfolioRepo, paperSettleRequest{
		AccountID:       config.AccountID,
		StrategyID:      config.StrategyID,
		Exchange:        config.Exchange,
		MarketType:      config.MarketType,
		Symbol:          config.Symbol,
		MarkPrice:       latestPrice,
		MarkTime:        latestTime,
		Equity:          config.Equity,
		Leverage:        config.Leverage,
		MaintMarginPct:  config.MaintMarginPct,
		TakeProfitPct:   config.TakeProfitPct,
		StopLossPct:     config.StopLossPct,
		FastWindow:      config.FastWindow,
		SlowWindow:      config.SlowWindow,
		FeeRate:         config.FeeRate,
		SlippageRate:    config.SlippageRate,
		StrategyType:    config.StrategyType,
		Candles:         candles,
		AllowPaperState: true,
	})
	if err != nil {
		return err
	}

	runID, err := strategyRepo.StartRun(ctx, strategy.RunRecord{
		StrategyID: config.StrategyID,
		Mode:       "paper",
		ConfigJSON: configJSON,
	})
	if err != nil {
		return fmt.Errorf("start strategy run: %w", err)
	}
	signal := paperSignalAction(candles, result, paperStrategyConfig{
		StrategyType:         config.StrategyType,
		MarketType:           config.MarketType,
		FastWindow:           config.FastWindow,
		SlowWindow:           config.SlowWindow,
		TakeProfitPct:        config.TakeProfitPct,
		StopLossPct:          config.StopLossPct,
		MinTrendSpreadPct:    config.MinTrendSpreadPct,
		ConfirmBars:          config.ConfirmBars,
		ATRWindow:            config.ATRWindow,
		MinATRPct:            config.MinATRPct,
		MaxATRPct:            config.MaxATRPct,
		VolumeWindow:         config.VolumeWindow,
		MinVolumeRatio:       config.MinVolumeRatio,
		MaxEntryExtensionPct: config.MaxEntryExtensionPct,
		PullbackLookback:     config.PullbackLookback,
		PullbackTolerancePct: config.PullbackTolerancePct,
	})
	action := signal.Action
	if paperState.Open {
		action = strategy.SignalHold
	}
	rawFeaturesJSON, err := marshalJSON(map[string]any{
		"strategy_name":           result.StrategyName,
		"total_return_pct":        result.TotalReturnPct,
		"excess_return_pct":       result.ExcessReturnPct,
		"max_drawdown_pct":        result.MaxDrawdownPct,
		"trade_count":             len(result.Trades),
		"win_rate_pct":            result.WinRatePct,
		"backtest_run_id":         backtestRunID,
		"profile":                 config.Profile,
		"take_profit_pct":         config.TakeProfitPct,
		"stop_loss_pct":           config.StopLossPct,
		"min_trend_spread_pct":    config.MinTrendSpreadPct,
		"confirm_bars":            config.ConfirmBars,
		"atr_window":              config.ATRWindow,
		"min_atr_pct":             config.MinATRPct,
		"max_atr_pct":             config.MaxATRPct,
		"volume_window":           config.VolumeWindow,
		"min_volume_ratio":        config.MinVolumeRatio,
		"max_entry_extension_pct": config.MaxEntryExtensionPct,
		"pullback_lookback":       config.PullbackLookback,
		"pullback_tolerance_pct":  config.PullbackTolerancePct,
		"risk_pct":                config.RiskPct,
		"max_notional_pct":        config.MaxNotionalPct,
		"max_margin_pct":          config.MaxMarginPct,
		"max_balance_use_pct":     config.MaxBalanceUsePct,
		"min_liq_distance_pct":    config.MinLiqDistancePct,
		"latest_signal_input":     normalizedStrategyType(config.StrategyType),
		"position_side":           signal.PositionSide,
	})
	if err != nil {
		return fmt.Errorf("encode signal features: %w", err)
	}
	if _, err := strategyRepo.SaveSignal(ctx, strategy.SignalRecord{
		StrategyID:      config.StrategyID,
		RunID:           runID,
		Exchange:        config.Exchange,
		MarketType:      config.MarketType,
		Symbol:          config.Symbol,
		Action:          action,
		Confidence:      confidence(result.ExcessReturnPct),
		Reason:          paperSignalReason(config.StrategyType),
		RawFeaturesJSON: rawFeaturesJSON,
	}); err != nil {
		return fmt.Errorf("save signal: %w", err)
	}
	metricsJSON, err := marshalJSON(map[string]any{
		"total_return_pct":        result.TotalReturnPct,
		"excess_return_pct":       result.ExcessReturnPct,
		"trades":                  len(result.Trades),
		"win_rate_pct":            result.WinRatePct,
		"profile":                 config.Profile,
		"take_profit_pct":         config.TakeProfitPct,
		"stop_loss_pct":           config.StopLossPct,
		"min_trend_spread_pct":    config.MinTrendSpreadPct,
		"confirm_bars":            config.ConfirmBars,
		"atr_window":              config.ATRWindow,
		"min_atr_pct":             config.MinATRPct,
		"max_atr_pct":             config.MaxATRPct,
		"volume_window":           config.VolumeWindow,
		"min_volume_ratio":        config.MinVolumeRatio,
		"max_entry_extension_pct": config.MaxEntryExtensionPct,
		"pullback_lookback":       config.PullbackLookback,
		"pullback_tolerance_pct":  config.PullbackTolerancePct,
		"risk_pct":                config.RiskPct,
		"max_notional_pct":        config.MaxNotionalPct,
		"max_margin_pct":          config.MaxMarginPct,
		"max_balance_use_pct":     config.MaxBalanceUsePct,
		"min_liq_distance_pct":    config.MinLiqDistancePct,
	})
	if err != nil {
		return fmt.Errorf("encode performance metrics: %w", err)
	}
	if _, err := strategyRepo.SavePerformanceSnapshot(ctx, strategy.PerformanceSnapshot{
		StrategyID:  config.StrategyID,
		RunID:       runID,
		Equity:      paperState.Equity,
		PnL:         paperState.TotalPnL,
		DrawdownPct: result.MaxDrawdownPct,
		Exposure:    result.FinalEquity,
		MetricsJSON: metricsJSON,
	}); err != nil {
		return fmt.Errorf("save performance: %w", err)
	}

	if action != strategy.SignalHold {
		side, positionSide, stopPrice, takeProfitPrice, err := paperOrderPlan(action, latestPrice, config.TakeProfitPct, config.StopLossPct)
		if err != nil {
			return err
		}
		orderQuantity, err := paperOrderQuantity(latestPrice, stopPrice, config, paperState)
		if err != nil {
			return err
		}
		orderRepo := execution.NewSQLiteRepository(db)
		liquidationPrice := risk.EstimateLiquidationPrice(latestPrice, side, config.Leverage, config.MaintMarginPct)
		dryRun, err := orderRepo.RecordDryRunOrder(ctx, execution.DryRunRequest{
			Account: paperRiskAccountSnapshot(config, paperState),
			Order: risk.OrderIntent{
				Exchange:         config.Exchange,
				MarketType:       risk.MarketType(config.MarketType),
				Symbol:           config.Symbol,
				Side:             side,
				Price:            latestPrice,
				Quantity:         orderQuantity,
				StopPrice:        stopPrice,
				Leverage:         config.Leverage,
				LiquidationPrice: liquidationPrice,
			},
			RiskConfig:      paperRiskConfig(config),
			StrategyID:      config.StrategyID,
			ClientOrderID:   fmt.Sprintf("paper-%s-%d", config.Symbol, time.Now().UTC().UnixNano()),
			OrderType:       "market",
			TimeInForce:     "IOC",
			TakeProfitPrice: takeProfitPrice,
		})
		if err != nil {
			return fmt.Errorf("record paper dry-run order: %w", err)
		}
		if !strings.EqualFold(string(dryRun.Order.RiskDecision), string(risk.DecisionAllow)) {
			fmt.Printf("paper_order_id=%d status=%s decision=%s stop=%.8f take_profit=%.8f rejected_no_position=true\n", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
			return nil
		}
		opened, err := openPaperPosition(ctx, portfolioRepo, paperOpenRequest{
			AccountID:       config.AccountID,
			StrategyID:      config.StrategyID,
			Exchange:        config.Exchange,
			MarketType:      config.MarketType,
			Symbol:          config.Symbol,
			PositionSide:    positionSide,
			Quantity:        orderQuantity,
			EntryPrice:      latestPrice,
			TakeProfitPrice: takeProfitPrice,
			StopLossPrice:   stopPrice,
			Equity:          paperState.Equity,
			Leverage:        config.Leverage,
			MaintMarginPct:  config.MaintMarginPct,
			FeeRate:         config.FeeRate,
			SlippageRate:    config.SlippageRate,
			OpenedAt:        latestTime,
		})
		if err != nil {
			return err
		}
		fmt.Printf("paper_order_id=%d status=%s decision=%s stop=%.8f take_profit=%.8f\n", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
		fmt.Printf("paper_position_id=%d status=open entry=%.8f mark=%.8f pnl=%.8f\n", opened.ID, opened.EntryPrice, opened.MarkPrice, paperPositionNetPnL(opened, latestPrice, config.FeeRate, config.SlippageRate))
	}

	fmt.Printf("paper_run_id=%d backtest_run_id=%d strategy=%s symbol=%s return_pct=%.4f excess_pct=%.4f drawdown_pct=%.4f trades=%d\n", runID, backtestRunID, config.StrategyID, config.Symbol, result.TotalReturnPct, result.ExcessReturnPct, result.MaxDrawdownPct, len(result.Trades))
	return nil
}

type paperStrategyConfig struct {
	StrategyType         string
	FastWindow           int
	SlowWindow           int
	TakeProfitPct        float64
	StopLossPct          float64
	CooldownBars         int
	FeeRate              float64
	SlippageRate         float64
	MarketType           string
	MinTrendSpreadPct    float64
	ConfirmBars          int
	ATRWindow            int
	MinATRPct            float64
	MaxATRPct            float64
	VolumeWindow         int
	MinVolumeRatio       float64
	MaxEntryExtensionPct float64
	PullbackLookback     int
	PullbackTolerancePct float64
}

type paperSettleRequest struct {
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	StrategyType    string
	MarkPrice       float64
	MarkTime        time.Time
	Equity          float64
	Leverage        float64
	MaintMarginPct  float64
	TakeProfitPct   float64
	StopLossPct     float64
	FastWindow      int
	SlowWindow      int
	FeeRate         float64
	SlippageRate    float64
	Candles         []marketdata.Candle
	AllowPaperState bool
}

type paperAccountState struct {
	Open                  bool
	Equity                float64
	AvailableBalance      float64
	TotalPnL              float64
	RealizedPnL           float64
	UnrealizedPnL         float64
	CurrentExposure       float64
	CurrentSymbolExposure float64
	CurrentInitialMargin  float64
	CurrentMaintMargin    float64
	Position              portfolio.PaperPositionRecord
	CloseNote             string
}

type paperOpenRequest struct {
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	PositionSide    string
	Quantity        float64
	EntryPrice      float64
	TakeProfitPrice float64
	StopLossPrice   float64
	Equity          float64
	Leverage        float64
	MaintMarginPct  float64
	FeeRate         float64
	SlippageRate    float64
	OpenedAt        time.Time
}

func settlePaperPosition(ctx context.Context, repo *portfolio.SQLiteRepository, request paperSettleRequest) (paperAccountState, error) {
	realizedPnL, err := repo.SumClosedPaperPositionRealizedPnL(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		return paperAccountState{}, fmt.Errorf("sum closed paper pnl: %w", err)
	}
	state := paperAccountState{
		Equity:           request.Equity + realizedPnL,
		AvailableBalance: request.Equity + realizedPnL,
		TotalPnL:         realizedPnL,
		RealizedPnL:      realizedPnL,
	}
	position, err := repo.LatestOpenPaperPosition(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
				AccountID:      request.AccountID,
				Exchange:       request.Exchange,
				MarketType:     request.MarketType,
				Symbol:         request.Symbol,
				Equity:         state.Equity,
				Leverage:       request.Leverage,
				MaintMarginPct: request.MaintMarginPct,
				MarkPrice:      request.MarkPrice,
				SnapshotTime:   request.MarkTime,
				FeeRate:        request.FeeRate,
				SlippageRate:   request.SlippageRate,
			}); err != nil {
				return paperAccountState{}, err
			}
			return state, nil
		}
		return paperAccountState{}, fmt.Errorf("load open paper position: %w", err)
	}

	exitReason := paperExitReason(position, request.MarkPrice)
	if exitReason == "" {
		exitReason = paperTrendExitReason(position, request.Candles, paperStrategyConfig{
			StrategyType: request.StrategyType,
			MarketType:   request.MarketType,
			FastWindow:   request.FastWindow,
			SlowWindow:   request.SlowWindow,
		})
	}
	if exitReason != "" {
		realizedPnL := paperPositionNetPnL(position, request.MarkPrice, request.FeeRate, request.SlippageRate)
		closed, err := repo.ClosePaperPositionWithRealizedPnL(ctx, position.ID, request.MarkPrice, request.MarkTime, realizedPnL)
		if err != nil {
			return paperAccountState{}, fmt.Errorf("close paper position: %w", err)
		}
		state.Position = closed
		state.RealizedPnL += closed.RealizedPnL
		state.TotalPnL = state.RealizedPnL
		state.Equity = request.Equity + state.TotalPnL
		state.AvailableBalance = state.Equity
		state.CloseNote = exitReason
		if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
			AccountID:      request.AccountID,
			Exchange:       request.Exchange,
			MarketType:     request.MarketType,
			Symbol:         request.Symbol,
			Equity:         state.Equity,
			Leverage:       request.Leverage,
			MaintMarginPct: request.MaintMarginPct,
			MarkPrice:      request.MarkPrice,
			SnapshotTime:   request.MarkTime,
			FeeRate:        request.FeeRate,
			SlippageRate:   request.SlippageRate,
		}); err != nil {
			return paperAccountState{}, err
		}
		fmt.Printf("paper_position_closed id=%d reason=%s exit=%.8f realized_pnl=%.8f\n", closed.ID, exitReason, request.MarkPrice, closed.RealizedPnL)
		return state, nil
	}

	if err := repo.UpdatePaperPositionMark(ctx, position.ID, request.MarkPrice); err != nil {
		return paperAccountState{}, fmt.Errorf("update paper position mark: %w", err)
	}
	position.MarkPrice = request.MarkPrice
	state.Open = true
	state.Position = position
	state.UnrealizedPnL = paperPositionNetPnL(position, request.MarkPrice, request.FeeRate, request.SlippageRate)
	state.TotalPnL = state.RealizedPnL + state.UnrealizedPnL
	state.Equity = request.Equity + state.TotalPnL
	state.CurrentExposure = paperPositionNotional(position, request.MarkPrice)
	state.CurrentSymbolExposure = state.CurrentExposure
	state.CurrentInitialMargin = paperInitialMargin(state.CurrentExposure, request.Leverage)
	state.CurrentMaintMargin = paperMaintenanceMargin(state.CurrentExposure, request.MaintMarginPct)
	state.AvailableBalance = paperAvailableBalance(state.Equity, state.CurrentInitialMargin)
	if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
		AccountID:      request.AccountID,
		Exchange:       request.Exchange,
		MarketType:     request.MarketType,
		Symbol:         request.Symbol,
		Equity:         state.Equity,
		Leverage:       request.Leverage,
		MaintMarginPct: request.MaintMarginPct,
		Position:       position,
		MarkPrice:      request.MarkPrice,
		SnapshotTime:   request.MarkTime,
		FeeRate:        request.FeeRate,
		SlippageRate:   request.SlippageRate,
	}); err != nil {
		return paperAccountState{}, err
	}
	return state, nil
}

func openPaperPosition(ctx context.Context, repo *portfolio.SQLiteRepository, request paperOpenRequest) (portfolio.PaperPositionRecord, error) {
	id, err := repo.OpenPaperPosition(ctx, portfolio.PaperPositionRecord{
		AccountID:       request.AccountID,
		StrategyID:      request.StrategyID,
		Exchange:        request.Exchange,
		MarketType:      request.MarketType,
		Symbol:          request.Symbol,
		PositionSide:    request.PositionSide,
		Quantity:        request.Quantity,
		EntryPrice:      request.EntryPrice,
		MarkPrice:       request.EntryPrice,
		TakeProfitPrice: request.TakeProfitPrice,
		StopLossPrice:   request.StopLossPrice,
		OpenedAt:        request.OpenedAt,
	})
	if err != nil {
		return portfolio.PaperPositionRecord{}, fmt.Errorf("open paper position: %w", err)
	}
	position, err := repo.LatestOpenPaperPosition(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		return portfolio.PaperPositionRecord{}, fmt.Errorf("reload opened paper position %d: %w", id, err)
	}
	openPnL := paperPositionNetPnL(position, request.EntryPrice, request.FeeRate, request.SlippageRate)
	if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
		AccountID:      request.AccountID,
		Exchange:       request.Exchange,
		MarketType:     request.MarketType,
		Symbol:         request.Symbol,
		Equity:         request.Equity + openPnL,
		Leverage:       request.Leverage,
		MaintMarginPct: request.MaintMarginPct,
		Position:       position,
		MarkPrice:      request.EntryPrice,
		SnapshotTime:   request.OpenedAt,
		FeeRate:        request.FeeRate,
		SlippageRate:   request.SlippageRate,
	}); err != nil {
		return portfolio.PaperPositionRecord{}, err
	}
	return position, nil
}

type paperSnapshotRequest struct {
	AccountID      string
	Exchange       string
	MarketType     string
	Symbol         string
	Equity         float64
	Leverage       float64
	MaintMarginPct float64
	Position       portfolio.PaperPositionRecord
	MarkPrice      float64
	SnapshotTime   time.Time
	FeeRate        float64
	SlippageRate   float64
}

func savePaperAccountSnapshots(ctx context.Context, repo *portfolio.SQLiteRepository, request paperSnapshotRequest) error {
	if request.SnapshotTime.IsZero() {
		request.SnapshotTime = time.Now().UTC()
	}
	notional := paperPositionNotional(request.Position, request.MarkPrice)
	locked := paperInitialMargin(notional, request.Leverage)
	free := paperAvailableBalance(request.Equity, locked)
	maintenanceMargin := paperMaintenanceMargin(notional, request.MaintMarginPct)
	marginRatio := 0.0
	if request.Equity > 0 {
		marginRatio = maintenanceMargin / request.Equity * 100
	}
	if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
		AccountID:    request.AccountID,
		Exchange:     request.Exchange,
		Asset:        "USDT",
		Free:         free,
		Locked:       locked,
		Total:        request.Equity,
		USDValue:     request.Equity,
		SnapshotTime: request.SnapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper balance snapshot: %w", err)
	}

	if request.Position.ID != 0 {
		pnl := paperPositionNetPnL(request.Position, request.MarkPrice, request.FeeRate, request.SlippageRate)
		liqPrice := paperLiquidationPrice(request.Position.PositionSide, request.Position.EntryPrice, request.Leverage, request.MaintMarginPct)
		if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
			AccountID:        request.AccountID,
			Exchange:         request.Exchange,
			MarketType:       request.MarketType,
			Symbol:           request.Symbol,
			PositionSide:     request.Position.PositionSide,
			Quantity:         request.Position.Quantity,
			EntryPrice:       request.Position.EntryPrice,
			MarkPrice:        request.MarkPrice,
			LiquidationPrice: liqPrice,
			Leverage:         request.Leverage,
			MarginMode:       "isolated-paper",
			UnrealizedPnL:    pnl,
			Notional:         notional,
			SnapshotTime:     request.SnapshotTime,
		}); err != nil {
			return fmt.Errorf("save paper position snapshot: %w", err)
		}
	} else {
		for _, side := range []string{"long", "short"} {
			if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
				AccountID:    request.AccountID,
				Exchange:     request.Exchange,
				MarketType:   request.MarketType,
				Symbol:       request.Symbol,
				PositionSide: side,
				Quantity:     0,
				EntryPrice:   0,
				MarkPrice:    request.MarkPrice,
				Leverage:     request.Leverage,
				MarginMode:   "isolated-paper",
				SnapshotTime: request.SnapshotTime,
			}); err != nil {
				return fmt.Errorf("save flat paper %s position snapshot: %w", side, err)
			}
		}
	}

	if _, err := repo.SaveMarginSnapshot(ctx, portfolio.MarginSnapshot{
		AccountID:         request.AccountID,
		Exchange:          request.Exchange,
		MarketType:        request.MarketType,
		Equity:            request.Equity,
		MarginBalance:     request.Equity,
		InitialMargin:     locked,
		MaintenanceMargin: maintenanceMargin,
		MarginRatio:       marginRatio,
		AvailableBalance:  free,
		SnapshotTime:      request.SnapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper margin snapshot: %w", err)
	}
	return nil
}

func paperExitReason(position portfolio.PaperPositionRecord, markPrice float64) string {
	if strings.EqualFold(position.PositionSide, "short") {
		if position.TakeProfitPrice > 0 && markPrice <= position.TakeProfitPrice {
			return "take_profit"
		}
		if position.StopLossPrice > 0 && markPrice >= position.StopLossPrice {
			return "stop_loss"
		}
		return ""
	}
	if position.TakeProfitPrice > 0 && markPrice >= position.TakeProfitPrice {
		return "take_profit"
	}
	if position.StopLossPrice > 0 && markPrice <= position.StopLossPrice {
		return "stop_loss"
	}
	return ""
}

func paperTrendExitReason(position portfolio.PaperPositionRecord, candles []marketdata.Candle, config paperStrategyConfig) string {
	if normalizedStrategyType(config.StrategyType) != "scalp-tpsl" {
		return ""
	}
	fastAverage, slowAverage, ok := latestAverages(candles, config.FastWindow, config.SlowWindow)
	if !ok {
		return ""
	}
	if strings.EqualFold(position.PositionSide, "short") {
		if fastAverage > slowAverage {
			return "trend_reversal"
		}
		return ""
	}
	if fastAverage < slowAverage {
		return "trend_reversal"
	}
	return ""
}

func paperOrderPlan(action strategy.SignalAction, price float64, takeProfitPct float64, stopLossPct float64) (risk.Side, string, float64, float64, error) {
	if price <= 0 {
		return "", "", 0, 0, errors.New("price must be positive")
	}
	switch action {
	case strategy.SignalBuy:
		return risk.SideBuy, "long", price * (1 - stopLossPct/100), price * (1 + takeProfitPct/100), nil
	case strategy.SignalShort:
		return risk.SideSell, "short", price * (1 + stopLossPct/100), price * (1 - takeProfitPct/100), nil
	default:
		return "", "", 0, 0, fmt.Errorf("unsupported paper action %q", action)
	}
}

func paperOrderQuantity(entryPrice float64, stopPrice float64, config paperRunConfig, state paperAccountState) (float64, error) {
	if entryPrice <= 0 || stopPrice <= 0 {
		return 0, errors.New("entry and stop price must be positive")
	}
	equity := state.Equity
	if equity <= 0 {
		equity = config.Equity
	}
	availableBalance := state.AvailableBalance
	if availableBalance <= 0 {
		availableBalance = equity
	}
	if equity <= 0 {
		return 0, errors.New("equity must be positive")
	}
	if config.RiskPct <= 0 {
		if config.Quantity <= 0 {
			return 0, errors.New("quantity must be positive")
		}
		quantity := paperCapQuantity(entryPrice, config.Quantity, config, equity, availableBalance, state.CurrentInitialMargin)
		if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
			return 0, errors.New("computed quantity must be positive and finite")
		}
		return quantity, nil
	}
	riskDistance := math.Abs(entryPrice - stopPrice)
	if riskDistance <= 0 {
		return 0, errors.New("stop price must be different from entry price")
	}
	riskBudget := equity * config.RiskPct / 100
	quantity := riskBudget / riskDistance
	quantity = paperCapQuantity(entryPrice, quantity, config, equity, availableBalance, state.CurrentInitialMargin)
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, errors.New("computed quantity must be positive and finite")
	}
	return quantity, nil
}

func paperCapQuantity(entryPrice float64, quantity float64, config paperRunConfig, equity float64, availableBalance float64, currentInitialMargin float64) float64 {
	if quantity <= 0 || entryPrice <= 0 || equity <= 0 {
		return 0
	}
	if config.MaxNotionalPct > 0 {
		maxNotional := equity * config.MaxNotionalPct / 100
		maxQuantity := maxNotional / entryPrice
		if quantity > maxQuantity {
			quantity = maxQuantity
		}
	}
	if config.Leverage > 0 {
		if config.MaxBalanceUsePct > 0 && availableBalance > 0 {
			maxOrderMargin := availableBalance * config.MaxBalanceUsePct / 100
			maxQuantity := maxOrderMargin * config.Leverage / entryPrice
			if quantity > maxQuantity {
				quantity = maxQuantity
			}
		}
		if config.MaxMarginPct > 0 {
			maxTotalMargin := equity * config.MaxMarginPct / 100
			remainingMargin := maxTotalMargin - currentInitialMargin
			if remainingMargin < 0 {
				remainingMargin = 0
			}
			maxQuantity := remainingMargin * config.Leverage / entryPrice
			if quantity > maxQuantity {
				quantity = maxQuantity
			}
		}
	}
	return quantity
}

func paperRiskConfig(config paperRunConfig) risk.Config {
	defaultRisk := risk.DefaultConfig()
	maxSymbolExposurePct := config.MaxNotionalPct
	if maxSymbolExposurePct <= 0 {
		maxSymbolExposurePct = defaultRisk.MaxSymbolExposurePct
	}
	maxTotalExposurePct := math.Max(defaultRisk.MaxTotalExposurePct, maxSymbolExposurePct)
	return risk.Config{
		MaxOrderRiskPct:           paperPositiveOrDefault(config.MaxOrderRiskPct, defaultRisk.MaxOrderRiskPct),
		MaxSymbolExposurePct:      maxSymbolExposurePct,
		MaxTotalExposurePct:       maxTotalExposurePct,
		MaxInitialMarginPct:       paperPositiveOrDefault(config.MaxMarginPct, defaultRisk.MaxInitialMarginPct),
		MaxAvailableBalanceUsePct: paperNonNegativeOrDefault(config.MaxBalanceUsePct, defaultRisk.MaxAvailableBalanceUsePct),
		MaxLeverage:               paperPositiveOrDefault(config.MaxLeverage, defaultRisk.MaxLeverage),
		MaxDailyLossPct:           defaultRisk.MaxDailyLossPct,
		MaxConsecutiveLosses:      defaultRisk.MaxConsecutiveLosses,
		MinLiquidationDistancePct: paperNonNegativeOrDefault(config.MinLiqDistancePct, defaultRisk.MinLiquidationDistancePct),
		MaintenanceMarginRatePct:  paperNonNegativeOrDefault(config.MaintMarginPct, defaultRisk.MaintenanceMarginRatePct),
		MaxAbsFundingRatePct:      defaultRisk.MaxAbsFundingRatePct,
	}
}

func paperRiskAccountSnapshot(config paperRunConfig, state paperAccountState) risk.AccountSnapshot {
	equity := state.Equity
	if equity <= 0 {
		equity = config.Equity
	}
	availableBalance := state.AvailableBalance
	if availableBalance <= 0 {
		availableBalance = equity
	}
	return risk.AccountSnapshot{
		AccountID:             config.AccountID,
		Equity:                equity,
		AvailableBalance:      availableBalance,
		CurrentTotalExposure:  state.CurrentExposure,
		CurrentSymbolExposure: state.CurrentSymbolExposure,
		CurrentInitialMargin:  state.CurrentInitialMargin,
		CurrentMaintMargin:    state.CurrentMaintMargin,
		SnapshotTime:          time.Now().UTC(),
	}
}

func paperPositionNotional(position portfolio.PaperPositionRecord, markPrice float64) float64 {
	if position.ID == 0 || markPrice <= 0 {
		return 0
	}
	return math.Abs(position.Quantity * markPrice)
}

func paperInitialMargin(notional float64, leverage float64) float64 {
	if notional <= 0 {
		return 0
	}
	return notional / math.Max(leverage, 1)
}

func paperMaintenanceMargin(notional float64, maintenanceMarginPct float64) float64 {
	if notional <= 0 || maintenanceMarginPct <= 0 {
		return 0
	}
	return notional * maintenanceMarginPct / 100
}

func paperAvailableBalance(equity float64, initialMargin float64) float64 {
	available := equity - initialMargin
	if available < 0 {
		return 0
	}
	return available
}

func paperLiquidationPrice(positionSide string, entryPrice float64, leverage float64, maintenanceMarginPct float64) float64 {
	side := risk.SideBuy
	if strings.EqualFold(positionSide, "short") {
		side = risk.SideSell
	}
	return risk.EstimateLiquidationPrice(entryPrice, side, leverage, maintenanceMarginPct)
}

func paperPositiveOrDefault(value float64, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func paperNonNegativeOrDefault(value float64, fallback float64) float64 {
	if value >= 0 {
		return value
	}
	return fallback
}

func paperPositionPnL(position portfolio.PaperPositionRecord, markPrice float64) float64 {
	if strings.EqualFold(position.PositionSide, "short") {
		return (position.EntryPrice - markPrice) * math.Abs(position.Quantity)
	}
	return (markPrice - position.EntryPrice) * math.Abs(position.Quantity)
}

func paperPositionNetPnL(position portfolio.PaperPositionRecord, markPrice float64, feeRate float64, slippageRate float64) float64 {
	grossPnL := paperPositionPnL(position, markPrice)
	costRate := math.Max(feeRate, 0) + math.Max(slippageRate, 0)
	if costRate == 0 {
		return grossPnL
	}
	entryCost := paperTradeCost(position.EntryPrice, position.Quantity, costRate)
	exitCost := paperTradeCost(markPrice, position.Quantity, costRate)
	return grossPnL - entryCost - exitCost
}

func paperTradeCost(price float64, quantity float64, costRate float64) float64 {
	if price <= 0 || quantity == 0 || costRate <= 0 {
		return 0
	}
	return math.Abs(price*quantity) * costRate
}

func latestCandlePrice(candles []marketdata.Candle) (float64, time.Time, error) {
	if len(candles) == 0 {
		return 0, time.Time{}, backtest.ErrNotEnoughData
	}
	last := candles[len(candles)-1]
	price, err := strconv.ParseFloat(last.Close, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("parse latest close price: %w", err)
	}
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, time.Time{}, fmt.Errorf("invalid latest close price %s", last.Close)
	}
	return price, last.OpenTime, nil
}

func paperLookbackDuration(interval string, candles int) time.Duration {
	if candles <= 0 {
		candles = 120
	}
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return 2 * time.Hour
	}
	return step * time.Duration(candles)
}

func runPaperBacktest(candles []marketdata.Candle, config paperStrategyConfig) (backtest.Result, error) {
	switch normalizedStrategyType(config.StrategyType) {
	case "scalp-tpsl":
		return backtest.RunScalpTPSL(candles, backtest.ScalpTPSLConfig{
			FastWindow:           config.FastWindow,
			SlowWindow:           config.SlowWindow,
			TakeProfitPct:        config.TakeProfitPct,
			StopLossPct:          config.StopLossPct,
			CooldownBars:         config.CooldownBars,
			FeeRate:              config.FeeRate,
			SlippageRate:         config.SlippageRate,
			AllowShort:           strings.EqualFold(config.MarketType, "perpetual"),
			MinTrendSpreadPct:    config.MinTrendSpreadPct,
			ConfirmBars:          config.ConfirmBars,
			ATRWindow:            config.ATRWindow,
			MinATRPct:            config.MinATRPct,
			MaxATRPct:            config.MaxATRPct,
			VolumeWindow:         config.VolumeWindow,
			MinVolumeRatio:       config.MinVolumeRatio,
			MaxEntryExtensionPct: config.MaxEntryExtensionPct,
			PullbackLookback:     config.PullbackLookback,
			PullbackTolerancePct: config.PullbackTolerancePct,
		})
	case "sma":
		return backtest.RunSMACrossover(candles, backtest.SMAConfig{
			FastWindow: config.FastWindow,
			SlowWindow: config.SlowWindow,
			FeeRate:    config.FeeRate,
		})
	default:
		return backtest.Result{}, fmt.Errorf("unsupported strategy type %q", config.StrategyType)
	}
}

func paperBacktestFeeRate(strategyType string, feeRate float64, slippageRate float64) float64 {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		return feeRate + slippageRate
	}
	return feeRate
}

type paperSignal struct {
	Action       strategy.SignalAction
	PositionSide string
}

func paperSignalAction(candles []marketdata.Candle, result backtest.Result, config paperStrategyConfig) paperSignal {
	switch normalizedStrategyType(config.StrategyType) {
	case "scalp-tpsl":
		return latestScalpSignal(candles, config)
	default:
		if result.TotalReturnPct > 0 {
			return paperSignal{Action: strategy.SignalBuy, PositionSide: "long"}
		}
		return paperSignal{Action: strategy.SignalHold}
	}
}

func latestScalpSignal(candles []marketdata.Candle, config paperStrategyConfig) paperSignal {
	side, ok, err := backtest.LatestScalpTPSLSignal(candles, backtest.ScalpTPSLConfig{
		FastWindow:           config.FastWindow,
		SlowWindow:           config.SlowWindow,
		TakeProfitPct:        config.TakeProfitPct,
		StopLossPct:          config.StopLossPct,
		CooldownBars:         config.CooldownBars,
		FeeRate:              config.FeeRate,
		SlippageRate:         config.SlippageRate,
		AllowShort:           strings.EqualFold(config.MarketType, "perpetual"),
		MinTrendSpreadPct:    config.MinTrendSpreadPct,
		ConfirmBars:          config.ConfirmBars,
		ATRWindow:            config.ATRWindow,
		MinATRPct:            config.MinATRPct,
		MaxATRPct:            config.MaxATRPct,
		VolumeWindow:         config.VolumeWindow,
		MinVolumeRatio:       config.MinVolumeRatio,
		MaxEntryExtensionPct: config.MaxEntryExtensionPct,
		PullbackLookback:     config.PullbackLookback,
		PullbackTolerancePct: config.PullbackTolerancePct,
	})
	if err != nil || !ok {
		return paperSignal{Action: strategy.SignalHold}
	}
	switch side {
	case "long":
		return paperSignal{Action: strategy.SignalBuy, PositionSide: "long"}
	case "short":
		return paperSignal{Action: strategy.SignalShort, PositionSide: "short"}
	}
	return paperSignal{Action: strategy.SignalHold}
}

func latestAverages(candles []marketdata.Candle, fastWindow int, slowWindow int) (float64, float64, bool) {
	if fastWindow <= 0 || slowWindow <= 0 || fastWindow >= slowWindow || len(candles) < slowWindow {
		return 0, 0, false
	}
	closes := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil || closePrice <= 0 || math.IsNaN(closePrice) || math.IsInf(closePrice, 0) {
			return 0, 0, false
		}
		closes = append(closes, closePrice)
	}
	latest := len(closes) - 1
	fastAverage := average(closes[latest-fastWindow+1 : latest+1])
	slowAverage := average(closes[latest-slowWindow+1 : latest+1])
	return fastAverage, slowAverage, true
}

func average(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func normalizedStrategyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "scalp", "scalping", "scalp-tpsl", "tpsl":
		return "scalp-tpsl"
	default:
		return "sma"
	}
}

func paperSignalReason(strategyType string) string {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		return "paper_scalp_tpsl_latest_momentum"
	}
	return "paper_sma_snapshot"
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func confidence(excess float64) float64 {
	if excess <= 0 {
		return 0.4
	}
	if excess >= 10 {
		return 0.9
	}
	return 0.5 + excess/25
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
