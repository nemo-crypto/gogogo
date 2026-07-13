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

func closeEnough(got float64, want float64) bool {
	return math.Abs(got-want) < 0.0000001
}
