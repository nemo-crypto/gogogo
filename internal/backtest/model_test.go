package backtest

import (
	"errors"
	"testing"
	"time"

	"gogogo/internal/marketdata"
)

func TestRunSMACrossover(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	prices := []string{"10", "10", "10", "11", "12", "13", "14", "15", "15", "15", "14", "13"}
	candles := make([]marketdata.Candle, 0, len(prices))
	for i, price := range prices {
		openTime := start.Add(time.Duration(i) * time.Hour)
		candles = append(candles, marketdata.Candle{
			Exchange:   "binance",
			MarketType: marketdata.MarketTypeSpot,
			Symbol:     "BTCUSDT",
			Interval:   "1h",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Hour),
			Close:      price,
		})
	}

	result, err := RunSMACrossover(candles, SMAConfig{
		FastWindow: 2,
		SlowWindow: 3,
		FeeRate:    0.001,
	})
	if err != nil {
		t.Fatalf("run sma crossover: %v", err)
	}
	if result.StrategyName != "sma_crossover_2_3" {
		t.Fatalf("strategy name = %q, want sma_crossover_2_3", result.StrategyName)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("trades length = %d, want 1", len(result.Trades))
	}
	if result.FinalEquity <= 1 {
		t.Fatalf("final equity = %f, want profitable test path", result.FinalEquity)
	}
}

func TestRunSMACrossoverNotEnoughData(t *testing.T) {
	t.Parallel()

	_, err := RunSMACrossover(nil, SMAConfig{
		FastWindow: 2,
		SlowWindow: 3,
	})
	if !errors.Is(err, ErrNotEnoughData) {
		t.Fatalf("err = %v, want ErrNotEnoughData", err)
	}
}

func TestRunScalpTPSL(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	prices := []string{"100", "100", "100", "100.5", "101", "101.8", "102.4", "101.7", "102.7", "103.6", "104.5", "103.8"}
	candles := make([]marketdata.Candle, 0, len(prices))
	for i, price := range prices {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "binance",
			MarketType: marketdata.MarketTypeSpot,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			Close:      price,
		})
	}

	result, err := RunScalpTPSL(candles, ScalpTPSLConfig{
		FastWindow:    2,
		SlowWindow:    3,
		TakeProfitPct: 0.6,
		StopLossPct:   0.4,
		FeeRate:       0.001,
		SlippageRate:  0.0005,
	})
	if err != nil {
		t.Fatalf("run scalp tpsl: %v", err)
	}
	if len(result.Trades) < 2 {
		t.Fatalf("trades length = %d, want at least 2 for short-term strategy", len(result.Trades))
	}
	if result.StrategyName != "scalp_tpsl_2_3_tp0.60_sl0.40" {
		t.Fatalf("strategy name = %q", result.StrategyName)
	}
}
