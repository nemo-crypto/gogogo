package main

import (
	"math"
	"testing"
	"time"

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
			Exchange:   "binance",
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
}

func TestApplyPaperProfileAggressiveDefaultsAndOverrides(t *testing.T) {
	strategyID := "sma-paper"
	market := "spot"
	interval := "1h"
	strategyType := "sma"
	fast := 12
	slow := 48
	takeProfitPct := 0.8
	stopLossPct := 0.4
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

	err := applyPaperProfile("aggressive", map[string]struct{}{"leverage": {}}, paperProfileFlags{
		strategyID:    &strategyID,
		market:        &market,
		interval:      &interval,
		strategyType:  &strategyType,
		fast:          &fast,
		slow:          &slow,
		takeProfitPct: &takeProfitPct,
		stopLossPct:   &stopLossPct,
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
	})
	if err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if strategyID != "perp-trend-scalp-aggressive-paper" {
		t.Fatalf("strategy id = %q", strategyID)
	}
	if market != "perpetual" || interval != "1m" || strategyType != "scalp-tpsl" {
		t.Fatalf("market=%q interval=%q strategy_type=%q, want aggressive perp 1m scalp", market, interval, strategyType)
	}
	if fast != 3 || slow != 12 {
		t.Fatalf("windows = %d/%d, want 3/12", fast, slow)
	}
	if !closeEnough(takeProfitPct, 0.65) || !closeEnough(stopLossPct, 0.25) {
		t.Fatalf("tp/sl = %f/%f, want 0.65/0.25", takeProfitPct, stopLossPct)
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
}

func testCandles(start time.Time, prices []string) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(prices))
	for i, price := range prices {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "binance",
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

func closeEnough(got float64, want float64) bool {
	return math.Abs(got-want) < 0.0000001
}
