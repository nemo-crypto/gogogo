package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gogogo/internal/backtest"
	"gogogo/internal/config"
	"gogogo/internal/exchange/onebullex"
	"gogogo/internal/marketdata"
	"gogogo/internal/strategy"
	"math"
	"strconv"
	"strings"
	"time"
)

func runPaperBacktest(candles []marketdata.Candle, config paperStrategyConfig) (backtest.Result, error) {
	switch normalizedStrategyType(config.StrategyType) {
	case "scalp-tpsl":
		return backtest.RunScalpTPSL(candles, paperBacktestScalpConfig(config))
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

func paperBacktestScalpConfig(config paperStrategyConfig) backtest.ScalpTPSLConfig {
	return backtest.ScalpTPSLConfig{
		FastWindow:           config.FastWindow,
		SlowWindow:           config.SlowWindow,
		TakeProfitPct:        config.TakeProfitPct,
		StopLossPct:          config.StopLossPct,
		DynamicTPSL:          config.DynamicTPSL,
		TakeProfitATRMult:    config.TakeProfitATRMult,
		StopLossATRMult:      config.StopLossATRMult,
		MinTakeProfitPct:     config.MinTakeProfitPct,
		MaxTakeProfitPct:     config.MaxTakeProfitPct,
		MinStopLossPct:       config.MinStopLossPct,
		MaxStopLossPct:       config.MaxStopLossPct,
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

type paperSignalAssessment struct {
	Score      float64
	AllowEntry bool
	Reason     string
	Features   map[string]any
}

type paperSignalFeatureValues struct {
	LatestClose       float64
	FastAverage       float64
	SlowAverage       float64
	TrendSpreadPct    float64
	ATRPct            float64
	VolumeRatio       float64
	EntryExtensionPct float64
	FundingAbsPct     float64
	MarkBasisPct      float64
	HasATR            bool
	HasVolume         bool
	HasMarkBasis      bool
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
	side, ok, err := backtest.LatestScalpTPSLSignal(candles, paperBacktestScalpConfig(config))
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

type paperTrendRegime struct {
	Enabled         bool
	Available       bool
	MacroAvailable  bool
	Regime          string
	Reason          string
	TrendInterval   string
	MacroInterval   string
	AllowLong       bool
	AllowShort      bool
	TrendSpreadPct  float64
	MacroSpreadPct  float64
	TrendSlopePct   float64
	MacroSlopePct   float64
	TrendCandleRows int
	MacroCandleRows int
}

type paperTrendFrame struct {
	Available   bool
	Direction   string
	Strong      bool
	Reason      string
	LatestClose float64
	FastEMA     float64
	SlowEMA     float64
	SpreadPct   float64
	SlopePct    float64
	CandleRows  int
}

func loadPaperTrendRegime(ctx context.Context, repo *marketdata.SQLiteRepository, config paperRunConfig) (paperTrendRegime, error) {
	regime := paperTrendRegime{
		Enabled:       config.TrendFilterEnabled,
		Regime:        "disabled",
		Reason:        "trend_filter_disabled",
		TrendInterval: strings.TrimSpace(config.TrendInterval),
		MacroInterval: strings.TrimSpace(config.MacroTrendInterval),
		AllowLong:     true,
		AllowShort:    true,
	}
	if !config.TrendFilterEnabled {
		return regime, nil
	}
	if regime.TrendInterval == "" {
		regime.TrendInterval = config.Interval
	}
	if regime.MacroInterval == "" {
		regime.MacroInterval = regime.TrendInterval
	}
	trendCandles, err := listPaperTrendCandles(ctx, repo, config, regime.TrendInterval)
	if err != nil {
		return paperTrendRegime{}, err
	}
	macroCandles := trendCandles
	if regime.MacroInterval != regime.TrendInterval {
		macroCandles, err = listPaperTrendCandles(ctx, repo, config, regime.MacroInterval)
		if err != nil {
			return paperTrendRegime{}, err
		}
	}
	return computePaperTrendRegime(trendCandles, macroCandles, config)
}

func listPaperTrendCandles(ctx context.Context, repo *marketdata.SQLiteRepository, config paperRunConfig, interval string) ([]marketdata.Candle, error) {
	if interval == "" {
		return nil, errors.New("trend interval is required")
	}
	lookbackStart, err := paperTrendLookbackStart(config.End, interval, config.TrendSlowWindow)
	if err != nil {
		return nil, err
	}
	limit := config.TrendSlowWindow * 3
	if limit < config.TrendSlowWindow+10 {
		limit = config.TrendSlowWindow + 10
	}
	candles, err := repo.ListCandles(ctx, marketdata.CandleQuery{
		Exchange:   config.Exchange,
		MarketType: marketdata.MarketType(config.MarketType),
		Symbol:     config.Symbol,
		Interval:   interval,
		Start:      lookbackStart,
		End:        config.End,
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list trend candles %s: %w", interval, err)
	}
	return closedPaperCandles(candles, config.End), nil
}

func paperTrendLookbackStart(end time.Time, interval string, slowWindow int) (time.Time, error) {
	if slowWindow <= 0 {
		return time.Time{}, errors.New("trend slow window must be positive")
	}
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return time.Time{}, fmt.Errorf("trend interval %q: %w", interval, err)
	}
	lookbackBars := slowWindow + 10
	return end.Add(-step * time.Duration(lookbackBars)), nil
}

func computePaperTrendRegime(trendCandles []marketdata.Candle, macroCandles []marketdata.Candle, config paperRunConfig) (paperTrendRegime, error) {
	regime := paperTrendRegime{
		Enabled:       config.TrendFilterEnabled,
		Regime:        "range",
		Reason:        "trend_filter_disabled",
		TrendInterval: config.TrendInterval,
		MacroInterval: config.MacroTrendInterval,
		AllowLong:     true,
		AllowShort:    true,
	}
	if !config.TrendFilterEnabled {
		regime.Regime = "disabled"
		return regime, nil
	}
	trendFrame, err := classifyPaperTrendFrame(trendCandles, config)
	if err != nil {
		return paperTrendRegime{}, err
	}
	macroFrame, err := classifyPaperTrendFrame(macroCandles, config)
	if err != nil {
		return paperTrendRegime{}, err
	}
	regime.Available = trendFrame.Available
	regime.MacroAvailable = macroFrame.Available
	regime.TrendSpreadPct = trendFrame.SpreadPct
	regime.MacroSpreadPct = macroFrame.SpreadPct
	regime.TrendSlopePct = trendFrame.SlopePct
	regime.MacroSlopePct = macroFrame.SlopePct
	regime.TrendCandleRows = trendFrame.CandleRows
	regime.MacroCandleRows = macroFrame.CandleRows
	if !trendFrame.Available || !macroFrame.Available {
		regime.AllowLong = false
		regime.AllowShort = false
		regime.Reason = "trend_data_unavailable"
		return regime, nil
	}
	if trendFrame.Direction == "range" || macroFrame.Direction == "range" {
		regime.AllowLong = false
		regime.AllowShort = false
		regime.Reason = "trend_range"
		return regime, nil
	}
	if trendFrame.Direction != macroFrame.Direction {
		regime.AllowLong = false
		regime.AllowShort = false
		regime.Reason = "trend_not_aligned"
		return regime, nil
	}
	switch trendFrame.Direction {
	case "up":
		regime.AllowLong = true
		regime.AllowShort = false
		if trendFrame.Strong && macroFrame.Strong {
			regime.Regime = "strong_up"
		} else {
			regime.Regime = "weak_up"
		}
		regime.Reason = "trend_allows_long"
	case "down":
		regime.AllowLong = false
		regime.AllowShort = true
		if trendFrame.Strong && macroFrame.Strong {
			regime.Regime = "strong_down"
		} else {
			regime.Regime = "weak_down"
		}
		regime.Reason = "trend_allows_short"
	default:
		regime.AllowLong = false
		regime.AllowShort = false
		regime.Reason = "trend_range"
	}
	return regime, nil
}

func classifyPaperTrendFrame(candles []marketdata.Candle, config paperRunConfig) (paperTrendFrame, error) {
	frame := paperTrendFrame{Direction: "range", Reason: "not_enough_trend_candles", CandleRows: len(candles)}
	if len(candles) < config.TrendSlowWindow+1 {
		return frame, nil
	}
	closes := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := parsePositiveFloat(candle.Close, "trend close")
		if err != nil {
			return paperTrendFrame{}, err
		}
		closes = append(closes, closePrice)
	}
	fastEMA, previousFastEMA, ok := paperEMA(closes, config.TrendFastWindow)
	if !ok {
		return frame, nil
	}
	slowEMA, _, ok := paperEMA(closes, config.TrendSlowWindow)
	if !ok {
		return frame, nil
	}
	latestClose := closes[len(closes)-1]
	spreadPct := (fastEMA - slowEMA) / latestClose * 100
	slopePct := (fastEMA - previousFastEMA) / latestClose * 100
	minSpreadPct := math.Max(config.TrendMinSpreadPct, 0)
	frame.Available = true
	frame.LatestClose = latestClose
	frame.FastEMA = fastEMA
	frame.SlowEMA = slowEMA
	frame.SpreadPct = spreadPct
	frame.SlopePct = slopePct
	frame.Reason = "trend_classified"
	if math.Abs(spreadPct) < minSpreadPct {
		frame.Direction = "range"
		frame.Reason = "trend_spread_too_small"
		return frame, nil
	}
	if spreadPct > 0 && slopePct >= 0 {
		frame.Direction = "up"
		frame.Strong = math.Abs(spreadPct) >= minSpreadPct*2
		return frame, nil
	}
	if spreadPct < 0 && slopePct <= 0 {
		frame.Direction = "down"
		frame.Strong = math.Abs(spreadPct) >= minSpreadPct*2
		return frame, nil
	}
	frame.Direction = "range"
	frame.Reason = "trend_slope_conflict"
	return frame, nil
}

func paperEMA(values []float64, window int) (float64, float64, bool) {
	if window <= 0 || len(values) < window+1 {
		return 0, 0, false
	}
	ema := 0.0
	for i := 0; i < window; i++ {
		ema += values[i]
	}
	ema /= float64(window)
	previous := ema
	alpha := 2.0 / float64(window+1)
	for i := window; i < len(values); i++ {
		previous = ema
		ema = values[i]*alpha + ema*(1-alpha)
	}
	return ema, previous, true
}

func trendRegimeAllowsAction(regime paperTrendRegime, action strategy.SignalAction) bool {
	if !regime.Enabled {
		return true
	}
	switch action {
	case strategy.SignalBuy:
		return regime.AllowLong
	case strategy.SignalShort:
		return regime.AllowShort
	default:
		return true
	}
}

func trendRegimeBlockReason(regime paperTrendRegime, action strategy.SignalAction) string {
	if regime.Reason != "" && regime.Reason != "trend_allows_long" && regime.Reason != "trend_allows_short" {
		return regime.Reason
	}
	if action == strategy.SignalBuy {
		return "trend_blocks_long"
	}
	if action == strategy.SignalShort {
		return "trend_blocks_short"
	}
	return "trend_blocks_entry"
}

func assessPaperSignal(candles []marketdata.Candle, result backtest.Result, signal paperSignal, snapshot paperMarketSnapshot, config paperRunConfig) paperSignalAssessment {
	features, values, ok := paperSignalFeatures(candles, signal, snapshot, config)
	if !isEntryAction(signal.Action) {
		reason := "no_entry_signal"
		if ok {
			blockers := paperEntryBlockers(values, config)
			features["entry_blockers"] = blockers
			if len(blockers) > 0 {
				reason = "no_entry_signal_" + blockers[0]
			}
		}
		return paperSignalAssessment{
			Score:      0,
			AllowEntry: false,
			Reason:     reason,
			Features:   features,
		}
	}
	if !ok {
		return paperSignalAssessment{
			Score:      0.35,
			AllowEntry: !config.SignalFilterEnabled,
			Reason:     "feature_unavailable",
			Features:   features,
		}
	}

	score := 0.50
	score += paperTrendScore(values.TrendSpreadPct, config.MinTrendSpreadPct)
	score += paperATRScore(values.ATRPct, values.HasATR, config.MinATRPct, config.MaxATRPct)
	score += paperVolumeScore(values.VolumeRatio, values.HasVolume, config.MinVolumeRatio)
	score += paperExtensionScore(values.EntryExtensionPct, config.MaxEntryExtensionPct)
	score += paperFundingScore(values.FundingAbsPct, config.MaxAbsFundingRatePct)
	score += paperMarkBasisScore(values.MarkBasisPct, values.HasMarkBasis)
	score += paperBacktestScore(result)
	score = clamp01(score)
	features["entry_blockers"] = paperEntryBlockers(values, config)

	allowed := !config.SignalFilterEnabled || score >= config.MinSignalScore
	reason := "score_pass"
	if !config.SignalFilterEnabled {
		reason = "filter_disabled"
	} else if !allowed {
		reason = "score_below_min"
	}
	features["score_components_version"] = "paper_signal_filter_v1"
	return paperSignalAssessment{
		Score:      score,
		AllowEntry: allowed,
		Reason:     reason,
		Features:   features,
	}
}

func paperSignalFeatures(candles []marketdata.Candle, signal paperSignal, snapshot paperMarketSnapshot, config paperRunConfig) (map[string]any, paperSignalFeatureValues, bool) {
	features := map[string]any{
		"feature_version": "paper_signal_features_v1",
	}
	values := paperSignalFeatureValues{}
	if len(candles) == 0 {
		features["feature_error"] = "no_candles"
		return features, values, false
	}
	last := candles[len(candles)-1]
	closePrice, err := parsePositiveFloat(last.Close, "latest close price")
	if err != nil {
		features["feature_error"] = "invalid_close"
		return features, values, false
	}
	fastAverage, slowAverage, ok := latestAverages(candles, config.FastWindow, config.SlowWindow)
	if !ok {
		features["feature_error"] = "not_enough_averages"
		return features, values, false
	}

	direction := 1.0
	if signal.Action == strategy.SignalShort {
		direction = -1
	}
	values.LatestClose = closePrice
	values.FastAverage = fastAverage
	values.SlowAverage = slowAverage
	values.TrendSpreadPct = math.Abs(fastAverage-slowAverage) / closePrice * 100
	values.EntryExtensionPct = (closePrice - fastAverage) / closePrice * 100
	if direction < 0 {
		values.EntryExtensionPct = (fastAverage - closePrice) / closePrice * 100
	}
	values.FundingAbsPct = math.Abs(snapshot.LatestFundingRatePct)
	if snapshot.MarkPrice > 0 {
		values.MarkBasisPct = math.Abs(snapshot.MarkPrice-closePrice) / closePrice * 100
		values.HasMarkBasis = true
	}
	if atrPct, ok := latestPaperATRPct(candles, config.ATRWindow); ok {
		values.ATRPct = atrPct
		values.HasATR = true
	}
	if ratio, ok := latestPaperVolumeRatio(candles, config.VolumeWindow); ok {
		values.VolumeRatio = ratio
		values.HasVolume = true
	}

	features["latest_close"] = values.LatestClose
	features["fast_average"] = values.FastAverage
	features["slow_average"] = values.SlowAverage
	features["trend_spread_pct"] = values.TrendSpreadPct
	features["entry_extension_pct"] = values.EntryExtensionPct
	features["funding_abs_pct"] = values.FundingAbsPct
	features["atr_pct"] = nullableFeature(values.ATRPct, values.HasATR)
	features["volume_ratio"] = nullableFeature(values.VolumeRatio, values.HasVolume)
	features["mark_basis_pct"] = nullableFeature(values.MarkBasisPct, values.HasMarkBasis)
	features["direction"] = signal.PositionSide
	return features, values, true
}

func paperEntryBlockers(values paperSignalFeatureValues, config paperRunConfig) []string {
	blockers := make([]string, 0, 4)
	if config.MinTrendSpreadPct > 0 && values.TrendSpreadPct < config.MinTrendSpreadPct {
		blockers = append(blockers, "trend_spread_below_min")
	}
	if values.HasATR {
		if config.MinATRPct > 0 && values.ATRPct < config.MinATRPct {
			blockers = append(blockers, "atr_below_min")
		}
		if config.MaxATRPct > 0 && values.ATRPct > config.MaxATRPct {
			blockers = append(blockers, "atr_above_max")
		}
	} else if config.ATRWindow > 0 && (config.MinATRPct > 0 || config.MaxATRPct > 0) {
		blockers = append(blockers, "atr_unavailable")
	}
	if values.HasVolume {
		if config.MinVolumeRatio > 0 && values.VolumeRatio < config.MinVolumeRatio {
			blockers = append(blockers, "volume_below_min")
		}
	} else if config.VolumeWindow > 0 && config.MinVolumeRatio > 0 {
		blockers = append(blockers, "volume_unavailable")
	}
	if config.MaxEntryExtensionPct > 0 && values.EntryExtensionPct > config.MaxEntryExtensionPct {
		blockers = append(blockers, "entry_extension_above_max")
	}
	return blockers
}

func nullableFeature(value float64, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func paperTrendScore(spreadPct float64, minSpreadPct float64) float64 {
	if minSpreadPct <= 0 {
		if spreadPct > 0.03 {
			return 0.08
		}
		return 0
	}
	if spreadPct < minSpreadPct {
		return -0.20
	}
	if spreadPct >= minSpreadPct*2 {
		return 0.14
	}
	if spreadPct >= minSpreadPct*1.5 {
		return 0.10
	}
	return 0.06
}

func paperATRScore(atrPct float64, ok bool, minATRPct float64, maxATRPct float64) float64 {
	if !ok {
		return -0.08
	}
	if minATRPct > 0 && atrPct < minATRPct {
		return -0.18
	}
	if maxATRPct > 0 && atrPct > maxATRPct {
		return -0.16
	}
	return 0.10
}

func paperVolumeScore(volumeRatio float64, ok bool, minVolumeRatio float64) float64 {
	if minVolumeRatio <= 0 {
		return 0
	}
	if !ok {
		return -0.06
	}
	if volumeRatio >= minVolumeRatio*1.5 {
		return 0.10
	}
	if volumeRatio >= minVolumeRatio {
		return 0.07
	}
	if volumeRatio >= 1 {
		return 0.02
	}
	return -0.12
}

func paperExtensionScore(extensionPct float64, maxExtensionPct float64) float64 {
	if maxExtensionPct <= 0 {
		return 0
	}
	if extensionPct <= maxExtensionPct {
		return 0.08
	}
	if extensionPct <= maxExtensionPct*1.5 {
		return -0.04
	}
	return -0.14
}

func paperFundingScore(absFundingPct float64, maxAbsFundingPct float64) float64 {
	if maxAbsFundingPct <= 0 {
		return 0
	}
	if absFundingPct <= maxAbsFundingPct*0.5 {
		return 0.05
	}
	if absFundingPct <= maxAbsFundingPct {
		return 0.01
	}
	return -0.15
}

func paperMarkBasisScore(markBasisPct float64, ok bool) float64 {
	if !ok {
		return -0.03
	}
	if markBasisPct <= 0.03 {
		return 0.04
	}
	if markBasisPct <= 0.10 {
		return 0
	}
	return -0.08
}

func paperBacktestScore(result backtest.Result) float64 {
	score := 0.0
	if result.ExcessReturnPct > 0 {
		score += math.Min(result.ExcessReturnPct/100, 0.06)
	} else if result.ExcessReturnPct < 0 {
		score -= math.Min(math.Abs(result.ExcessReturnPct)/100, 0.08)
	}
	if len(result.Trades) >= 5 {
		if result.WinRatePct >= 55 {
			score += 0.04
		} else if result.WinRatePct < 40 {
			score -= 0.05
		}
	}
	return score
}

func latestPaperATRPct(candles []marketdata.Candle, window int) (float64, bool) {
	if window <= 0 || len(candles) < window+1 {
		return 0, false
	}
	closes := make([]float64, 0, len(candles))
	highs := make([]float64, 0, len(candles))
	lows := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := parsePositiveFloat(candle.Close, "close price")
		if err != nil {
			return 0, false
		}
		high, err := parsePositiveFloat(candle.High, "high price")
		if err != nil {
			return 0, false
		}
		low, err := parsePositiveFloat(candle.Low, "low price")
		if err != nil || low > high {
			return 0, false
		}
		closes = append(closes, closePrice)
		highs = append(highs, high)
		lows = append(lows, low)
	}
	index := len(candles) - 1
	total := 0.0
	for i := index - window + 1; i <= index; i++ {
		highLow := highs[i] - lows[i]
		highPrevClose := math.Abs(highs[i] - closes[i-1])
		lowPrevClose := math.Abs(lows[i] - closes[i-1])
		total += math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
	}
	return total / float64(window) / closes[index] * 100, true
}

func latestPaperVolumeRatio(candles []marketdata.Candle, window int) (float64, bool) {
	if window <= 0 || len(candles) < window+1 {
		return 0, false
	}
	index := len(candles) - 1
	total := 0.0
	for i := index - window; i < index; i++ {
		volume, err := strconv.ParseFloat(candles[i].Volume, 64)
		if err != nil || volume < 0 || math.IsNaN(volume) || math.IsInf(volume, 0) {
			return 0, false
		}
		total += volume
	}
	current, err := strconv.ParseFloat(candles[index].Volume, 64)
	if err != nil || current < 0 || math.IsNaN(current) || math.IsInf(current, 0) {
		return 0, false
	}
	avg := total / float64(window)
	if avg <= 0 {
		return 0, false
	}
	return current / avg, true
}

func isEntryAction(action strategy.SignalAction) bool {
	return action == strategy.SignalBuy || action == strategy.SignalShort
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
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

func paperSignalReason(strategyType string, assessment paperSignalAssessment) string {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		if assessment.Reason != "" && assessment.Reason != "score_pass" {
			return "paper_scalp_tpsl_" + assessment.Reason
		}
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
	return config.Env(key, fallback)
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
