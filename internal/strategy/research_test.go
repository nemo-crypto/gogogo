package strategy

import (
	"testing"
	"time"

	"gogogo/internal/marketdata"
)

func TestMomentumRotation(t *testing.T) {
	t.Parallel()

	candidates, err := MomentumRotation(map[string][]marketdata.Candle{
		"BTCUSDT": candles("BTCUSDT", 100, 105),
		"SOLUSDT": candles("SOLUSDT", 100, 120),
	}, 1, 1)
	if err != nil {
		t.Fatalf("rotation: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Symbol != "SOLUSDT" {
		t.Fatalf("candidates = %+v, want SOLUSDT", candidates)
	}
}

func TestMeanReversionSignal(t *testing.T) {
	t.Parallel()

	action, err := MeanReversionSignal(candles("BTCUSDT", 100, 100, 90), 2, 5)
	if err != nil {
		t.Fatalf("signal: %v", err)
	}
	if action != SignalBuy {
		t.Fatalf("action = %s, want buy", action)
	}
}

func TestHedgeRatio(t *testing.T) {
	t.Parallel()

	if hedge := HedgeRatio(1000, 25); hedge != 750 {
		t.Fatalf("hedge = %f, want 750", hedge)
	}
}

func candles(symbol string, closes ...float64) []marketdata.Candle {
	out := make([]marketdata.Candle, 0, len(closes))
	start := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	for i, close := range closes {
		out = append(out, marketdata.Candle{
			Exchange:    "binance",
			MarketType:  marketdata.MarketTypeSpot,
			Symbol:      symbol,
			Interval:    "1h",
			OpenTime:    start.Add(time.Duration(i) * time.Hour),
			CloseTime:   start.Add(time.Duration(i+1) * time.Hour),
			Open:        "100",
			High:        "120",
			Low:         "80",
			Close:       formatClose(close),
			Volume:      "1",
			QuoteVolume: "100",
			Source:      "unit",
		})
	}
	return out
}

func formatClose(value float64) string {
	switch value {
	case 90:
		return "90"
	case 100:
		return "100"
	case 105:
		return "105"
	case 120:
		return "120"
	default:
		return "100"
	}
}
