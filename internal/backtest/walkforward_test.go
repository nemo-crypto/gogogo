package backtest

import (
	"strconv"
	"testing"
	"time"

	"gogogo/internal/marketdata"
)

func TestRunWalkForward(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]marketdata.Candle, 0, 80)
	for i := 0; i < 80; i++ {
		openTime := start.Add(time.Duration(i) * time.Hour)
		price := 100.0 + float64(i%20)
		if i >= 40 {
			price = 120.0 - float64(i%20)
		}
		candles = append(candles, marketdata.Candle{
			Exchange:   "binance",
			MarketType: marketdata.MarketTypeSpot,
			Symbol:     "BTCUSDT",
			Interval:   "1h",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Hour),
			Close:      formatFloat(price),
		})
	}

	result, err := RunWalkForward(candles, WalkForwardConfig{
		TrainWindow: 30,
		TestWindow:  20,
		Configs: []SMAConfig{
			{FastWindow: 2, SlowWindow: 5, FeeRate: 0.001},
			{FastWindow: 3, SlowWindow: 8, FeeRate: 0.001},
		},
	})
	if err != nil {
		t.Fatalf("run walk-forward: %v", err)
	}
	if len(result.Steps) == 0 {
		t.Fatal("steps length = 0, want at least one step")
	}
	if result.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", result.Symbol)
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 4, 64)
}
