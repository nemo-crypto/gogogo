package backtest

import (
	"errors"
	"strconv"
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
			Exchange:   "onebullex",
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
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypeSpot,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			High:       price,
			Low:        price,
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

func TestRunScalpTPSLUsesIntrabarStopLossConservatively(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	candles := testBacktestCandlesWithRanges(start,
		[]float64{100, 100, 101, 102, 102},
		[]float64{100, 100, 101, 103, 102},
		[]float64{100, 100, 101, 99, 102},
	)

	result, err := RunScalpTPSL(candles, ScalpTPSLConfig{
		FastWindow:    1,
		SlowWindow:    2,
		TakeProfitPct: 0.5,
		StopLossPct:   0.5,
	})
	if err != nil {
		t.Fatalf("run scalp tpsl: %v", err)
	}
	if len(result.Trades) == 0 {
		t.Fatal("trades length = 0, want intrabar exit")
	}
	first := result.Trades[0]
	if first.ExitPrice != 101.49 {
		t.Fatalf("exit price = %f, want conservative stop loss 101.49", first.ExitPrice)
	}
	if first.ReturnPct >= 0 {
		t.Fatalf("return pct = %f, want stop loss", first.ReturnPct)
	}
}

func TestLatestScalpTPSLPercentsUsesATRAndClamps(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	candles := testBacktestCandlesWithRanges(start,
		[]float64{100, 101, 102, 103, 104, 105},
		[]float64{102, 103, 104, 105, 106, 107},
		[]float64{98, 99, 100, 101, 102, 103},
	)

	takeProfitPct, stopLossPct, ok, err := LatestScalpTPSLPercents(candles, ScalpTPSLConfig{
		FastWindow:        2,
		SlowWindow:        3,
		TakeProfitPct:     0.8,
		StopLossPct:       0.45,
		DynamicTPSL:       true,
		TakeProfitATRMult: 1.6,
		StopLossATRMult:   1.0,
		ATRWindow:         3,
		MinTakeProfitPct:  0.55,
		MaxTakeProfitPct:  1.4,
		MinStopLossPct:    0.3,
		MaxStopLossPct:    0.75,
	})
	if err != nil {
		t.Fatalf("latest tpsl percents: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if takeProfitPct != 1.4 {
		t.Fatalf("take profit pct = %f, want clamp 1.4", takeProfitPct)
	}
	if stopLossPct != 0.75 {
		t.Fatalf("stop loss pct = %f, want clamp 0.75", stopLossPct)
	}
}

func TestLatestScalpTPSLPercentsUsesCurrentFallbackDefaults(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	candles := testBacktestCandles(start, closes, nil)

	takeProfitPct, stopLossPct, ok, err := LatestScalpTPSLPercents(candles, ScalpTPSLConfig{})
	if err != nil {
		t.Fatalf("latest tpsl percents: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if takeProfitPct != 0.80 {
		t.Fatalf("take profit pct = %f, want 0.80", takeProfitPct)
	}
	if stopLossPct != 0.45 {
		t.Fatalf("stop loss pct = %f, want 0.45", stopLossPct)
	}
}

func TestLatestScalpTPSLSignalFiltersWeakTrend(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testBacktestCandles(start, []float64{100, 100.01, 100.02, 100.03, 100.04, 100.05}, nil)

	side, ok, err := LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:    2,
		SlowWindow:    3,
		TakeProfitPct: 0.6,
		StopLossPct:   0.3,
	})
	if err != nil {
		t.Fatalf("latest scalp signal: %v", err)
	}
	if !ok || side != "long" {
		t.Fatalf("side = %q ok=%v, want long", side, ok)
	}

	side, ok, err = LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:        2,
		SlowWindow:        3,
		TakeProfitPct:     0.6,
		StopLossPct:       0.3,
		MinTrendSpreadPct: 1,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with filter: %v", err)
	}
	if ok || side != "" {
		t.Fatalf("side = %q ok=%v, want filtered hold", side, ok)
	}
}

func TestLatestScalpTPSLSignalVolumeAndATRFilters(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	volumes := []float64{100, 100, 100, 100, 100, 240}
	candles := testBacktestCandles(start, []float64{100, 101, 102, 103, 104, 105}, volumes)

	side, ok, err := LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:     2,
		SlowWindow:     3,
		TakeProfitPct:  0.6,
		StopLossPct:    0.3,
		ATRWindow:      3,
		MinATRPct:      0.5,
		MaxATRPct:      5,
		VolumeWindow:   3,
		MinVolumeRatio: 1.5,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with filters: %v", err)
	}
	if !ok || side != "long" {
		t.Fatalf("side = %q ok=%v, want filtered long", side, ok)
	}

	side, ok, err = LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:     2,
		SlowWindow:     3,
		TakeProfitPct:  0.6,
		StopLossPct:    0.3,
		ATRWindow:      3,
		MinATRPct:      0.5,
		MaxATRPct:      5,
		VolumeWindow:   3,
		MinVolumeRatio: 3,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with strict volume filter: %v", err)
	}
	if ok || side != "" {
		t.Fatalf("side = %q ok=%v, want volume filtered hold", side, ok)
	}
}

func TestLatestScalpTPSLSignalEntryExtensionAndPullbackFilters(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	candles := testBacktestCandles(start, []float64{100, 101, 102, 103, 108, 112}, nil)

	side, ok, err := LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:           2,
		SlowWindow:           3,
		TakeProfitPct:        0.6,
		StopLossPct:          0.3,
		MaxEntryExtensionPct: 0.5,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with extension filter: %v", err)
	}
	if ok || side != "" {
		t.Fatalf("side = %q ok=%v, want overextended hold", side, ok)
	}

	candles = testBacktestCandlesWithRanges(start,
		[]float64{100, 101, 102, 103, 104, 105},
		[]float64{100.4, 101.4, 102.4, 103.4, 104.4, 105.4},
		[]float64{99.6, 100.6, 101.6, 102.6, 102.8, 104.6},
	)
	side, ok, err = LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:           2,
		SlowWindow:           3,
		TakeProfitPct:        0.6,
		StopLossPct:          0.3,
		MaxEntryExtensionPct: 1,
		PullbackLookback:     3,
		PullbackTolerancePct: 0.1,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with pullback filter: %v", err)
	}
	if !ok || side != "long" {
		t.Fatalf("side = %q ok=%v, want pullback-confirmed long", side, ok)
	}

	candles = testBacktestCandlesWithRanges(start,
		[]float64{100, 101, 102, 103, 104, 105},
		[]float64{100.4, 101.4, 102.4, 103.4, 104.4, 105.4},
		[]float64{99.6, 100.6, 101.6, 102.6, 103.6, 104.6},
	)
	side, ok, err = LatestScalpTPSLSignal(candles, ScalpTPSLConfig{
		FastWindow:           2,
		SlowWindow:           3,
		TakeProfitPct:        0.6,
		StopLossPct:          0.3,
		MaxEntryExtensionPct: 1,
		PullbackLookback:     2,
		PullbackTolerancePct: 0,
	})
	if err != nil {
		t.Fatalf("latest scalp signal with missing pullback: %v", err)
	}
	if ok || side != "" {
		t.Fatalf("side = %q ok=%v, want missing-pullback hold", side, ok)
	}
}

func testBacktestCandles(start time.Time, closes []float64, volumes []float64) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(closes))
	for i, closePrice := range closes {
		openTime := start.Add(time.Duration(i) * time.Minute)
		volume := 100.0
		if len(volumes) > i {
			volume = volumes[i]
		}
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			High:       formatTestFloat(closePrice + 1),
			Low:        formatTestFloat(closePrice - 1),
			Close:      formatTestFloat(closePrice),
			Volume:     formatTestFloat(volume),
		})
	}
	return candles
}

func testBacktestCandlesWithRanges(start time.Time, closes []float64, highs []float64, lows []float64) []marketdata.Candle {
	candles := make([]marketdata.Candle, 0, len(closes))
	for i, closePrice := range closes {
		openTime := start.Add(time.Duration(i) * time.Minute)
		candles = append(candles, marketdata.Candle{
			Exchange:   "onebullex",
			MarketType: marketdata.MarketTypePerpetual,
			Symbol:     "BTCUSDT",
			Interval:   "1m",
			OpenTime:   openTime,
			CloseTime:  openTime.Add(time.Minute),
			High:       formatTestFloat(highs[i]),
			Low:        formatTestFloat(lows[i]),
			Close:      formatTestFloat(closePrice),
			Volume:     "100",
		})
	}
	return candles
}

func formatTestFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
