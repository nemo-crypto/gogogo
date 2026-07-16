package main

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

type paperProfileFlags struct {
	strategyID    *string
	market        *string
	interval      *string
	strategyType  *string
	fast          *int
	slow          *int
	takeProfitPct *float64
	stopLossPct   *float64
	dynamicTPSL   *bool
	takeATRMult   *float64
	stopATRMult   *float64
	minTPPct      *float64
	maxTPPct      *float64
	minSLPct      *float64
	maxSLPct      *float64
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
	signalFilter  *bool
	minSignal     *float64
	trendFilter   *bool
	trendInterval *string
	macroInterval *string
	trendFast     *int
	trendSlow     *int
	trendMin      *float64
	maxCandleAge  *time.Duration
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
		setStringFlag(visited, "strategy", flags.strategyID, defaultPaperStrategyID)
		setStringFlag(visited, "market", flags.market, "perpetual")
		setStringFlag(visited, "interval", flags.interval, "5m")
		setStringFlag(visited, "strategy-type", flags.strategyType, "scalp-tpsl")
		setIntFlag(visited, "fast", flags.fast, defaultPaperFastWindow)
		setIntFlag(visited, "slow", flags.slow, defaultPaperSlowWindow)
		setFloatFlag(visited, "take-profit-pct", flags.takeProfitPct, defaultPaperTakeProfitPct)
		setFloatFlag(visited, "stop-loss-pct", flags.stopLossPct, defaultPaperStopLossPct)
		setBoolFlag(visited, "dynamic-tpsl", flags.dynamicTPSL, true)
		setFloatFlag(visited, "take-profit-atr-mult", flags.takeATRMult, 1.8)
		setFloatFlag(visited, "stop-loss-atr-mult", flags.stopATRMult, 1.0)
		setFloatFlag(visited, "min-take-profit-pct", flags.minTPPct, 0.60)
		setFloatFlag(visited, "max-take-profit-pct", flags.maxTPPct, 1.60)
		setFloatFlag(visited, "min-stop-loss-pct", flags.minSLPct, defaultPaperMinStopLoss)
		setFloatFlag(visited, "max-stop-loss-pct", flags.maxSLPct, defaultPaperMaxStopLoss)
		setIntFlag(visited, "cooldown-bars", flags.cooldownBars, defaultPaperCooldownBars)
		setFloatFlag(visited, "min-trend-spread-pct", flags.minSpreadPct, defaultPaperMinTrendSpread)
		setIntFlag(visited, "confirm-bars", flags.confirmBars, 1)
		setIntFlag(visited, "atr-window", flags.atrWindow, defaultPaperATRWindow)
		setFloatFlag(visited, "min-atr-pct", flags.minATRPct, defaultPaperMinATRPct)
		setFloatFlag(visited, "max-atr-pct", flags.maxATRPct, defaultPaperMaxATRPct)
		setIntFlag(visited, "volume-window", flags.volumeWindow, defaultPaperVolumeWindow)
		setFloatFlag(visited, "min-volume-ratio", flags.minVolume, 1.15)
		setFloatFlag(visited, "max-entry-extension-pct", flags.maxExtension, defaultPaperMaxExtensionPct)
		setIntFlag(visited, "pullback-lookback", flags.pullbackBars, defaultPaperPullbackBars)
		setFloatFlag(visited, "pullback-tolerance-pct", flags.pullbackTol, defaultPaperPullbackTolPct)
		setFloatFlag(visited, "fee-rate", flags.feeRate, defaultPaperFeeRate)
		setFloatFlag(visited, "slippage-rate", flags.slippageRate, defaultPaperSlippageRate)
		setFloatFlag(visited, "risk-pct", flags.riskPct, 2)
		setFloatFlag(visited, "max-notional-pct", flags.maxNotional, 220)
		setFloatFlag(visited, "max-margin-pct", flags.maxMargin, 65)
		setFloatFlag(visited, "max-balance-use-pct", flags.maxBalanceUse, 90)
		setFloatFlag(visited, "min-liquidation-distance-pct", flags.minLiqDist, 15)
		setFloatFlag(visited, "max-order-risk-pct", flags.maxOrderRisk, 2.5)
		setFloatFlag(visited, "max-leverage", flags.maxLeverage, 3)
		setFloatFlag(visited, "leverage", flags.leverage, 3)
		setBoolFlag(visited, "signal-filter", flags.signalFilter, true)
		setFloatFlag(visited, "min-signal-score", flags.minSignal, 0.50)
		setBoolFlag(visited, "trend-filter", flags.trendFilter, true)
		setStringFlag(visited, "trend-interval", flags.trendInterval, defaultPaperTrendInterval)
		setStringFlag(visited, "macro-trend-interval", flags.macroInterval, defaultPaperMacroInterval)
		setIntFlag(visited, "trend-fast", flags.trendFast, defaultPaperTrendFastWindow)
		setIntFlag(visited, "trend-slow", flags.trendSlow, defaultPaperTrendSlowWindow)
		setFloatFlag(visited, "trend-min-spread-pct", flags.trendMin, defaultPaperTrendMinSpread)
		return nil
	case "small-scalp":
		applySmallScalpProfile(visited, flags, false)
		return nil
	case "small-scalp-fast":
		applySmallScalpProfile(visited, flags, true)
		return nil
	case "micro-trend-1m":
		applyMicroTrend1MProfile(visited, flags)
		return nil
	default:
		return fmt.Errorf("unsupported paper profile %q", profile)
	}
}

func applyMicroTrend1MProfile(visited map[string]struct{}, flags paperProfileFlags) {
	setStringFlag(visited, "strategy", flags.strategyID, defaultPaperStrategyID)
	setStringFlag(visited, "market", flags.market, "perpetual")
	setStringFlag(visited, "interval", flags.interval, "1m")
	setStringFlag(visited, "strategy-type", flags.strategyType, "scalp-tpsl")
	setIntFlag(visited, "fast", flags.fast, 2)
	setIntFlag(visited, "slow", flags.slow, 5)
	setFloatFlag(visited, "take-profit-pct", flags.takeProfitPct, 0.35)
	setFloatFlag(visited, "stop-loss-pct", flags.stopLossPct, 0.22)
	setBoolFlag(visited, "dynamic-tpsl", flags.dynamicTPSL, true)
	setFloatFlag(visited, "take-profit-atr-mult", flags.takeATRMult, 1.10)
	setFloatFlag(visited, "stop-loss-atr-mult", flags.stopATRMult, 0.75)
	setFloatFlag(visited, "min-take-profit-pct", flags.minTPPct, 0.25)
	setFloatFlag(visited, "max-take-profit-pct", flags.maxTPPct, 0.80)
	setFloatFlag(visited, "min-stop-loss-pct", flags.minSLPct, 0.15)
	setFloatFlag(visited, "max-stop-loss-pct", flags.maxSLPct, 0.45)
	setIntFlag(visited, "cooldown-bars", flags.cooldownBars, 0)
	setFloatFlag(visited, "min-trend-spread-pct", flags.minSpreadPct, 0.004)
	setIntFlag(visited, "confirm-bars", flags.confirmBars, 1)
	setIntFlag(visited, "atr-window", flags.atrWindow, defaultPaperATRWindow)
	setFloatFlag(visited, "min-atr-pct", flags.minATRPct, 0.02)
	setFloatFlag(visited, "max-atr-pct", flags.maxATRPct, defaultPaperMaxATRPct)
	setIntFlag(visited, "volume-window", flags.volumeWindow, defaultPaperVolumeWindow)
	setFloatFlag(visited, "min-volume-ratio", flags.minVolume, 0.50)
	setFloatFlag(visited, "max-entry-extension-pct", flags.maxExtension, 0.50)
	setIntFlag(visited, "pullback-lookback", flags.pullbackBars, 1)
	setFloatFlag(visited, "pullback-tolerance-pct", flags.pullbackTol, 0.10)
	setFloatFlag(visited, "fee-rate", flags.feeRate, defaultPaperFeeRate)
	setFloatFlag(visited, "slippage-rate", flags.slippageRate, defaultPaperSlippageRate)
	setFloatFlag(visited, "risk-pct", flags.riskPct, 0.50)
	setFloatFlag(visited, "max-notional-pct", flags.maxNotional, 150)
	setFloatFlag(visited, "max-margin-pct", flags.maxMargin, 45)
	setFloatFlag(visited, "max-balance-use-pct", flags.maxBalanceUse, 75)
	setFloatFlag(visited, "min-liquidation-distance-pct", flags.minLiqDist, 15)
	setFloatFlag(visited, "max-order-risk-pct", flags.maxOrderRisk, 0.80)
	setFloatFlag(visited, "max-leverage", flags.maxLeverage, 3)
	setFloatFlag(visited, "leverage", flags.leverage, 3)
	setBoolFlag(visited, "signal-filter", flags.signalFilter, true)
	setFloatFlag(visited, "min-signal-score", flags.minSignal, 0.35)
	setBoolFlag(visited, "trend-filter", flags.trendFilter, true)
	setStringFlag(visited, "trend-interval", flags.trendInterval, "5m")
	setStringFlag(visited, "macro-trend-interval", flags.macroInterval, "5m")
	setIntFlag(visited, "trend-fast", flags.trendFast, 8)
	setIntFlag(visited, "trend-slow", flags.trendSlow, 21)
	setFloatFlag(visited, "trend-min-spread-pct", flags.trendMin, 0.005)
	setDurationFlag(visited, "max-candle-age", flags.maxCandleAge, 2*time.Minute)
}

func applySmallScalpProfile(visited map[string]struct{}, flags paperProfileFlags, fastMode bool) {
	minSpreadPct := 0.015
	minVolumeRatio := 1.00
	maxEntryExtensionPct := 0.22
	pullbackLookback := 3
	minSignalScore := 0.45
	trendMinSpreadPct := 0.02
	if fastMode {
		minSpreadPct = 0.008
		minVolumeRatio = 0.80
		maxEntryExtensionPct = 0.35
		pullbackLookback = 2
		minSignalScore = 0.40
		trendMinSpreadPct = 0.01
	}

	setStringFlag(visited, "strategy", flags.strategyID, defaultPaperStrategyID)
	setStringFlag(visited, "market", flags.market, "perpetual")
	setStringFlag(visited, "interval", flags.interval, "5m")
	setStringFlag(visited, "strategy-type", flags.strategyType, "scalp-tpsl")
	setIntFlag(visited, "fast", flags.fast, defaultPaperFastWindow)
	setIntFlag(visited, "slow", flags.slow, defaultPaperSlowWindow)
	setFloatFlag(visited, "take-profit-pct", flags.takeProfitPct, 0.65)
	setFloatFlag(visited, "stop-loss-pct", flags.stopLossPct, 0.40)
	setBoolFlag(visited, "dynamic-tpsl", flags.dynamicTPSL, true)
	setFloatFlag(visited, "take-profit-atr-mult", flags.takeATRMult, 1.35)
	setFloatFlag(visited, "stop-loss-atr-mult", flags.stopATRMult, 0.90)
	setFloatFlag(visited, "min-take-profit-pct", flags.minTPPct, 0.45)
	setFloatFlag(visited, "max-take-profit-pct", flags.maxTPPct, 1.20)
	setFloatFlag(visited, "min-stop-loss-pct", flags.minSLPct, 0.25)
	setFloatFlag(visited, "max-stop-loss-pct", flags.maxSLPct, 0.65)
	setIntFlag(visited, "cooldown-bars", flags.cooldownBars, defaultPaperCooldownBars)
	setFloatFlag(visited, "min-trend-spread-pct", flags.minSpreadPct, minSpreadPct)
	setIntFlag(visited, "confirm-bars", flags.confirmBars, 1)
	setIntFlag(visited, "atr-window", flags.atrWindow, defaultPaperATRWindow)
	setFloatFlag(visited, "min-atr-pct", flags.minATRPct, 0.05)
	setFloatFlag(visited, "max-atr-pct", flags.maxATRPct, defaultPaperMaxATRPct)
	setIntFlag(visited, "volume-window", flags.volumeWindow, defaultPaperVolumeWindow)
	setFloatFlag(visited, "min-volume-ratio", flags.minVolume, minVolumeRatio)
	setFloatFlag(visited, "max-entry-extension-pct", flags.maxExtension, maxEntryExtensionPct)
	setIntFlag(visited, "pullback-lookback", flags.pullbackBars, pullbackLookback)
	setFloatFlag(visited, "pullback-tolerance-pct", flags.pullbackTol, 0.08)
	setFloatFlag(visited, "fee-rate", flags.feeRate, defaultPaperFeeRate)
	setFloatFlag(visited, "slippage-rate", flags.slippageRate, defaultPaperSlippageRate)
	setFloatFlag(visited, "risk-pct", flags.riskPct, 0.80)
	setFloatFlag(visited, "max-notional-pct", flags.maxNotional, 150)
	setFloatFlag(visited, "max-margin-pct", flags.maxMargin, 45)
	setFloatFlag(visited, "max-balance-use-pct", flags.maxBalanceUse, 75)
	setFloatFlag(visited, "min-liquidation-distance-pct", flags.minLiqDist, 15)
	setFloatFlag(visited, "max-order-risk-pct", flags.maxOrderRisk, 1.20)
	setFloatFlag(visited, "max-leverage", flags.maxLeverage, 3)
	setFloatFlag(visited, "leverage", flags.leverage, 3)
	setBoolFlag(visited, "signal-filter", flags.signalFilter, true)
	setFloatFlag(visited, "min-signal-score", flags.minSignal, minSignalScore)
	setBoolFlag(visited, "trend-filter", flags.trendFilter, true)
	setStringFlag(visited, "trend-interval", flags.trendInterval, "15m")
	setStringFlag(visited, "macro-trend-interval", flags.macroInterval, "15m")
	setIntFlag(visited, "trend-fast", flags.trendFast, 8)
	setIntFlag(visited, "trend-slow", flags.trendSlow, 21)
	setFloatFlag(visited, "trend-min-spread-pct", flags.trendMin, trendMinSpreadPct)
}

func normalizedPaperProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", "manual", "none", "default":
		return ""
	case "aggressive", "small-aggressive", "small_aggressive":
		return "aggressive"
	case "small", "small-scalp", "small_scalp", "small-capital", "small_capital", "micro-scalp", "micro_scalp":
		return "small-scalp"
	case "small-scalp-fast", "small_scalp_fast", "small-fast", "small_fast", "fast-scalp", "fast_scalp", "micro-scalp-fast", "micro_scalp_fast", "300u", "300u-fast", "300u_fast":
		return "small-scalp-fast"
	case "micro", "micro-trend", "micro_trend", "micro-trend-1m", "micro_trend_1m", "micro-1m", "micro_1m", "one-min", "one_min", "one-minute", "one_minute", "1m", "1m-scalp", "1m_scalp", "scalp-1m", "scalp_1m", "minute-scalp", "minute_scalp":
		return "micro-trend-1m"
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

func setBoolFlag(visited map[string]struct{}, name string, target *bool, value bool) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setDurationFlag(visited map[string]struct{}, name string, target *time.Duration, value time.Duration) {
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
	PositionModel        string
	Symbol               string
	Interval             string
	Start                time.Time
	End                  time.Time
	StrategyType         string
	FastWindow           int
	SlowWindow           int
	TakeProfitPct        float64
	StopLossPct          float64
	DynamicTPSL          bool
	TakeProfitATRMult    float64
	StopLossATRMult      float64
	MinTakeProfitPct     float64
	MaxTakeProfitPct     float64
	MinStopLossPct       float64
	MaxStopLossPct       float64
	BreakevenStopEnabled bool
	BreakevenTriggerR    float64
	TrailingStopEnabled  bool
	TrailingActivationR  float64
	TrailingATRMult      float64
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
	MaxDailyLossPct      float64
	MaxConsecutiveLosses int
	MaxAbsFundingRatePct float64
	MaxLeverage          float64
	Leverage             float64
	SignalFilterEnabled  bool
	MinSignalScore       float64
	TrendFilterEnabled   bool
	TrendInterval        string
	MacroTrendInterval   string
	TrendFastWindow      int
	TrendSlowWindow      int
	TrendMinSpreadPct    float64
	MaxMarketDataAge     time.Duration
	MaxCandleAge         time.Duration
	MaxMarkPriceAge      time.Duration
	RequireMarkPrice     bool
	MinOrderQuantity     float64
	QuantityStep         float64
	PriceTickSize        float64
	SubmitExchange       bool
	Watch                bool
	PollInterval         time.Duration
	PersistInterval      time.Duration
	BacktestInterval     time.Duration
	LookbackCandles      int
}

type paperMarketSnapshot struct {
	Price                float64
	PriceTime            time.Time
	PriceSource          string
	CandleClosePrice     float64
	CandleCloseTime      time.Time
	MarkPrice            float64
	MarkPriceTime        time.Time
	LatestFundingRatePct float64
	FundingRateTime      time.Time
	FundingRateSource    string
}
