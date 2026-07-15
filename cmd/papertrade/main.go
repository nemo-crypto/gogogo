package main

import (
	"context"
	"flag"
	"fmt"
	"gogogo/internal/exchange/onebullex"
	"gogogo/internal/marketdata"
	"gogogo/internal/risk"
	"gogogo/internal/storage"
	"log"
	"strings"
	"time"
)

const (
	defaultPaperStrategyID      = "perp-trend-scalp-v2-paper"
	defaultPaperInterval        = "5m"
	defaultPaperFastWindow      = 3
	defaultPaperSlowWindow      = 9
	defaultPaperTakeProfitPct   = 0.80
	defaultPaperStopLossPct     = 0.45
	defaultPaperDynamicTPSL     = true
	defaultPaperTakeATRMult     = 1.6
	defaultPaperStopATRMult     = 1.0
	defaultPaperMinTakeProfit   = 0.55
	defaultPaperMaxTakeProfit   = 1.40
	defaultPaperMinStopLoss     = 0.30
	defaultPaperMaxStopLoss     = 0.75
	defaultPaperCooldownBars    = 1
	defaultPaperFeeRate         = 0.0005
	defaultPaperSlippageRate    = 0.0005
	defaultPaperRiskPct         = 1
	defaultPaperQuantity        = 0.001
	defaultPaperMaxDataAge      = 0
	defaultPaperMaxCandleAge    = 7 * time.Minute
	defaultPaperMaxMarkAge      = 45 * time.Second
	defaultPaperMinQuantity     = 0.001
	defaultPaperQuantityStep    = 0.001
	defaultPaperPriceTickSize   = 0.1
	defaultPaperMinTrendSpread  = 0.03
	defaultPaperATRWindow       = 14
	defaultPaperMinATRPct       = 0.08
	defaultPaperMaxATRPct       = 1.6
	defaultPaperVolumeWindow    = 20
	defaultPaperMinVolumeRatio  = 1.10
	defaultPaperMaxExtensionPct = 0.18
	defaultPaperPullbackBars    = 5
	defaultPaperPullbackTolPct  = 0.06
	defaultPaperSignalFilter    = true
	defaultPaperMinSignalScore  = 0.55
	defaultPaperTrendFilter     = true
	defaultPaperTrendInterval   = "15m"
	defaultPaperMacroInterval   = "1h"
	defaultPaperTrendFastWindow = 20
	defaultPaperTrendSlowWindow = 60
	defaultPaperTrendMinSpread  = 0.05
	defaultPaperBreakevenStop   = true
	defaultPaperBreakevenR      = 1.0
	defaultPaperTrailingStop    = true
	defaultPaperTrailingR       = 1.5
	defaultPaperTrailingATRMult = 1.2
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dsn            = flag.String("dsn", env("DATABASE_DSN", "data.db"), "sqlite database path")
		accountID      = flag.String("account", "paper", "paper account id")
		strategyID     = flag.String("strategy", defaultPaperStrategyID, "strategy id")
		profile        = flag.String("profile", "", "paper strategy profile: aggressive or empty/manual")
		exchange       = flag.String("exchange", env("EXCHANGE_NAME", onebullex.ExchangeName), "exchange")
		market         = flag.String("market", "perpetual", "market type")
		positionModel  = flag.String("position-model", "", "exchange position model: AGGREGATION for one-way or DISAGGREGATION for hedge")
		symbol         = flag.String("symbol", "BTCUSDT", "symbol")
		interval       = flag.String("interval", defaultPaperInterval, "interval")
		start          = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time")
		end            = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time")
		strategyType   = flag.String("strategy-type", "scalp-tpsl", "strategy type: scalp-tpsl or sma")
		fast           = flag.Int("fast", defaultPaperFastWindow, "fast SMA")
		slow           = flag.Int("slow", defaultPaperSlowWindow, "slow SMA")
		takeProfitPct  = flag.Float64("take-profit-pct", defaultPaperTakeProfitPct, "take profit pct for scalp-tpsl")
		stopLossPct    = flag.Float64("stop-loss-pct", defaultPaperStopLossPct, "stop loss pct for scalp-tpsl")
		dynamicTPSL    = flag.Bool("dynamic-tpsl", defaultPaperDynamicTPSL, "use ATR-based dynamic take profit and stop loss")
		takeATRMult    = flag.Float64("take-profit-atr-mult", defaultPaperTakeATRMult, "ATR multiplier for dynamic take profit")
		stopATRMult    = flag.Float64("stop-loss-atr-mult", defaultPaperStopATRMult, "ATR multiplier for dynamic stop loss")
		minTPPct       = flag.Float64("min-take-profit-pct", defaultPaperMinTakeProfit, "minimum dynamic take profit pct")
		maxTPPct       = flag.Float64("max-take-profit-pct", defaultPaperMaxTakeProfit, "maximum dynamic take profit pct; 0 disables cap")
		minSLPct       = flag.Float64("min-stop-loss-pct", defaultPaperMinStopLoss, "minimum dynamic stop loss pct")
		maxSLPct       = flag.Float64("max-stop-loss-pct", defaultPaperMaxStopLoss, "maximum dynamic stop loss pct; 0 disables cap")
		breakevenStop  = flag.Bool("breakeven-stop", defaultPaperBreakevenStop, "move stop loss to fee-adjusted breakeven after profit reaches trigger R")
		breakevenR     = flag.Float64("breakeven-trigger-r", defaultPaperBreakevenR, "R multiple that activates breakeven stop")
		trailingStop   = flag.Bool("trailing-stop", defaultPaperTrailingStop, "enable ATR trailing stop after profit reaches activation R")
		trailingR      = flag.Float64("trailing-activation-r", defaultPaperTrailingR, "R multiple that activates ATR trailing stop")
		trailingATR    = flag.Float64("trailing-atr-mult", defaultPaperTrailingATRMult, "ATR multiplier for trailing stop")
		cooldownBars   = flag.Int("cooldown-bars", defaultPaperCooldownBars, "cooldown bars after scalp-tpsl exit")
		minSpreadPct   = flag.Float64("min-trend-spread-pct", defaultPaperMinTrendSpread, "minimum SMA spread pct required to enter scalp-tpsl trades")
		confirmBars    = flag.Int("confirm-bars", 1, "consecutive close direction bars required to enter scalp-tpsl trades")
		atrWindow      = flag.Int("atr-window", defaultPaperATRWindow, "ATR window for scalp-tpsl volatility filter")
		minATRPct      = flag.Float64("min-atr-pct", defaultPaperMinATRPct, "minimum ATR pct required to enter scalp-tpsl trades")
		maxATRPct      = flag.Float64("max-atr-pct", defaultPaperMaxATRPct, "maximum ATR pct allowed to enter scalp-tpsl trades")
		volumeWindow   = flag.Int("volume-window", defaultPaperVolumeWindow, "volume average window for scalp-tpsl volume filter")
		minVolume      = flag.Float64("min-volume-ratio", defaultPaperMinVolumeRatio, "minimum current volume / average volume required to enter scalp-tpsl trades")
		maxExtension   = flag.Float64("max-entry-extension-pct", defaultPaperMaxExtensionPct, "maximum entry distance from fast SMA pct; zero disables")
		pullbackBars   = flag.Int("pullback-lookback", defaultPaperPullbackBars, "recent bars that must touch fast SMA zone before entry; zero disables")
		pullbackTol    = flag.Float64("pullback-tolerance-pct", defaultPaperPullbackTolPct, "pullback touch tolerance pct around fast SMA")
		feeRate        = flag.Float64("fee-rate", defaultPaperFeeRate, "fee rate per trade side")
		slippageRate   = flag.Float64("slippage-rate", defaultPaperSlippageRate, "slippage rate per trade side")
		equity         = flag.Float64("equity", 10000, "paper account equity")
		quantity       = flag.Float64("quantity", defaultPaperQuantity, "paper order quantity")
		riskPct        = flag.Float64("risk-pct", defaultPaperRiskPct, "risk pct of equity per trade; overrides -quantity when positive")
		maxNotional    = flag.Float64("max-notional-pct", 100, "maximum notional as pct of equity for paper order sizing")
		maxMargin      = flag.Float64("max-margin-pct", risk.DefaultConfig().MaxInitialMarginPct, "maximum total initial margin as pct of equity")
		maxBalanceUse  = flag.Float64("max-balance-use-pct", risk.DefaultConfig().MaxAvailableBalanceUsePct, "maximum order initial margin as pct of available balance")
		minLiqDist     = flag.Float64("min-liquidation-distance-pct", risk.DefaultConfig().MinLiquidationDistancePct, "minimum estimated liquidation distance pct")
		maintMargin    = flag.Float64("maintenance-margin-rate-pct", risk.DefaultConfig().MaintenanceMarginRatePct, "maintenance margin rate pct used for paper liquidation estimate")
		maxOrderRisk   = flag.Float64("max-order-risk-pct", risk.DefaultConfig().MaxOrderRiskPct, "maximum order stop loss risk as pct of equity")
		maxDailyLoss   = flag.Float64("max-daily-loss-pct", risk.DefaultConfig().MaxDailyLossPct, "daily realized loss pct that halts new entries")
		maxLosses      = flag.Int("max-consecutive-losses", risk.DefaultConfig().MaxConsecutiveLosses, "consecutive losing paper positions that halt new entries")
		maxFunding     = flag.Float64("max-abs-funding-rate-pct", risk.DefaultConfig().MaxAbsFundingRatePct, "maximum absolute funding rate pct allowed for new perpetual entries")
		maxLeverage    = flag.Float64("max-leverage", risk.DefaultConfig().MaxLeverage, "maximum allowed paper leverage")
		leverage       = flag.Float64("leverage", 1, "paper position leverage")
		signalFilter   = flag.Bool("signal-filter", defaultPaperSignalFilter, "filter low-quality entry signals using a feature score")
		minSignal      = flag.Float64("min-signal-score", defaultPaperMinSignalScore, "minimum 0-1 signal quality score required for new entries")
		trendFilter    = flag.Bool("trend-filter", defaultPaperTrendFilter, "filter new entries by higher-timeframe trend regime")
		trendInterval  = flag.String("trend-interval", defaultPaperTrendInterval, "higher-timeframe trend interval")
		macroInterval  = flag.String("macro-trend-interval", defaultPaperMacroInterval, "macro trend confirmation interval")
		trendFast      = flag.Int("trend-fast", defaultPaperTrendFastWindow, "fast EMA window for higher-timeframe trend")
		trendSlow      = flag.Int("trend-slow", defaultPaperTrendSlowWindow, "slow EMA window for higher-timeframe trend")
		trendMinSpread = flag.Float64("trend-min-spread-pct", defaultPaperTrendMinSpread, "minimum higher-timeframe EMA spread pct required to allow new entries")
		maxDataAge     = flag.Duration("max-market-data-age", defaultPaperMaxDataAge, "legacy maximum age for latest candle and mark price; 0 uses split candle/mark limits")
		maxCandleAge   = flag.Duration("max-candle-age", defaultPaperMaxCandleAge, "maximum allowed age for latest closed signal candle; 0 disables candle freshness checks")
		maxMarkAge     = flag.Duration("max-mark-price-age", defaultPaperMaxMarkAge, "maximum allowed age for latest mark price; 0 disables mark freshness checks")
		requireMark    = flag.Bool("require-mark-price", true, "require latest OneBullEx mark price before paper settlement or entry")
		minQuantity    = flag.Float64("min-order-quantity", defaultPaperMinQuantity, "minimum order quantity before opening paper positions")
		quantityStep   = flag.Float64("quantity-step", defaultPaperQuantityStep, "quantity step used to round paper order size down; 0 disables")
		priceTick      = flag.Float64("price-tick-size", defaultPaperPriceTickSize, "price tick size used to align paper entry/TP/SL prices; 0 disables")
		submitExchange = flag.Bool("submit-exchange", false, "submit allowed open/close orders to OneBullEx; also requires ONEBULLEX_LIVE_TRADING=true")
		watch          = flag.Bool("watch", false, "keep running paper strategy on latest local market data")
		pollEvery      = flag.Duration("poll-interval", 15*time.Second, "poll interval when -watch is enabled")
		persistEvery   = flag.Duration("persist-interval", time.Minute, "minimum interval for persisting hold signals/account snapshots in watch mode; new candles and orders always persist")
		backtestEvery  = flag.Duration("backtest-interval", 5*time.Minute, "minimum interval for saving backtest_runs in watch mode; 0 saves every tick")
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
		dynamicTPSL:   dynamicTPSL,
		takeATRMult:   takeATRMult,
		stopATRMult:   stopATRMult,
		minTPPct:      minTPPct,
		maxTPPct:      maxTPPct,
		minSLPct:      minSLPct,
		maxSLPct:      maxSLPct,
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
		signalFilter:  signalFilter,
		minSignal:     minSignal,
	}); err != nil {
		return err
	}
	actualStrategyID := *strategyID
	if (actualStrategyID == "sma-paper" || actualStrategyID == "scalp-tpsl-paper") && strings.EqualFold(*strategyType, "scalp-tpsl") {
		actualStrategyID = defaultPaperStrategyID
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
		Exchange:             normalizeExchangeName(*exchange),
		MarketType:           *market,
		PositionModel:        *positionModel,
		Symbol:               *symbol,
		Interval:             *interval,
		Start:                startTime,
		End:                  endTime,
		StrategyType:         *strategyType,
		FastWindow:           *fast,
		SlowWindow:           *slow,
		TakeProfitPct:        *takeProfitPct,
		StopLossPct:          *stopLossPct,
		DynamicTPSL:          *dynamicTPSL,
		TakeProfitATRMult:    *takeATRMult,
		StopLossATRMult:      *stopATRMult,
		MinTakeProfitPct:     *minTPPct,
		MaxTakeProfitPct:     *maxTPPct,
		MinStopLossPct:       *minSLPct,
		MaxStopLossPct:       *maxSLPct,
		BreakevenStopEnabled: *breakevenStop,
		BreakevenTriggerR:    *breakevenR,
		TrailingStopEnabled:  *trailingStop,
		TrailingActivationR:  *trailingR,
		TrailingATRMult:      *trailingATR,
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
		MaxDailyLossPct:      *maxDailyLoss,
		MaxConsecutiveLosses: *maxLosses,
		MaxAbsFundingRatePct: *maxFunding,
		MaxLeverage:          *maxLeverage,
		Leverage:             *leverage,
		SignalFilterEnabled:  *signalFilter,
		MinSignalScore:       *minSignal,
		TrendFilterEnabled:   *trendFilter,
		TrendInterval:        *trendInterval,
		MacroTrendInterval:   *macroInterval,
		TrendFastWindow:      *trendFast,
		TrendSlowWindow:      *trendSlow,
		TrendMinSpreadPct:    *trendMinSpread,
		MaxMarketDataAge:     *maxDataAge,
		MaxCandleAge:         *maxCandleAge,
		MaxMarkPriceAge:      *maxMarkAge,
		RequireMarkPrice:     *requireMark,
		MinOrderQuantity:     *minQuantity,
		QuantityStep:         *quantityStep,
		PriceTickSize:        *priceTick,
		SubmitExchange:       *submitExchange,
		Watch:                *watch,
		PollInterval:         *pollEvery,
		PersistInterval:      *persistEvery,
		BacktestInterval:     *backtestEvery,
		LookbackCandles:      120,
	}
	if config.Exchange != onebullex.ExchangeName {
		return fmt.Errorf("paper strategy currently supports %s only", onebullex.ExchangeName)
	}
	if config.MinSignalScore < 0 || config.MinSignalScore > 1 {
		return fmt.Errorf("min signal score must be between 0 and 1")
	}
	if config.TrendFastWindow <= 0 || config.TrendSlowWindow <= 0 || config.TrendFastWindow >= config.TrendSlowWindow {
		return fmt.Errorf("trend windows must be positive and fast must be below slow")
	}

	if config.Watch {
		return watchPaperStrategy(context.Background(), db, config)
	}

	return runPaperStrategyOnce(ctx, db, config)
}
