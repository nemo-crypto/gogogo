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
	})
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
	})
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
		Quantity: 0.25,
	})
	if err != nil {
		t.Fatalf("paper order quantity: %v", err)
	}
	if !closeEnough(got, 0.25) {
		t.Fatalf("quantity = %f, want 0.25", got)
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
