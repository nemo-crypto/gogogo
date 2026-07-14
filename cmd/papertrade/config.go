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
		setStringFlag(visited, "interval", flags.interval, "3m")
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

func setBoolFlag(visited map[string]struct{}, name string, target *bool, value bool) {
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
