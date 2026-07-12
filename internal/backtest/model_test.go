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
