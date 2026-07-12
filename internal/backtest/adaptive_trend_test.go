package backtest

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gogogo/internal/marketdata"
)

func TestRunAdaptiveTrendRotation(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	candlesBySymbol := map[string][]marketdata.Candle{
		"BTCUSDT": adaptiveTestCandles("BTCUSDT", start, 100, 1.008, 120),
		"ETHUSDT": adaptiveTestCandles("ETHUSDT", start, 100, 1.003, 120),
		"SOLUSDT": adaptiveTestCandles("SOLUSDT", start, 100, 0.998, 120),
	}
	fundingBySymbol := map[string][]marketdata.FundingRate{
		"BTCUSDT": {{Exchange: "binance", Symbol: "BTCUSDT", FundingTime: start.Add(24 * time.Hour), FundingRate: "0.0001", MarkPrice: "100"}},
	}

	result, err := RunAdaptiveTrendRotation(candlesBySymbol, fundingBySymbol, AdaptiveTrendConfig{
		MomentumWindow:      12,
		TrendWindow:         24,
		BreakoutWindow:      12,
		VolatilityWindow:    12,
		RebalanceWindow:     6,
		TopN:                2,
		TargetVolatilityPct: 1,
		MaxPositionPct:      40,
		TrailingStopPct:     10,
		MaxFundingRatePct:   0.05,
		FeeRate:             0.001,
		SlippageRate:        0.0005,
	})
	if err != nil {
		t.Fatalf("run adaptive trend: %v", err)
	}
	if result.StrategyName != "adaptive_trend_rotation_12_24" {
		t.Fatalf("strategy name = %q", result.StrategyName)
	}
	if len(result.Trades) == 0 {
		t.Fatal("trades length = 0, want strategy to enter and exit positions")
	}
	if result.Symbol != "BTCUSDT,ETHUSDT,SOLUSDT" {
		t.Fatalf("symbols = %q", result.Symbol)
	}
	if result.FinalEquity <= 1 {
		t.Fatalf("final equity = %f, want profitable test path", result.FinalEquity)
	}
}

func TestRunAdaptiveTrendRotationNotEnoughData(t *testing.T) {
	t.Parallel()

	_, err := RunAdaptiveTrendRotation(map[string][]marketdata.Candle{
		"BTCUSDT": adaptiveTestCandles("BTCUSDT", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 100, 1.01, 5),
	}, nil, AdaptiveTrendConfig{MomentumWindow: 12, TrendWindow: 24})
	if !errors.Is(err, ErrNotEnoughData) {
		t.Fatalf("err = %v, want ErrNotEnoughData", err)
	}
}

func adaptiveTestCandles(symbol string, start time.Time, initial float64, drift float64, count int) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, count)
	price := initial
	for i := 0; i < count; i++ {
		price *= drift
		openTime := start.Add(time.Duration(i) * time.Hour)
		high := price * 1.002
		low := price * 0.998
		candles = append(candles, marketdata.Candle{
			Exchange:   "binance",
			MarketType: marketdata.MarketTypeSpot,
			Symbol:     symbol,
			Interval:   "1h",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Hour),
			High:       fmt.Sprintf("%.8f", high),
			Low:        fmt.Sprintf("%.8f", low),
			Close:      fmt.Sprintf("%.8f", price),
		})
	}
	return candles
}
