package strategy

import (
	"errors"
	"sort"
	"strconv"

	"gogogo/internal/marketdata"
)

type RotationCandidate struct {
	Symbol    string
	ReturnPct float64
}

func MomentumRotation(candlesBySymbol map[string][]marketdata.Candle, lookback int, topN int) ([]RotationCandidate, error) {
	if lookback <= 0 {
		return nil, errors.New("lookback must be positive")
	}
	if topN <= 0 {
		return nil, errors.New("topN must be positive")
	}
	candidates := make([]RotationCandidate, 0, len(candlesBySymbol))
	for symbol, candles := range candlesBySymbol {
		if len(candles) <= lookback {
			continue
		}
		start, err := strconv.ParseFloat(candles[len(candles)-lookback-1].Close, 64)
		if err != nil {
			return nil, err
		}
		end, err := strconv.ParseFloat(candles[len(candles)-1].Close, 64)
		if err != nil {
			return nil, err
		}
		if start <= 0 {
			continue
		}
		candidates = append(candidates, RotationCandidate{Symbol: symbol, ReturnPct: (end - start) / start * 100})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].ReturnPct > candidates[j].ReturnPct
	})
	if len(candidates) > topN {
		candidates = candidates[:topN]
	}
	return candidates, nil
}

func MeanReversionSignal(candles []marketdata.Candle, window int, thresholdPct float64) (SignalAction, error) {
	if window <= 0 {
		return SignalHold, errors.New("window must be positive")
	}
	if len(candles) < window+1 {
		return SignalHold, nil
	}
	total := 0.0
	for _, candle := range candles[len(candles)-window-1 : len(candles)-1] {
		price, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil {
			return SignalHold, err
		}
		total += price
	}
	mean := total / float64(window)
	latest, err := strconv.ParseFloat(candles[len(candles)-1].Close, 64)
	if err != nil {
		return SignalHold, err
	}
	deviation := (latest - mean) / mean * 100
	if deviation <= -thresholdPct {
		return SignalBuy, nil
	}
	if deviation >= thresholdPct {
		return SignalSell, nil
	}
	return SignalHold, nil
}

func HedgeRatio(spotNotional float64, targetNetExposurePct float64) float64 {
	if spotNotional <= 0 {
		return 0
	}
	if targetNetExposurePct < 0 {
		targetNetExposurePct = 0
	}
	if targetNetExposurePct > 100 {
		targetNetExposurePct = 100
	}
	return spotNotional * (1 - targetNetExposurePct/100)
}
