package main

import (
	"math"
	"strconv"
	"testing"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/strategy"
)

func TestLatestScalpSignalAllowsPerpetualShort(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	prices := []string{"105", "104", "103", "102", "101", "100"}
	candles := make([]marketdata.Candle, 0, len(prices))
	for i, price := range prices {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			Close:      price,
		})
	}

	signal := latestScalpSignal(candles, paperStrategyConfig{
		StrategyType: "scalp-tpsl",
		MarketType:   "perpetual",
		FastWindow:   2,
		SlowWindow:   3,
	})
	if signal.Action != strategy.SignalShort {
		t.Fatalf("action = %q, want short", signal.Action)
	}
	if signal.PositionSide != "short" {
		t.Fatalf("position side = %q, want short", signal.PositionSide)
	}
}

func TestPaperOrderPlanShortTPSL(t *testing.T) {
	side, positionSide, stopPrice, takeProfitPrice, err := paperOrderPlan(strategy.SignalShort, 100, 1, 0.5)
	if err != nil {
		t.Fatalf("paper order plan: %v", err)
	}
	if side != "sell" {
		t.Fatalf("side = %q, want sell", side)
	}
	if positionSide != "short" {
		t.Fatalf("position side = %q, want short", positionSide)
	}
	if !closeEnough(stopPrice, 100.5) {
		t.Fatalf("stop price = %f, want 100.5", stopPrice)
	}
	if !closeEnough(takeProfitPrice, 99) {
		t.Fatalf("take profit price = %f, want 99", takeProfitPrice)
	}
}

func TestPaperExitReasonShortDirection(t *testing.T) {
	position := portfolio.PaperPositionRecord{
		PositionSide:    "short",
		EntryPrice:      100,
		TakeProfitPrice: 99,
		StopLossPrice:   100.5,
	}
	if reason := paperExitReason(position, 99); reason != "take_profit" {
		t.Fatalf("reason = %q, want take_profit", reason)
	}
	if reason := paperExitReason(position, 100.5); reason != "stop_loss" {
		t.Fatalf("reason = %q, want stop_loss", reason)
	}
}

func TestPaperTrendExitReason(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testCandles(start, []string{"100", "99", "101", "103"})
	config := paperStrategyConfig{
		StrategyType: "scalp-tpsl",
		MarketType:   "perpetual",
		FastWindow:   2,
		SlowWindow:   3,
	}

	shortPosition := portfolio.PaperPositionRecord{PositionSide: "short"}
	if reason := paperTrendExitReason(shortPosition, candles, config); reason != "trend_reversal" {
		t.Fatalf("short reason = %q, want trend_reversal", reason)
	}

	longPosition := portfolio.PaperPositionRecord{PositionSide: "long"}
	if reason := paperTrendExitReason(longPosition, candles, config); reason != "" {
		t.Fatalf("long reason = %q, want empty", reason)
	}
}

func TestPaperProtectiveStopLossMovesLongStopToBreakevenAndTrail(t *testing.T) {
	start := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	candles := testFeatureCandles(start,
		[]float64{100, 100.5, 101, 101.5, 102},
		[]float64{100, 100, 100, 100, 100},
	)
	position := portfolio.PaperPositionRecord{
		PositionSide:    "long",
		EntryPrice:      100,
		MarkPrice:       100,
		StopLossPrice:   99,
		InitialStopLoss: 99,
		TakeProfitPrice: 104,
	}
	stop, reason, ok := paperProtectiveStopLoss(position, paperSettleRequest{
		MarkPrice:       101.1,
		Candles:         candles,
		ATRWindow:       3,
		BreakevenStop:   true,
		BreakevenR:      1,
		TrailingStop:    true,
		TrailingR:       1.5,
		TrailingATRMult: 1.2,
		PriceTickSize:   0.1,
	})
	if !ok {
		t.Fatal("protective stop not adjusted")
	}
	if reason != "breakeven_stop" {
		t.Fatalf("reason = %q, want breakeven_stop", reason)
	}
	if !closeEnough(stop, 100) {
		t.Fatalf("stop = %f, want 100", stop)
	}

	position.StopLossPrice = 100
	stop, reason, ok = paperProtectiveStopLoss(position, paperSettleRequest{
		MarkPrice:       102,
		Candles:         candles,
		ATRWindow:       3,
		BreakevenStop:   true,
		BreakevenR:      1,
		TrailingStop:    true,
		TrailingR:       1.5,
		TrailingATRMult: 0.5,
		PriceTickSize:   0.1,
	})
	if !ok {
		t.Fatal("trailing stop not adjusted")
	}
	if reason != "atr_trailing_stop" {
		t.Fatalf("reason = %q, want atr_trailing_stop", reason)
	}
	if stop <= 100 || stop >= 102 {
		t.Fatalf("trailing stop = %f, want between 100 and 102", stop)
	}
}

func TestPaperPositionNetPnLIncludesRoundTripCosts(t *testing.T) {
	position := portfolio.PaperPositionRecord{
		PositionSide: "long",
		Quantity:     2,
		EntryPrice:   100,
	}
	got := paperPositionNetPnL(position, 110, 0.001, 0.002)
	want := 20 - 100*2*0.003 - 110*2*0.003
	if !closeEnough(got, want) {
		t.Fatalf("net pnl = %f, want %f", got, want)
	}
}

func TestPaperOrderQuantityUsesRiskBudgetAndNotionalCap(t *testing.T) {
	got, err := paperOrderQuantity(100, 99, paperRunConfig{
		Equity:         10000,
		Quantity:       0.01,
		RiskPct:        0.5,
		MaxNotionalPct: 20,
	}, paperAccountState{Equity: 10000, AvailableBalance: 10000})
	if err != nil {
		t.Fatalf("paper order quantity: %v", err)
	}
	want := 20.0
	if !closeEnough(got, want) {
		t.Fatalf("quantity = %f, want %f", got, want)
	}

	got, err = paperOrderQuantity(100, 99, paperRunConfig{
		Equity:         10000,
		Quantity:       0.01,
		RiskPct:        0.5,
		MaxNotionalPct: 0,
	}, paperAccountState{Equity: 10000, AvailableBalance: 10000})
	if err != nil {
		t.Fatalf("paper order quantity without cap: %v", err)
	}
	want = 50.0
	if !closeEnough(got, want) {
		t.Fatalf("quantity = %f, want %f", got, want)
	}
}

func TestPaperOrderQuantityFallsBackToFixedQuantity(t *testing.T) {
	got, err := paperOrderQuantity(100, 99, paperRunConfig{
		Equity:   10000,
		Quantity: 0.25,
	}, paperAccountState{Equity: 10000, AvailableBalance: 10000})
	if err != nil {
		t.Fatalf("paper order quantity: %v", err)
	}
	if !closeEnough(got, 0.25) {
		t.Fatalf("quantity = %f, want 0.25", got)
	}
}

func TestPaperOrderQuantityCapsByAvailableBalanceAndMargin(t *testing.T) {
	state := paperAccountState{
		Equity:               10000,
		AvailableBalance:     1000,
		CurrentInitialMargin: 3400,
	}
	got, err := paperOrderQuantity(100, 99, paperRunConfig{
		Equity:            10000,
		RiskPct:           5,
		MaxNotionalPct:    100,
		MaxMarginPct:      35,
		MaxBalanceUsePct:  80,
		Leverage:          2,
		MaxOrderRiskPct:   10,
		MaxLeverage:       3,
		MinLiqDistancePct: 10,
		MaintMarginPct:    0.5,
	}, state)
	if err != nil {
		t.Fatalf("paper order quantity: %v", err)
	}
	want := 2.0
	if !closeEnough(got, want) {
		t.Fatalf("quantity = %f, want %f", got, want)
	}
}

func TestPaperOrderQuantityRejectsWhenNoMarginRemains(t *testing.T) {
	_, err := paperOrderQuantity(100, 99, paperRunConfig{
		Equity:           10000,
		RiskPct:          1,
		MaxMarginPct:     35,
		MaxBalanceUsePct: 80,
		Leverage:         1,
	}, paperAccountState{
		Equity:               10000,
		AvailableBalance:     1000,
		CurrentInitialMargin: 3500,
	})
	if err == nil {
		t.Fatal("paper order quantity error = nil, want error")
	}
}

func TestPaperRiskAccountSnapshotUsesCurrentState(t *testing.T) {
	snapshot := paperRiskAccountSnapshot(paperRunConfig{
		AccountID: "paper-v2",
		Equity:    10000,
	}, paperAccountState{
		Equity:                9900,
		AvailableBalance:      8500,
		DailyRealizedPnL:      -120,
		ConsecutiveLosses:     2,
		CurrentExposure:       2500,
		CurrentSymbolExposure: 2500,
		CurrentInitialMargin:  1250,
		CurrentMaintMargin:    12.5,
	})
	if snapshot.Equity != 9900 {
		t.Fatalf("equity = %f, want 9900", snapshot.Equity)
	}
	if snapshot.AvailableBalance != 8500 {
		t.Fatalf("available balance = %f, want 8500", snapshot.AvailableBalance)
	}
	if snapshot.CurrentInitialMargin != 1250 {
		t.Fatalf("initial margin = %f, want 1250", snapshot.CurrentInitialMargin)
	}
	if snapshot.DailyRealizedPnL != -120 {
		t.Fatalf("daily realized pnl = %f, want -120", snapshot.DailyRealizedPnL)
	}
	if snapshot.ConsecutiveLosses != 2 {
		t.Fatalf("consecutive losses = %d, want 2", snapshot.ConsecutiveLosses)
	}
}

func TestComputePaperTrendRegimeAllowsOnlyAlignedLong(t *testing.T) {
	start := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	trendCandles := testTrendCandles(start, "15m", []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109})
	macroCandles := testTrendCandles(start, "1h", []float64{90, 91, 92, 93, 94, 95, 96, 97, 98, 99})
	regime, err := computePaperTrendRegime(trendCandles, macroCandles, paperRunConfig{
		TrendFilterEnabled: true,
		TrendInterval:      "15m",
		MacroTrendInterval: "1h",
		TrendFastWindow:    3,
		TrendSlowWindow:    6,
		TrendMinSpreadPct:  0.01,
	})
	if err != nil {
		t.Fatalf("compute trend regime: %v", err)
	}
	if !regime.AllowLong || regime.AllowShort {
		t.Fatalf("allow long/short = %v/%v, want true/false; regime=%+v", regime.AllowLong, regime.AllowShort, regime)
	}
	if !trendRegimeAllowsAction(regime, strategy.SignalBuy) {
		t.Fatalf("buy blocked by regime: %+v", regime)
	}
	if trendRegimeAllowsAction(regime, strategy.SignalShort) {
		t.Fatalf("short allowed by long regime: %+v", regime)
	}
}

func TestComputePaperTrendRegimeBlocksWhenTrendDataMissing(t *testing.T) {
	start := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	candles := testTrendCandles(start, "15m", []float64{100, 101, 102})
	regime, err := computePaperTrendRegime(candles, candles, paperRunConfig{
		TrendFilterEnabled: true,
		TrendFastWindow:    3,
		TrendSlowWindow:    6,
		TrendMinSpreadPct:  0.01,
	})
	if err != nil {
		t.Fatalf("compute trend regime: %v", err)
	}
	if regime.AllowLong || regime.AllowShort {
		t.Fatalf("entries allowed with missing data: %+v", regime)
	}
	if regime.Reason != "trend_data_unavailable" {
		t.Fatalf("reason = %q, want trend_data_unavailable", regime.Reason)
	}
}

func TestApplyPaperProfileAggressiveDefaultsAndOverrides(t *testing.T) {
	strategyID := "sma-paper"
	market := "manual"
	interval := "1h"
	strategyType := "sma"
	fast := 12
	slow := 48
	takeProfitPct := 0.8
	stopLossPct := 0.4
	dynamicTPSL := false
	takeATRMult := 0.0
	stopATRMult := 0.0
	minTPPct := 0.0
	maxTPPct := 0.0
	minSLPct := 0.0
	maxSLPct := 0.0
	cooldownBars := 0
	minSpreadPct := 0.0
	confirmBars := 1
	atrWindow := 0
	minATRPct := 0.0
	maxATRPct := 0.0
	volumeWindow := 0
	minVolume := 0.0
	maxExtension := 0.0
	pullbackBars := 0
	pullbackTol := 0.0
	feeRate := 0.001
	slippageRate := 0.0005
	riskPct := 0.0
	maxNotional := 100.0
	maxMargin := 35.0
	maxBalanceUse := 80.0
	minLiqDist := 10.0
	maxOrderRisk := 1.0
	maxLeverage := 3.0
	leverage := 1.0
	signalFilter := false
	minSignal := 0.0
	trendFilter := false
	trendInterval := ""
	macroInterval := ""
	trendFast := 0
	trendSlow := 0
	trendMin := 0.0

	err := applyPaperProfile("aggressive", map[string]struct{}{"leverage": {}}, paperProfileFlags{
		strategyID:    &strategyID,
		market:        &market,
		interval:      &interval,
		strategyType:  &strategyType,
		fast:          &fast,
		slow:          &slow,
		takeProfitPct: &takeProfitPct,
		stopLossPct:   &stopLossPct,
		dynamicTPSL:   &dynamicTPSL,
		takeATRMult:   &takeATRMult,
		stopATRMult:   &stopATRMult,
		minTPPct:      &minTPPct,
		maxTPPct:      &maxTPPct,
		minSLPct:      &minSLPct,
		maxSLPct:      &maxSLPct,
		cooldownBars:  &cooldownBars,
		minSpreadPct:  &minSpreadPct,
		confirmBars:   &confirmBars,
		atrWindow:     &atrWindow,
		minATRPct:     &minATRPct,
		maxATRPct:     &maxATRPct,
		volumeWindow:  &volumeWindow,
		minVolume:     &minVolume,
		maxExtension:  &maxExtension,
		pullbackBars:  &pullbackBars,
		pullbackTol:   &pullbackTol,
		feeRate:       &feeRate,
		slippageRate:  &slippageRate,
		riskPct:       &riskPct,
		maxNotional:   &maxNotional,
		maxMargin:     &maxMargin,
		maxBalanceUse: &maxBalanceUse,
		minLiqDist:    &minLiqDist,
		maxOrderRisk:  &maxOrderRisk,
		maxLeverage:   &maxLeverage,
		leverage:      &leverage,
		signalFilter:  &signalFilter,
		minSignal:     &minSignal,
		trendFilter:   &trendFilter,
		trendInterval: &trendInterval,
		macroInterval: &macroInterval,
		trendFast:     &trendFast,
		trendSlow:     &trendSlow,
		trendMin:      &trendMin,
	})
	if err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if strategyID != defaultPaperStrategyID {
		t.Fatalf("strategy id = %q", strategyID)
	}
	if market != "perpetual" || interval != "5m" || strategyType != "scalp-tpsl" {
		t.Fatalf("market=%q interval=%q strategy_type=%q, want aggressive perp 5m scalp", market, interval, strategyType)
	}
	if fast != 3 || slow != 9 {
		t.Fatalf("windows = %d/%d, want 3/9", fast, slow)
	}
	if !closeEnough(takeProfitPct, 0.80) || !closeEnough(stopLossPct, 0.45) {
		t.Fatalf("tp/sl = %f/%f, want 0.80/0.45", takeProfitPct, stopLossPct)
	}
	if !dynamicTPSL || !closeEnough(takeATRMult, 1.8) || !closeEnough(stopATRMult, 1.0) {
		t.Fatalf("dynamic tpsl = %v multipliers=%f/%f, want true 1.8/1.0", dynamicTPSL, takeATRMult, stopATRMult)
	}
	if !closeEnough(minTPPct, 0.60) || !closeEnough(maxTPPct, 1.60) || !closeEnough(minSLPct, 0.30) || !closeEnough(maxSLPct, 0.75) {
		t.Fatalf("dynamic tpsl bounds tp=%f/%f sl=%f/%f", minTPPct, maxTPPct, minSLPct, maxSLPct)
	}
	if !closeEnough(riskPct, 2) || !closeEnough(maxNotional, 220) {
		t.Fatalf("risk/notional = %f/%f, want 2/220", riskPct, maxNotional)
	}
	if !closeEnough(maxExtension, 0.18) || pullbackBars != 5 || !closeEnough(pullbackTol, 0.06) {
		t.Fatalf("entry filters = %f/%d/%f", maxExtension, pullbackBars, pullbackTol)
	}
	if !closeEnough(leverage, 1) {
		t.Fatalf("leverage = %f, want manual override 1", leverage)
	}
	if !signalFilter || !closeEnough(minSignal, 0.50) {
		t.Fatalf("signal filter = %v min=%f, want true 0.50", signalFilter, minSignal)
	}
	if !trendFilter || trendInterval != "15m" || macroInterval != "1h" || trendFast != 20 || trendSlow != 60 || !closeEnough(trendMin, 0.05) {
		t.Fatalf("trend profile = %v %s/%s %d/%d min=%f", trendFilter, trendInterval, macroInterval, trendFast, trendSlow, trendMin)
	}
}

func TestApplyPaperProfileSmallScalpDefaults(t *testing.T) {
	strategyID := "sma-paper"
	market := "manual"
	interval := "1h"
	strategyType := "sma"
	fast := 12
	slow := 48
	takeProfitPct := 0.0
	stopLossPct := 0.0
	dynamicTPSL := false
	takeATRMult := 0.0
	stopATRMult := 0.0
	minTPPct := 0.0
	maxTPPct := 0.0
	minSLPct := 0.0
	maxSLPct := 0.0
	cooldownBars := 0
	minSpreadPct := 0.0
	confirmBars := 0
	atrWindow := 0
	minATRPct := 0.0
	maxATRPct := 0.0
	volumeWindow := 0
	minVolume := 0.0
	maxExtension := 0.0
	pullbackBars := 0
	pullbackTol := 0.0
	feeRate := 0.0
	slippageRate := 0.0
	riskPct := 0.0
	maxNotional := 0.0
	maxMargin := 0.0
	maxBalanceUse := 0.0
	minLiqDist := 0.0
	maxOrderRisk := 0.0
	maxLeverage := 0.0
	leverage := 0.0
	signalFilter := false
	minSignal := 0.0
	trendFilter := false
	trendInterval := ""
	macroInterval := ""
	trendFast := 0
	trendSlow := 0
	trendMin := 0.0

	err := applyPaperProfile("small-scalp", map[string]struct{}{}, paperProfileFlags{
		strategyID:    &strategyID,
		market:        &market,
		interval:      &interval,
		strategyType:  &strategyType,
		fast:          &fast,
		slow:          &slow,
		takeProfitPct: &takeProfitPct,
		stopLossPct:   &stopLossPct,
		dynamicTPSL:   &dynamicTPSL,
		takeATRMult:   &takeATRMult,
		stopATRMult:   &stopATRMult,
		minTPPct:      &minTPPct,
		maxTPPct:      &maxTPPct,
		minSLPct:      &minSLPct,
		maxSLPct:      &maxSLPct,
		cooldownBars:  &cooldownBars,
		minSpreadPct:  &minSpreadPct,
		confirmBars:   &confirmBars,
		atrWindow:     &atrWindow,
		minATRPct:     &minATRPct,
		maxATRPct:     &maxATRPct,
		volumeWindow:  &volumeWindow,
		minVolume:     &minVolume,
		maxExtension:  &maxExtension,
		pullbackBars:  &pullbackBars,
		pullbackTol:   &pullbackTol,
		feeRate:       &feeRate,
		slippageRate:  &slippageRate,
		riskPct:       &riskPct,
		maxNotional:   &maxNotional,
		maxMargin:     &maxMargin,
		maxBalanceUse: &maxBalanceUse,
		minLiqDist:    &minLiqDist,
		maxOrderRisk:  &maxOrderRisk,
		maxLeverage:   &maxLeverage,
		leverage:      &leverage,
		signalFilter:  &signalFilter,
		minSignal:     &minSignal,
		trendFilter:   &trendFilter,
		trendInterval: &trendInterval,
		macroInterval: &macroInterval,
		trendFast:     &trendFast,
		trendSlow:     &trendSlow,
		trendMin:      &trendMin,
	})
	if err != nil {
		t.Fatalf("apply small scalp profile: %v", err)
	}
	if normalizedPaperProfile("small") != "small-scalp" || normalizedPaperProfile("small-capital") != "small-scalp" {
		t.Fatalf("small aliases did not normalize to small-scalp")
	}
	if market != "perpetual" || interval != "5m" || strategyType != "scalp-tpsl" {
		t.Fatalf("market=%q interval=%q strategy_type=%q, want small scalp perp 5m scalp", market, interval, strategyType)
	}
	if !closeEnough(takeProfitPct, 0.65) || !closeEnough(stopLossPct, 0.40) {
		t.Fatalf("tp/sl = %f/%f, want 0.65/0.40", takeProfitPct, stopLossPct)
	}
	if !dynamicTPSL || !closeEnough(takeATRMult, 1.35) || !closeEnough(stopATRMult, 0.90) {
		t.Fatalf("dynamic tpsl = %v multipliers=%f/%f, want true 1.35/0.90", dynamicTPSL, takeATRMult, stopATRMult)
	}
	if !closeEnough(minTPPct, 0.45) || !closeEnough(maxTPPct, 1.20) || !closeEnough(minSLPct, 0.25) || !closeEnough(maxSLPct, 0.65) {
		t.Fatalf("dynamic tpsl bounds tp=%f/%f sl=%f/%f", minTPPct, maxTPPct, minSLPct, maxSLPct)
	}
	if !closeEnough(minSpreadPct, 0.015) || !closeEnough(minATRPct, 0.05) || !closeEnough(minVolume, 1.00) {
		t.Fatalf("entry filters spread=%f atr=%f volume=%f", minSpreadPct, minATRPct, minVolume)
	}
	if !closeEnough(maxExtension, 0.22) || pullbackBars != 3 || !closeEnough(pullbackTol, 0.08) {
		t.Fatalf("entry filters extension=%f pullback=%d/%f", maxExtension, pullbackBars, pullbackTol)
	}
	if !closeEnough(riskPct, 0.80) || !closeEnough(maxNotional, 150) || !closeEnough(maxMargin, 45) || !closeEnough(maxBalanceUse, 75) || !closeEnough(maxOrderRisk, 1.20) {
		t.Fatalf("risk profile risk=%f notional=%f margin=%f balance=%f order_risk=%f", riskPct, maxNotional, maxMargin, maxBalanceUse, maxOrderRisk)
	}
	if !closeEnough(leverage, 3) || !closeEnough(maxLeverage, 3) {
		t.Fatalf("leverage profile leverage=%f max=%f", leverage, maxLeverage)
	}
	if !signalFilter || !closeEnough(minSignal, 0.45) {
		t.Fatalf("signal filter = %v min=%f, want true 0.45", signalFilter, minSignal)
	}
	if !trendFilter || trendInterval != "15m" || macroInterval != "15m" || trendFast != 8 || trendSlow != 21 || !closeEnough(trendMin, 0.02) {
		t.Fatalf("trend profile = %v %s/%s %d/%d min=%f", trendFilter, trendInterval, macroInterval, trendFast, trendSlow, trendMin)
	}
}

type paperProfileTestValues struct {
	strategyID    string
	market        string
	interval      string
	strategyType  string
	fast          int
	slow          int
	takeProfitPct float64
	stopLossPct   float64
	dynamicTPSL   bool
	takeATRMult   float64
	stopATRMult   float64
	minTPPct      float64
	maxTPPct      float64
	minSLPct      float64
	maxSLPct      float64
	cooldownBars  int
	minSpreadPct  float64
	confirmBars   int
	atrWindow     int
	minATRPct     float64
	maxATRPct     float64
	volumeWindow  int
	minVolume     float64
	maxExtension  float64
	pullbackBars  int
	pullbackTol   float64
	feeRate       float64
	slippageRate  float64
	riskPct       float64
	maxNotional   float64
	maxMargin     float64
	maxBalanceUse float64
	minLiqDist    float64
	maxOrderRisk  float64
	maxLeverage   float64
	leverage      float64
	signalFilter  bool
	minSignal     float64
	trendFilter   bool
	trendInterval string
	macroInterval string
	trendFast     int
	trendSlow     int
	trendMin      float64
	maxCandleAge  time.Duration
}

func newPaperProfileTestValues() *paperProfileTestValues {
	return &paperProfileTestValues{
		strategyID:   "sma-paper",
		market:       "manual",
		interval:     "1h",
		strategyType: "sma",
		fast:         12,
		slow:         48,
	}
}

func (v *paperProfileTestValues) flags() paperProfileFlags {
	return paperProfileFlags{
		strategyID:    &v.strategyID,
		market:        &v.market,
		interval:      &v.interval,
		strategyType:  &v.strategyType,
		fast:          &v.fast,
		slow:          &v.slow,
		takeProfitPct: &v.takeProfitPct,
		stopLossPct:   &v.stopLossPct,
		dynamicTPSL:   &v.dynamicTPSL,
		takeATRMult:   &v.takeATRMult,
		stopATRMult:   &v.stopATRMult,
		minTPPct:      &v.minTPPct,
		maxTPPct:      &v.maxTPPct,
		minSLPct:      &v.minSLPct,
		maxSLPct:      &v.maxSLPct,
		cooldownBars:  &v.cooldownBars,
		minSpreadPct:  &v.minSpreadPct,
		confirmBars:   &v.confirmBars,
		atrWindow:     &v.atrWindow,
		minATRPct:     &v.minATRPct,
		maxATRPct:     &v.maxATRPct,
		volumeWindow:  &v.volumeWindow,
		minVolume:     &v.minVolume,
		maxExtension:  &v.maxExtension,
		pullbackBars:  &v.pullbackBars,
		pullbackTol:   &v.pullbackTol,
		feeRate:       &v.feeRate,
		slippageRate:  &v.slippageRate,
		riskPct:       &v.riskPct,
		maxNotional:   &v.maxNotional,
		maxMargin:     &v.maxMargin,
		maxBalanceUse: &v.maxBalanceUse,
		minLiqDist:    &v.minLiqDist,
		maxOrderRisk:  &v.maxOrderRisk,
		maxLeverage:   &v.maxLeverage,
		leverage:      &v.leverage,
		signalFilter:  &v.signalFilter,
		minSignal:     &v.minSignal,
		trendFilter:   &v.trendFilter,
		trendInterval: &v.trendInterval,
		macroInterval: &v.macroInterval,
		trendFast:     &v.trendFast,
		trendSlow:     &v.trendSlow,
		trendMin:      &v.trendMin,
		maxCandleAge:  &v.maxCandleAge,
	}
}

func TestApplyPaperProfileMicroTrend1MDefaults(t *testing.T) {
	values := newPaperProfileTestValues()

	err := applyPaperProfile("1m", map[string]struct{}{}, values.flags())
	if err != nil {
		t.Fatalf("apply micro trend profile: %v", err)
	}
	for _, alias := range []string{"micro", "micro-trend-1m", "1m", "scalp-1m"} {
		if normalizedPaperProfile(alias) != "micro-trend-1m" {
			t.Fatalf("alias %q normalized to %q, want micro-trend-1m", alias, normalizedPaperProfile(alias))
		}
	}
	if values.market != "perpetual" || values.interval != "1m" || values.strategyType != "scalp-tpsl" {
		t.Fatalf("market=%q interval=%q strategy_type=%q, want micro trend perp 1m scalp", values.market, values.interval, values.strategyType)
	}
	if values.fast != 2 || values.slow != 5 {
		t.Fatalf("windows = %d/%d, want 2/5", values.fast, values.slow)
	}
	if !closeEnough(values.takeProfitPct, 0.35) || !closeEnough(values.stopLossPct, 0.22) {
		t.Fatalf("tp/sl = %f/%f, want 0.35/0.22", values.takeProfitPct, values.stopLossPct)
	}
	if !values.dynamicTPSL || !closeEnough(values.takeATRMult, 1.10) || !closeEnough(values.stopATRMult, 0.75) {
		t.Fatalf("dynamic tpsl = %v multipliers=%f/%f, want true 1.10/0.75", values.dynamicTPSL, values.takeATRMult, values.stopATRMult)
	}
	if !closeEnough(values.minTPPct, 0.25) || !closeEnough(values.maxTPPct, 0.80) || !closeEnough(values.minSLPct, 0.15) || !closeEnough(values.maxSLPct, 0.45) {
		t.Fatalf("dynamic tpsl bounds tp=%f/%f sl=%f/%f", values.minTPPct, values.maxTPPct, values.minSLPct, values.maxSLPct)
	}
	if values.cooldownBars != 0 || !closeEnough(values.minSpreadPct, 0.004) || values.confirmBars != 1 {
		t.Fatalf("entry timing cooldown=%d spread=%f confirm=%d", values.cooldownBars, values.minSpreadPct, values.confirmBars)
	}
	if !closeEnough(values.minATRPct, 0.02) || !closeEnough(values.minVolume, 0.50) || !closeEnough(values.maxExtension, 0.50) {
		t.Fatalf("entry filters atr=%f volume=%f extension=%f", values.minATRPct, values.minVolume, values.maxExtension)
	}
	if values.pullbackBars != 1 || !closeEnough(values.pullbackTol, 0.10) {
		t.Fatalf("pullback = %d/%f, want 1/0.10", values.pullbackBars, values.pullbackTol)
	}
	if !closeEnough(values.riskPct, 0.50) || !closeEnough(values.maxNotional, 150) || !closeEnough(values.maxMargin, 45) || !closeEnough(values.maxBalanceUse, 75) || !closeEnough(values.maxOrderRisk, 0.80) {
		t.Fatalf("risk profile risk=%f notional=%f margin=%f balance=%f order_risk=%f", values.riskPct, values.maxNotional, values.maxMargin, values.maxBalanceUse, values.maxOrderRisk)
	}
	if !closeEnough(values.leverage, 3) || !closeEnough(values.maxLeverage, 3) {
		t.Fatalf("leverage profile leverage=%f max=%f", values.leverage, values.maxLeverage)
	}
	if !values.signalFilter || !closeEnough(values.minSignal, 0.35) {
		t.Fatalf("signal filter = %v min=%f, want true 0.35", values.signalFilter, values.minSignal)
	}
	if !values.trendFilter || values.trendInterval != "5m" || values.macroInterval != "5m" || values.trendFast != 8 || values.trendSlow != 21 || !closeEnough(values.trendMin, 0.005) {
		t.Fatalf("trend profile = %v %s/%s %d/%d min=%f", values.trendFilter, values.trendInterval, values.macroInterval, values.trendFast, values.trendSlow, values.trendMin)
	}
	if values.maxCandleAge != 2*time.Minute {
		t.Fatalf("max candle age = %s, want 2m", values.maxCandleAge)
	}
}

func TestApplyPaperProfileSmallScalpFastDefaults(t *testing.T) {
	values := newPaperProfileTestValues()

	err := applyPaperProfile("300u", map[string]struct{}{}, values.flags())
	if err != nil {
		t.Fatalf("apply fast small scalp profile: %v", err)
	}
	if normalizedPaperProfile("small-scalp-fast") != "small-scalp-fast" || normalizedPaperProfile("300u") != "small-scalp-fast" {
		t.Fatalf("fast aliases did not normalize to small-scalp-fast")
	}
	if values.market != "perpetual" || values.interval != "5m" || values.strategyType != "scalp-tpsl" {
		t.Fatalf("market=%q interval=%q strategy_type=%q, want fast small scalp perp 5m scalp", values.market, values.interval, values.strategyType)
	}
	if !closeEnough(values.minSpreadPct, 0.008) || !closeEnough(values.minATRPct, 0.05) || !closeEnough(values.minVolume, 0.80) {
		t.Fatalf("entry filters spread=%f atr=%f volume=%f", values.minSpreadPct, values.minATRPct, values.minVolume)
	}
	if !closeEnough(values.maxExtension, 0.35) || values.pullbackBars != 2 || !closeEnough(values.pullbackTol, 0.08) {
		t.Fatalf("entry filters extension=%f pullback=%d/%f", values.maxExtension, values.pullbackBars, values.pullbackTol)
	}
	if !closeEnough(values.riskPct, 0.80) || !closeEnough(values.maxMargin, 45) || !closeEnough(values.maxBalanceUse, 75) || !closeEnough(values.maxOrderRisk, 1.20) {
		t.Fatalf("risk profile risk=%f margin=%f balance=%f order_risk=%f", values.riskPct, values.maxMargin, values.maxBalanceUse, values.maxOrderRisk)
	}
	if !values.signalFilter || !closeEnough(values.minSignal, 0.40) {
		t.Fatalf("signal filter = %v min=%f, want true 0.40", values.signalFilter, values.minSignal)
	}
	if !values.trendFilter || values.trendInterval != "15m" || values.macroInterval != "15m" || values.trendFast != 8 || values.trendSlow != 21 || !closeEnough(values.trendMin, 0.01) {
		t.Fatalf("trend profile = %v %s/%s %d/%d min=%f", values.trendFilter, values.trendInterval, values.macroInterval, values.trendFast, values.trendSlow, values.trendMin)
	}
}

func TestAlignPaperOrderPlanRoundsConservatively(t *testing.T) {
	plan, err := alignPaperOrderPlan("short", 100.04, 100.26, 99.87, 0.0019, paperRunConfig{
		MinOrderQuantity: 0.001,
		QuantityStep:     0.001,
		PriceTickSize:    0.1,
	})
	if err != nil {
		t.Fatalf("align order plan: %v", err)
	}
	if !closeEnough(plan.EntryPrice, 100.0) {
		t.Fatalf("entry = %f, want 100.0", plan.EntryPrice)
	}
	if !closeEnough(plan.StopPrice, 100.3) {
		t.Fatalf("stop = %f, want 100.3", plan.StopPrice)
	}
	if !closeEnough(plan.TakeProfitPrice, 99.9) {
		t.Fatalf("take profit = %f, want 99.9", plan.TakeProfitPrice)
	}
	if !closeEnough(plan.Quantity, 0.001) {
		t.Fatalf("quantity = %f, want 0.001", plan.Quantity)
	}
}

func TestValidatePaperMarketFreshness(t *testing.T) {
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	err := validatePaperMarketFreshness(paperMarketSnapshot{
		PriceSource:     "latest_mark_price",
		PriceTime:       now.Add(-30 * time.Second),
		CandleCloseTime: now.Add(-45 * time.Second),
	}, now, paperRunConfig{MaxCandleAge: time.Minute, MaxMarkPriceAge: time.Minute})
	if err != nil {
		t.Fatalf("freshness: %v", err)
	}
	err = validatePaperMarketFreshness(paperMarketSnapshot{
		PriceSource:     "latest_mark_price",
		PriceTime:       now.Add(-3 * time.Minute),
		CandleCloseTime: now.Add(-30 * time.Second),
	}, now, paperRunConfig{MaxCandleAge: time.Minute, MaxMarkPriceAge: time.Minute})
	if err == nil {
		t.Fatal("freshness error = nil, want stale mark price")
	}
}

func TestClosedPaperCandlesExcludeInProgressCandle(t *testing.T) {
	start := time.Date(2026, 7, 15, 11, 30, 0, 0, time.UTC)
	candles := make([]marketdata.Candle, 0, 3)
	for i, closePrice := range []float64{100, 101, 102} {
		openTime := start.Add(time.Duration(i) * 5 * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "5m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(5 * time.Minute),
			High:       formatTestFloat(closePrice + 0.5),
			Low:        formatTestFloat(closePrice - 0.5),
			Close:      formatTestFloat(closePrice),
			Volume:     "100",
		})
	}
	asOf := start.Add(10*time.Minute + 15*time.Second)

	closed := closedPaperCandles(candles, asOf)
	if len(closed) != 2 {
		t.Fatalf("closed candles = %d, want 2", len(closed))
	}
	if !closed[len(closed)-1].CloseTime.Equal(start.Add(10 * time.Minute)) {
		t.Fatalf("latest closed candle close = %s", closed[len(closed)-1].CloseTime)
	}
}

func TestPaperEffectiveTPSLUsesDynamicATR(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := make([]marketdata.Candle, 0, 6)
	for i, price := range []float64{100, 101, 102, 103, 104, 105} {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "5m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(5 * time.Minute),
			High:       formatTestFloat(price + 2),
			Low:        formatTestFloat(price - 2),
			Close:      formatTestFloat(price),
			Volume:     "100",
		})
	}

	got, err := paperEffectiveTPSL(candles, paperRunConfig{
		MarketType:        "perpetual",
		FastWindow:        2,
		SlowWindow:        3,
		TakeProfitPct:     0.8,
		StopLossPct:       0.45,
		DynamicTPSL:       true,
		TakeProfitATRMult: 1.6,
		StopLossATRMult:   1,
		ATRWindow:         3,
		MinTakeProfitPct:  0.55,
		MaxTakeProfitPct:  1.4,
		MinStopLossPct:    0.3,
		MaxStopLossPct:    0.75,
	})
	if err != nil {
		t.Fatalf("effective tpsl: %v", err)
	}
	if got.Source != "atr_dynamic" || !closeEnough(got.TakeProfitPct, 1.4) || !closeEnough(got.StopLossPct, 0.75) {
		t.Fatalf("effective tpsl = %+v, want atr_dynamic 1.4/0.75", got)
	}
}

func TestAssessPaperSignalAllowsHighQualityEntry(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testFeatureCandles(start,
		[]float64{100, 100.2, 100.4, 100.8, 101.2, 101.7, 102.2, 102.8, 103.4, 104.1, 104.9},
		[]float64{100, 110, 105, 120, 115, 130, 125, 140, 135, 145, 210},
	)
	assessment := assessPaperSignal(candles, backtestResult(2, 56, 12), paperSignal{
		Action:       strategy.SignalBuy,
		PositionSide: "long",
	}, paperMarketSnapshot{
		MarkPrice:            104.9,
		LatestFundingRatePct: 0.01,
	}, paperRunConfig{
		FastWindow:           3,
		SlowWindow:           9,
		ATRWindow:            3,
		MinATRPct:            0.05,
		MaxATRPct:            2,
		VolumeWindow:         3,
		MinVolumeRatio:       1.10,
		MaxEntryExtensionPct: 0.5,
		MaxAbsFundingRatePct: 0.05,
		SignalFilterEnabled:  true,
		MinSignalScore:       0.55,
	})
	if !assessment.AllowEntry {
		t.Fatalf("allow entry = false, reason=%s score=%f features=%v", assessment.Reason, assessment.Score, assessment.Features)
	}
	if assessment.Score < 0.55 {
		t.Fatalf("score = %f, want >= 0.55", assessment.Score)
	}
}

func TestAssessPaperSignalBlocksBelowThreshold(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testFeatureCandles(start,
		[]float64{100, 100.2, 100.4, 100.8, 101.2, 101.7, 102.2, 102.8, 103.4, 104.1, 104.9},
		[]float64{100, 105, 100, 105, 100, 105, 100, 105, 100, 105, 90},
	)
	assessment := assessPaperSignal(candles, backtestResult(-8, 35, 12), paperSignal{
		Action:       strategy.SignalBuy,
		PositionSide: "long",
	}, paperMarketSnapshot{
		MarkPrice:            105.5,
		LatestFundingRatePct: 0.08,
	}, paperRunConfig{
		FastWindow:           3,
		SlowWindow:           9,
		ATRWindow:            3,
		MinATRPct:            0.05,
		MaxATRPct:            2,
		VolumeWindow:         3,
		MinVolumeRatio:       1.10,
		MaxEntryExtensionPct: 0.05,
		MaxAbsFundingRatePct: 0.05,
		SignalFilterEnabled:  true,
		MinSignalScore:       0.90,
	})
	if assessment.AllowEntry {
		t.Fatalf("allow entry = true, want blocked; score=%f features=%v", assessment.Score, assessment.Features)
	}
	if assessment.Reason != "score_below_min" {
		t.Fatalf("reason = %q, want score_below_min", assessment.Reason)
	}
}

func TestAssessPaperSignalExplainsNoEntryBlockers(t *testing.T) {
	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testFeatureCandles(start,
		[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05, 100.06, 100.07, 100.08, 100.09, 100.10},
		[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 10},
	)
	assessment := assessPaperSignal(candles, backtestResult(0, 50, 0), paperSignal{
		Action: strategy.SignalHold,
	}, paperMarketSnapshot{
		MarkPrice: 100.1,
	}, paperRunConfig{
		FastWindow:           3,
		SlowWindow:           9,
		ATRWindow:            3,
		MinATRPct:            0.50,
		MaxATRPct:            2,
		VolumeWindow:         3,
		MinVolumeRatio:       1.10,
		MaxEntryExtensionPct: 0.5,
		MinTrendSpreadPct:    0.05,
	})
	if assessment.AllowEntry {
		t.Fatal("hold signal allowed entry, want blocked")
	}
	blockers, ok := assessment.Features["entry_blockers"].([]string)
	if !ok || len(blockers) == 0 {
		t.Fatalf("entry blockers = %#v, want non-empty []string", assessment.Features["entry_blockers"])
	}
	if assessment.Reason != "no_entry_signal_trend_spread_below_min" {
		t.Fatalf("reason = %q", assessment.Reason)
	}
}

func TestParseFundingRatePct(t *testing.T) {
	got, err := parseFundingRatePct("0.0001")
	if err != nil {
		t.Fatalf("parse funding rate: %v", err)
	}
	if !closeEnough(got, 0.01) {
		t.Fatalf("funding pct = %f, want 0.01", got)
	}
}

func TestPaperWatchStateThrottlesBacktestAndObservationPersistence(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	state := paperWatchState{}
	config := paperRunConfig{
		PersistInterval:  time.Minute,
		BacktestInterval: 5 * time.Minute,
	}

	first := state.nextOptions(now, config)
	if !first.SaveObservation || !first.SaveBacktest || !first.SaveAccountSnapshot {
		t.Fatalf("first options = %+v, want all persistence enabled", first)
	}

	state.observe(now, paperRunSummary{
		ObservationSaved: true,
		BacktestSaved:    true,
		LatestCandleTime: now.Add(-5 * time.Minute),
	})
	second := state.nextOptions(now.Add(30*time.Second), config)
	if second.SaveObservation || second.SaveBacktest || second.SaveAccountSnapshot {
		t.Fatalf("second options = %+v, want throttled", second)
	}
	if !second.LastObservedCandleTime.Equal(now.Add(-5 * time.Minute)) {
		t.Fatalf("last candle = %s", second.LastObservedCandleTime)
	}

	third := state.nextOptions(now.Add(5*time.Minute), config)
	if !third.SaveObservation || !third.SaveBacktest || !third.SaveAccountSnapshot {
		t.Fatalf("third options = %+v, want persistence after intervals", third)
	}
}

func TestShouldSavePaperObservationForNewCandleAndOrders(t *testing.T) {
	lastCandle := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	options := paperRunOptions{LastObservedCandleTime: lastCandle}

	if shouldSavePaperObservation(options, lastCandle, strategy.SignalHold, "") {
		t.Fatal("unchanged hold observation saved, want throttled")
	}
	if !shouldSavePaperObservation(options, lastCandle.Add(5*time.Minute), strategy.SignalHold, "") {
		t.Fatal("new candle observation not saved")
	}
	if !shouldSavePaperObservation(options, lastCandle, strategy.SignalShort, "") {
		t.Fatal("entry signal observation not saved")
	}
	if !shouldSavePaperObservation(options, lastCandle, strategy.SignalHold, "take_profit") {
		t.Fatal("close event observation not saved")
	}
}

func TestPaperEntryRiskHaltDetectsConsecutiveLosses(t *testing.T) {
	halt := paperEntryRiskHalt(paperRunConfig{MaxConsecutiveLosses: 3}, paperAccountState{ConsecutiveLosses: 3})
	if halt.Reason != "consecutive_loss_halt" {
		t.Fatalf("halt reason = %q, want consecutive_loss_halt", halt.Reason)
	}

	halt = paperEntryRiskHalt(paperRunConfig{Equity: 300, MaxDailyLossPct: 2}, paperAccountState{DailyRealizedPnL: -6})
	if halt.Reason != "daily_loss_halt" {
		t.Fatalf("halt reason = %q, want daily_loss_halt", halt.Reason)
	}

	halt = paperEntryRiskHalt(paperRunConfig{MaxConsecutiveLosses: 3}, paperAccountState{ConsecutiveLosses: 2})
	if halt.Reason != "" {
		t.Fatalf("halt reason = %q, want none", halt.Reason)
	}
}

func testCandles(start time.Time, prices []string) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(prices))
	for i, price := range prices {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			Close:      price,
		})
	}
	return candles
}

func testFeatureCandles(start time.Time, closes []float64, volumes []float64) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(closes))
	for i, closePrice := range closes {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "5m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(5 * time.Minute),
			High:       formatTestFloat(closePrice + 0.5),
			Low:        formatTestFloat(closePrice - 0.5),
			Close:      formatTestFloat(closePrice),
			Volume:     formatTestFloat(volumes[i]),
		})
	}
	return candles
}

func testTrendCandles(start time.Time, interval string, closes []float64) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(closes))
	for i, closePrice := range closes {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   interval,
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			Close:      formatTestFloat(closePrice),
		})
	}
	return candles
}

func backtestResult(excessReturnPct float64, winRatePct float64, trades int) backtest.Result {
	return backtest.Result{
		ExcessReturnPct: excessReturnPct,
		WinRatePct:      winRatePct,
		Trades:          make([]backtest.Trade, trades),
	}
}

func closeEnough(got float64, want float64) bool {
	return math.Abs(got-want) < 0.0000001
}

func formatTestFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
