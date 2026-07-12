package backtest

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gogogo/internal/marketdata"
)

type AdaptiveTrendConfig struct {
	MomentumWindow      int
	TrendWindow         int
	BreakoutWindow      int
	VolatilityWindow    int
	RebalanceWindow     int
	TopN                int
	TargetVolatilityPct float64
	MaxPositionPct      float64
	TrailingStopPct     float64
	MaxFundingRatePct   float64
	FeeRate             float64
	SlippageRate        float64
}

type adaptiveBar struct {
	Time  time.Time
	High  float64
	Low   float64
	Close float64
}

type adaptivePosition struct {
	Symbol     string
	Weight     float64
	EntryTime  time.Time
	EntryPrice float64
	PeakPrice  float64
}

type adaptiveCandidate struct {
	Symbol        string
	Score         float64
	MomentumPct   float64
	VolatilityPct float64
	FundingPct    float64
	Breakout      bool
}

func RunAdaptiveTrendRotation(candlesBySymbol map[string][]marketdata.Candle, fundingBySymbol map[string][]marketdata.FundingRate, config AdaptiveTrendConfig) (Result, error) {
	config, err := normalizeAdaptiveTrendConfig(config)
	if err != nil {
		return Result{}, err
	}
	aligned, symbols, err := alignAdaptiveCandles(candlesBySymbol)
	if err != nil {
		return Result{}, err
	}

	maxWindow := maxInt(config.MomentumWindow, config.TrendWindow, config.BreakoutWindow, config.VolatilityWindow)
	if len(aligned[symbols[0]]) < maxWindow+2 {
		return Result{}, ErrNotEnoughData
	}

	equity := 1.0
	peakEquity := equity
	maxDrawdown := 0.0
	positions := make(map[string]*adaptivePosition)
	trades := make([]Trade, 0)
	lastRebalance := -config.RebalanceWindow
	costRate := config.FeeRate + config.SlippageRate
	startIndex := maxWindow
	endIndex := len(aligned[symbols[0]]) - 1

	updateDrawdown := func() {
		if equity > peakEquity {
			peakEquity = equity
		}
		if peakEquity > 0 {
			drawdown := (peakEquity - equity) / peakEquity
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}

	closePosition := func(symbol string, index int) {
		position, ok := positions[symbol]
		if !ok {
			return
		}
		price := aligned[symbol][index].Close
		if price <= 0 || position.EntryPrice <= 0 {
			delete(positions, symbol)
			return
		}
		equity *= 1 - position.Weight*costRate
		trades = append(trades, Trade{
			EntryTime:  position.EntryTime,
			ExitTime:   aligned[symbol][index].Time,
			EntryPrice: position.EntryPrice,
			ExitPrice:  price,
			ReturnPct:  (price - position.EntryPrice) / position.EntryPrice * 100,
		})
		delete(positions, symbol)
		updateDrawdown()
	}

	for i := startIndex; i < endIndex; i++ {
		for symbol, position := range positions {
			bar := aligned[symbol][i]
			if bar.Close > position.PeakPrice {
				position.PeakPrice = bar.Close
			}
			stopPrice := position.PeakPrice * (1 - config.TrailingStopPct/100)
			if bar.Close <= stopPrice {
				closePosition(symbol, i)
			}
		}

		if i-lastRebalance >= config.RebalanceWindow {
			candidates := rankAdaptiveCandidates(aligned, fundingBySymbol, symbols, i, config)
			targets := targetAdaptiveWeights(candidates, config)
			for symbol := range positions {
				if _, ok := targets[symbol]; !ok {
					closePosition(symbol, i)
				}
			}
			for symbol, weight := range targets {
				if position, ok := positions[symbol]; ok {
					position.Weight = weight
					continue
				}
				price := aligned[symbol][i].Close
				equity *= 1 - weight*costRate
				positions[symbol] = &adaptivePosition{
					Symbol:     symbol,
					Weight:     weight,
					EntryTime:  aligned[symbol][i].Time,
					EntryPrice: price,
					PeakPrice:  price,
				}
				updateDrawdown()
			}
			lastRebalance = i
		}

		periodReturn := 0.0
		for symbol, position := range positions {
			current := aligned[symbol][i].Close
			next := aligned[symbol][i+1].Close
			if current <= 0 {
				continue
			}
			periodReturn += position.Weight * ((next - current) / current)
		}
		equity *= 1 + periodReturn
		updateDrawdown()
	}

	for symbol := range positions {
		closePosition(symbol, endIndex)
	}

	buyHoldReturn := equalWeightBuyHoldReturn(aligned, symbols, startIndex, endIndex)
	totalReturn := (equity - 1) * 100
	return Result{
		StrategyName:     fmt.Sprintf("adaptive_trend_rotation_%d_%d", config.MomentumWindow, config.TrendWindow),
		Symbol:           strings.Join(symbols, ","),
		Interval:         candlesBySymbol[symbols[0]][0].Interval,
		Start:            aligned[symbols[0]][startIndex].Time,
		End:              aligned[symbols[0]][endIndex].Time,
		InitialEquity:    1,
		FinalEquity:      equity,
		TotalReturnPct:   totalReturn,
		BuyHoldReturnPct: buyHoldReturn,
		ExcessReturnPct:  totalReturn - buyHoldReturn,
		MaxDrawdownPct:   maxDrawdown * 100,
		Trades:           trades,
		WinRatePct:       winRate(trades),
	}, nil
}

func normalizeAdaptiveTrendConfig(config AdaptiveTrendConfig) (AdaptiveTrendConfig, error) {
	if config.MomentumWindow == 0 {
		config.MomentumWindow = 24
	}
	if config.TrendWindow == 0 {
		config.TrendWindow = 72
	}
	if config.BreakoutWindow == 0 {
		config.BreakoutWindow = 48
	}
	if config.VolatilityWindow == 0 {
		config.VolatilityWindow = 24
	}
	if config.RebalanceWindow == 0 {
		config.RebalanceWindow = 6
	}
	if config.TopN == 0 {
		config.TopN = 2
	}
	if config.TargetVolatilityPct == 0 {
		config.TargetVolatilityPct = 1
	}
	if config.MaxPositionPct == 0 {
		config.MaxPositionPct = 30
	}
	if config.TrailingStopPct == 0 {
		config.TrailingStopPct = 6
	}
	if config.MaxFundingRatePct == 0 {
		config.MaxFundingRatePct = 0.05
	}
	switch {
	case config.MomentumWindow <= 0:
		return AdaptiveTrendConfig{}, errors.New("momentum window must be positive")
	case config.TrendWindow <= 0:
		return AdaptiveTrendConfig{}, errors.New("trend window must be positive")
	case config.BreakoutWindow <= 0:
		return AdaptiveTrendConfig{}, errors.New("breakout window must be positive")
	case config.VolatilityWindow <= 1:
		return AdaptiveTrendConfig{}, errors.New("volatility window must be greater than 1")
	case config.RebalanceWindow <= 0:
		return AdaptiveTrendConfig{}, errors.New("rebalance window must be positive")
	case config.TopN <= 0:
		return AdaptiveTrendConfig{}, errors.New("topN must be positive")
	case config.TargetVolatilityPct <= 0:
		return AdaptiveTrendConfig{}, errors.New("target volatility pct must be positive")
	case config.MaxPositionPct <= 0 || config.MaxPositionPct > 100:
		return AdaptiveTrendConfig{}, errors.New("max position pct must be within 0 and 100")
	case config.TrailingStopPct <= 0 || config.TrailingStopPct >= 100:
		return AdaptiveTrendConfig{}, errors.New("trailing stop pct must be within 0 and 100")
	case config.MaxFundingRatePct < 0:
		return AdaptiveTrendConfig{}, errors.New("max funding rate pct cannot be negative")
	case config.FeeRate < 0:
		return AdaptiveTrendConfig{}, errors.New("fee rate cannot be negative")
	case config.SlippageRate < 0:
		return AdaptiveTrendConfig{}, errors.New("slippage rate cannot be negative")
	}
	return config, nil
}

func alignAdaptiveCandles(candlesBySymbol map[string][]marketdata.Candle) (map[string][]adaptiveBar, []string, error) {
	if len(candlesBySymbol) == 0 {
		return nil, nil, ErrNotEnoughData
	}
	symbols := make([]string, 0, len(candlesBySymbol))
	for symbol := range candlesBySymbol {
		if len(candlesBySymbol[symbol]) == 0 {
			continue
		}
		symbols = append(symbols, strings.ToUpper(strings.TrimSpace(symbol)))
	}
	sort.Strings(symbols)
	if len(symbols) == 0 {
		return nil, nil, ErrNotEnoughData
	}

	base := symbols[0]
	baseTimes := make([]time.Time, 0, len(candlesBySymbol[base]))
	for _, candle := range candlesBySymbol[base] {
		baseTimes = append(baseTimes, candle.OpenTime)
	}

	bySymbolTime := make(map[string]map[time.Time]marketdata.Candle, len(symbols))
	for _, symbol := range symbols {
		byTime := make(map[time.Time]marketdata.Candle, len(candlesBySymbol[symbol]))
		for _, candle := range candlesBySymbol[symbol] {
			byTime[candle.OpenTime] = candle
		}
		bySymbolTime[symbol] = byTime
	}

	aligned := make(map[string][]adaptiveBar, len(symbols))
	for _, symbol := range symbols {
		aligned[symbol] = make([]adaptiveBar, 0, len(baseTimes))
	}

	for _, at := range baseTimes {
		row := make(map[string]adaptiveBar, len(symbols))
		ok := true
		for _, symbol := range symbols {
			candle, exists := bySymbolTime[symbol][at]
			if !exists {
				ok = false
				break
			}
			bar, err := adaptiveBarFromCandle(candle)
			if err != nil {
				return nil, nil, err
			}
			row[symbol] = bar
		}
		if !ok {
			continue
		}
		for _, symbol := range symbols {
			aligned[symbol] = append(aligned[symbol], row[symbol])
		}
	}
	if len(aligned[base]) == 0 {
		return nil, nil, ErrNotEnoughData
	}
	return aligned, symbols, nil
}

func adaptiveBarFromCandle(candle marketdata.Candle) (adaptiveBar, error) {
	high, err := strconv.ParseFloat(candle.High, 64)
	if err != nil {
		return adaptiveBar{}, err
	}
	low, err := strconv.ParseFloat(candle.Low, 64)
	if err != nil {
		return adaptiveBar{}, err
	}
	closePrice, err := strconv.ParseFloat(candle.Close, 64)
	if err != nil {
		return adaptiveBar{}, err
	}
	if high <= 0 || low <= 0 || closePrice <= 0 {
		return adaptiveBar{}, errors.New("adaptive trend requires positive OHLC prices")
	}
	return adaptiveBar{Time: candle.OpenTime, High: high, Low: low, Close: closePrice}, nil
}

func rankAdaptiveCandidates(aligned map[string][]adaptiveBar, fundingBySymbol map[string][]marketdata.FundingRate, symbols []string, index int, config AdaptiveTrendConfig) []adaptiveCandidate {
	candidates := make([]adaptiveCandidate, 0, len(symbols))
	for _, symbol := range symbols {
		bars := aligned[symbol]
		closePrice := bars[index].Close
		trend := smaAdaptiveClose(bars, index, config.TrendWindow)
		if closePrice <= trend {
			continue
		}
		momentumPct := (closePrice - bars[index-config.MomentumWindow].Close) / bars[index-config.MomentumWindow].Close * 100
		if momentumPct <= 0 {
			continue
		}
		volatilityPct := realizedVolatilityPct(bars, index, config.VolatilityWindow)
		if volatilityPct <= 0 || math.IsNaN(volatilityPct) || math.IsInf(volatilityPct, 0) {
			continue
		}
		fundingPct, hasFunding := latestFundingPct(fundingBySymbol[symbol], bars[index].Time)
		if hasFunding && config.MaxFundingRatePct > 0 && fundingPct > config.MaxFundingRatePct {
			continue
		}
		breakout := closePrice >= highestHigh(bars, index, config.BreakoutWindow)*0.995
		score := momentumPct / math.Max(volatilityPct, 0.01)
		if breakout {
			score += 1
		}
		candidates = append(candidates, adaptiveCandidate{
			Symbol:        symbol,
			Score:         score,
			MomentumPct:   momentumPct,
			VolatilityPct: volatilityPct,
			FundingPct:    fundingPct,
			Breakout:      breakout,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].MomentumPct > candidates[j].MomentumPct
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > config.TopN {
		candidates = candidates[:config.TopN]
	}
	return candidates
}

func targetAdaptiveWeights(candidates []adaptiveCandidate, config AdaptiveTrendConfig) map[string]float64 {
	targets := make(map[string]float64, len(candidates))
	for _, candidate := range candidates {
		weightPct := config.TargetVolatilityPct / math.Max(candidate.VolatilityPct, 0.01) * 100
		if weightPct > config.MaxPositionPct {
			weightPct = config.MaxPositionPct
		}
		if weightPct <= 0 {
			continue
		}
		targets[candidate.Symbol] = weightPct / 100
	}
	return targets
}

func smaAdaptiveClose(bars []adaptiveBar, endInclusive int, window int) float64 {
	start := endInclusive - window + 1
	total := 0.0
	for i := start; i <= endInclusive; i++ {
		total += bars[i].Close
	}
	return total / float64(window)
}

func realizedVolatilityPct(bars []adaptiveBar, endInclusive int, window int) float64 {
	start := endInclusive - window + 1
	returns := make([]float64, 0, window)
	for i := start; i <= endInclusive; i++ {
		previous := bars[i-1].Close
		current := bars[i].Close
		if previous <= 0 || current <= 0 {
			continue
		}
		returns = append(returns, math.Log(current/previous))
	}
	if len(returns) < 2 {
		return 0
	}
	mean := 0.0
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, value := range returns {
		diff := value - mean
		variance += diff * diff
	}
	variance /= float64(len(returns) - 1)
	return math.Sqrt(variance) * 100
}

func highestHigh(bars []adaptiveBar, endInclusive int, window int) float64 {
	start := endInclusive - window + 1
	highest := 0.0
	for i := start; i <= endInclusive; i++ {
		if bars[i].High > highest {
			highest = bars[i].High
		}
	}
	return highest
}

func latestFundingPct(rates []marketdata.FundingRate, at time.Time) (float64, bool) {
	if len(rates) == 0 {
		return 0, false
	}
	latestIndex := sort.Search(len(rates), func(i int) bool {
		return rates[i].FundingTime.After(at)
	}) - 1
	if latestIndex < 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(rates[latestIndex].FundingRate, 64)
	if err != nil {
		return 0, false
	}
	return value * 100, true
}

func equalWeightBuyHoldReturn(aligned map[string][]adaptiveBar, symbols []string, startIndex int, endIndex int) float64 {
	if len(symbols) == 0 {
		return 0
	}
	total := 0.0
	for _, symbol := range symbols {
		start := aligned[symbol][startIndex].Close
		end := aligned[symbol][endIndex].Close
		if start <= 0 {
			continue
		}
		total += (end - start) / start
	}
	return total / float64(len(symbols)) * 100
}

func maxInt(values ...int) int {
	maxValue := values[0]
	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}
