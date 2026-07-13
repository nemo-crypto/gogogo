package backtest

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"gogogo/internal/marketdata"
)

var ErrNotEnoughData = errors.New("not enough data")

type SMAConfig struct {
	FastWindow int
	SlowWindow int
	FeeRate    float64
}

type ScalpTPSLConfig struct {
	FastWindow           int
	SlowWindow           int
	TakeProfitPct        float64
	StopLossPct          float64
	DynamicTPSL          bool
	TakeProfitATRMult    float64
	StopLossATRMult      float64
	MinTakeProfitPct     float64
	MaxTakeProfitPct     float64
	MinStopLossPct       float64
	MaxStopLossPct       float64
	CooldownBars         int
	FeeRate              float64
	SlippageRate         float64
	AllowShort           bool
	MinTrendSpreadPct    float64
	ConfirmBars          int
	ATRWindow            int
	MinATRPct            float64
	MaxATRPct            float64
	VolumeWindow         int
	MinVolumeRatio       float64
	MaxEntryExtensionPct float64
	PullbackLookback     int
	PullbackTolerancePct float64
}

type Trade struct {
	EntryTime  time.Time
	ExitTime   time.Time
	EntryPrice float64
	ExitPrice  float64
	ReturnPct  float64
}

type Result struct {
	StrategyName     string
	Symbol           string
	Interval         string
	Start            time.Time
	End              time.Time
	InitialEquity    float64
	FinalEquity      float64
	TotalReturnPct   float64
	BuyHoldReturnPct float64
	ExcessReturnPct  float64
	MaxDrawdownPct   float64
	Trades           []Trade
	WinRatePct       float64
}

func RunSMACrossover(candles []marketdata.Candle, config SMAConfig) (Result, error) {
	// TODO(strategy): SMA crossover is a low-frequency baseline. Keep it for comparison,
	// but production paper/live strategy should use explicit TP/SL and shorter-horizon signals.
	if config.FastWindow <= 0 {
		return Result{}, errors.New("fast window must be positive")
	}
	if config.SlowWindow <= 0 {
		return Result{}, errors.New("slow window must be positive")
	}
	if config.FastWindow >= config.SlowWindow {
		return Result{}, errors.New("fast window must be less than slow window")
	}
	if config.FeeRate < 0 {
		return Result{}, errors.New("fee rate cannot be negative")
	}
	if len(candles) < config.SlowWindow+2 {
		return Result{}, ErrNotEnoughData
	}

	closes, err := closePrices(candles)
	if err != nil {
		return Result{}, err
	}

	equity := 1.0
	peak := equity
	maxDrawdown := 0.0
	inPosition := false
	entryPrice := 0.0
	entryTime := time.Time{}
	trades := make([]Trade, 0)

	for i := config.SlowWindow; i < len(candles)-1; i++ {
		fast := sma(closes, i, config.FastWindow)
		slow := sma(closes, i, config.SlowWindow)
		nextPrice := closes[i+1]

		if !inPosition && fast > slow {
			inPosition = true
			entryPrice = nextPrice
			entryTime = candles[i+1].OpenTime
			equity *= 1 - config.FeeRate
			continue
		}

		if inPosition && fast < slow {
			tradeReturn := (nextPrice - entryPrice) / entryPrice
			equity *= 1 + tradeReturn
			equity *= 1 - config.FeeRate

			trades = append(trades, Trade{
				EntryTime:  entryTime,
				ExitTime:   candles[i+1].OpenTime,
				EntryPrice: entryPrice,
				ExitPrice:  nextPrice,
				ReturnPct:  tradeReturn * 100,
			})

			inPosition = false
			entryPrice = 0
			entryTime = time.Time{}
		}

		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			drawdown := (peak - equity) / peak
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}

	if inPosition {
		last := candles[len(candles)-1]
		lastPrice := closes[len(closes)-1]
		tradeReturn := (lastPrice - entryPrice) / entryPrice
		equity *= 1 + tradeReturn
		equity *= 1 - config.FeeRate
		trades = append(trades, Trade{
			EntryTime:  entryTime,
			ExitTime:   last.OpenTime,
			EntryPrice: entryPrice,
			ExitPrice:  lastPrice,
			ReturnPct:  tradeReturn * 100,
		})
	}

	buyHoldReturn := (closes[len(closes)-1] - closes[0]) / closes[0] * 100
	totalReturn := (equity - 1) * 100

	return Result{
		StrategyName:     fmt.Sprintf("sma_crossover_%d_%d", config.FastWindow, config.SlowWindow),
		Symbol:           candles[0].Symbol,
		Interval:         candles[0].Interval,
		Start:            candles[0].OpenTime,
		End:              candles[len(candles)-1].OpenTime,
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

func RunScalpTPSL(candles []marketdata.Candle, config ScalpTPSLConfig) (Result, error) {
	config, err := normalizeScalpTPSLConfig(config)
	if err != nil {
		return Result{}, err
	}
	if len(candles) < config.SlowWindow+2 {
		return Result{}, ErrNotEnoughData
	}

	closes, err := closePrices(candles)
	if err != nil {
		return Result{}, err
	}
	highs, lows, volumes, err := scalpFilterSeries(candles, config, true)
	if err != nil {
		return Result{}, err
	}

	equity := 1.0
	peak := equity
	maxDrawdown := 0.0
	inPosition := false
	positionSide := "long"
	entryPrice := 0.0
	entryTime := time.Time{}
	takeProfitPrice := 0.0
	stopLossPrice := 0.0
	cooldownUntil := -1
	trades := make([]Trade, 0)
	costRate := config.FeeRate + config.SlippageRate

	updateDrawdown := func() {
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			drawdown := (peak - equity) / peak
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}
	closePosition := func(index int, exitPrice float64) {
		tradeReturn := (exitPrice - entryPrice) / entryPrice
		if positionSide == "short" {
			tradeReturn = (entryPrice - exitPrice) / entryPrice
		}
		equity *= 1 + tradeReturn
		equity *= 1 - costRate
		trades = append(trades, Trade{
			EntryTime:  entryTime,
			ExitTime:   candles[index].OpenTime,
			EntryPrice: entryPrice,
			ExitPrice:  exitPrice,
			ReturnPct:  tradeReturn * 100,
		})
		inPosition = false
		positionSide = "long"
		entryPrice = 0
		entryTime = time.Time{}
		takeProfitPrice = 0
		stopLossPrice = 0
		cooldownUntil = index + config.CooldownBars
		updateDrawdown()
	}

	for i := config.SlowWindow; i < len(candles)-1; i++ {
		fast := sma(closes, i, config.FastWindow)
		slow := sma(closes, i, config.SlowWindow)
		nextPrice := closes[i+1]

		if inPosition {
			if positionSide == "short" {
				// For a short position, profit is taken below entry and stop loss is above entry.
				if exitPrice, ok := scalpIntrabarExitPrice(positionSide, takeProfitPrice, stopLossPrice, highs[i], lows[i]); ok {
					closePosition(i, exitPrice)
					continue
				}
				switch {
				case fast > slow:
					closePosition(i+1, nextPrice)
					continue
				}
			} else {
				// For a long position, profit is taken above entry and stop loss is below entry.
				if exitPrice, ok := scalpIntrabarExitPrice(positionSide, takeProfitPrice, stopLossPrice, highs[i], lows[i]); ok {
					closePosition(i, exitPrice)
					continue
				}
				switch {
				case fast < slow:
					closePosition(i+1, nextPrice)
					continue
				}
			}
		}

		if !inPosition && i >= cooldownUntil {
			switch scalpSignalAt(closes, highs, lows, volumes, i, config) {
			case "long":
				tpPct, slPct, ok := scalpTPSLPercents(closes, highs, lows, i, config)
				if !ok {
					continue
				}
				inPosition = true
				positionSide = "long"
				entryPrice = nextPrice
				entryTime = candles[i+1].OpenTime
				takeProfitPrice = entryPrice * (1 + tpPct/100)
				stopLossPrice = entryPrice * (1 - slPct/100)
				equity *= 1 - costRate
				updateDrawdown()
				continue
			case "short":
				tpPct, slPct, ok := scalpTPSLPercents(closes, highs, lows, i, config)
				if !ok {
					continue
				}
				inPosition = true
				positionSide = "short"
				entryPrice = nextPrice
				entryTime = candles[i+1].OpenTime
				takeProfitPrice = entryPrice * (1 - tpPct/100)
				stopLossPrice = entryPrice * (1 + slPct/100)
				equity *= 1 - costRate
				updateDrawdown()
				continue
			}
		}

		updateDrawdown()
	}

	if inPosition {
		last := candles[len(candles)-1]
		lastPrice := closes[len(closes)-1]
		closePosition(len(candles)-1, lastPrice)
		trades[len(trades)-1].ExitTime = last.OpenTime
	}

	buyHoldReturn := (closes[len(closes)-1] - closes[0]) / closes[0] * 100
	totalReturn := (equity - 1) * 100
	return Result{
		StrategyName:     scalpStrategyName(config),
		Symbol:           candles[0].Symbol,
		Interval:         candles[0].Interval,
		Start:            candles[0].OpenTime,
		End:              candles[len(candles)-1].OpenTime,
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

func LatestScalpTPSLSignal(candles []marketdata.Candle, config ScalpTPSLConfig) (string, bool, error) {
	config, err := normalizeScalpTPSLConfig(config)
	if err != nil {
		return "", false, err
	}
	if len(candles) < config.SlowWindow+1 {
		return "", false, ErrNotEnoughData
	}
	closes, err := closePrices(candles)
	if err != nil {
		return "", false, err
	}
	highs, lows, volumes, err := scalpFilterSeries(candles, config, false)
	if err != nil {
		return "", false, err
	}
	side := scalpSignalAt(closes, highs, lows, volumes, len(closes)-1, config)
	if side == "" {
		return "", false, nil
	}
	return side, true, nil
}

func LatestScalpTPSLPercents(candles []marketdata.Candle, config ScalpTPSLConfig) (float64, float64, bool, error) {
	config, err := normalizeScalpTPSLConfig(config)
	if err != nil {
		return 0, 0, false, err
	}
	if len(candles) < config.SlowWindow+1 {
		return 0, 0, false, ErrNotEnoughData
	}
	closes, err := closePrices(candles)
	if err != nil {
		return 0, 0, false, err
	}
	highs, lows, _, err := scalpFilterSeries(candles, config, true)
	if err != nil {
		return 0, 0, false, err
	}
	takeProfitPct, stopLossPct, ok := scalpTPSLPercents(closes, highs, lows, len(closes)-1, config)
	return takeProfitPct, stopLossPct, ok, nil
}

func normalizeScalpTPSLConfig(config ScalpTPSLConfig) (ScalpTPSLConfig, error) {
	if config.FastWindow == 0 {
		config.FastWindow = 3
	}
	if config.SlowWindow == 0 {
		config.SlowWindow = 9
	}
	if config.TakeProfitPct == 0 {
		config.TakeProfitPct = 0.80
	}
	if config.StopLossPct == 0 {
		config.StopLossPct = 0.45
	}
	if config.ConfirmBars == 0 {
		config.ConfirmBars = 1
	}
	if config.ATRWindow == 0 && (config.MinATRPct > 0 || config.MaxATRPct > 0) {
		config.ATRWindow = 14
	}
	if config.DynamicTPSL {
		if config.ATRWindow == 0 {
			config.ATRWindow = 14
		}
		if config.TakeProfitATRMult == 0 {
			config.TakeProfitATRMult = 1.6
		}
		if config.StopLossATRMult == 0 {
			config.StopLossATRMult = 1.0
		}
		if config.MinTakeProfitPct == 0 {
			config.MinTakeProfitPct = config.TakeProfitPct
		}
		if config.MinStopLossPct == 0 {
			config.MinStopLossPct = config.StopLossPct
		}
	}
	if config.VolumeWindow == 0 && config.MinVolumeRatio > 0 {
		config.VolumeWindow = 20
	}
	if config.CooldownBars < 0 {
		return ScalpTPSLConfig{}, errors.New("cooldown bars cannot be negative")
	}
	switch {
	case config.FastWindow <= 0:
		return ScalpTPSLConfig{}, errors.New("fast window must be positive")
	case config.SlowWindow <= 0:
		return ScalpTPSLConfig{}, errors.New("slow window must be positive")
	case config.FastWindow >= config.SlowWindow:
		return ScalpTPSLConfig{}, errors.New("fast window must be less than slow window")
	case config.TakeProfitPct <= 0:
		return ScalpTPSLConfig{}, errors.New("take profit pct must be positive")
	case config.StopLossPct <= 0:
		return ScalpTPSLConfig{}, errors.New("stop loss pct must be positive")
	case config.DynamicTPSL && config.TakeProfitATRMult <= 0:
		return ScalpTPSLConfig{}, errors.New("take profit atr multiplier must be positive")
	case config.DynamicTPSL && config.StopLossATRMult <= 0:
		return ScalpTPSLConfig{}, errors.New("stop loss atr multiplier must be positive")
	case config.MinTakeProfitPct < 0:
		return ScalpTPSLConfig{}, errors.New("min take profit pct cannot be negative")
	case config.MaxTakeProfitPct < 0:
		return ScalpTPSLConfig{}, errors.New("max take profit pct cannot be negative")
	case config.MaxTakeProfitPct > 0 && config.MinTakeProfitPct > 0 && config.MaxTakeProfitPct < config.MinTakeProfitPct:
		return ScalpTPSLConfig{}, errors.New("max take profit pct must be greater than min take profit pct")
	case config.MinStopLossPct < 0:
		return ScalpTPSLConfig{}, errors.New("min stop loss pct cannot be negative")
	case config.MaxStopLossPct < 0:
		return ScalpTPSLConfig{}, errors.New("max stop loss pct cannot be negative")
	case config.MaxStopLossPct > 0 && config.MinStopLossPct > 0 && config.MaxStopLossPct < config.MinStopLossPct:
		return ScalpTPSLConfig{}, errors.New("max stop loss pct must be greater than min stop loss pct")
	case config.FeeRate < 0:
		return ScalpTPSLConfig{}, errors.New("fee rate cannot be negative")
	case config.SlippageRate < 0:
		return ScalpTPSLConfig{}, errors.New("slippage rate cannot be negative")
	case config.MinTrendSpreadPct < 0:
		return ScalpTPSLConfig{}, errors.New("min trend spread pct cannot be negative")
	case config.ConfirmBars < 0:
		return ScalpTPSLConfig{}, errors.New("confirm bars cannot be negative")
	case config.ATRWindow < 0:
		return ScalpTPSLConfig{}, errors.New("atr window cannot be negative")
	case config.MinATRPct < 0:
		return ScalpTPSLConfig{}, errors.New("min atr pct cannot be negative")
	case config.MaxATRPct < 0:
		return ScalpTPSLConfig{}, errors.New("max atr pct cannot be negative")
	case config.MaxATRPct > 0 && config.MinATRPct > 0 && config.MaxATRPct < config.MinATRPct:
		return ScalpTPSLConfig{}, errors.New("max atr pct must be greater than min atr pct")
	case config.VolumeWindow < 0:
		return ScalpTPSLConfig{}, errors.New("volume window cannot be negative")
	case config.MinVolumeRatio < 0:
		return ScalpTPSLConfig{}, errors.New("min volume ratio cannot be negative")
	case config.MaxEntryExtensionPct < 0:
		return ScalpTPSLConfig{}, errors.New("max entry extension pct cannot be negative")
	case config.PullbackLookback < 0:
		return ScalpTPSLConfig{}, errors.New("pullback lookback cannot be negative")
	case config.PullbackTolerancePct < 0:
		return ScalpTPSLConfig{}, errors.New("pullback tolerance pct cannot be negative")
	}
	return config, nil
}

func scalpStrategyName(config ScalpTPSLConfig) string {
	name := fmt.Sprintf("scalp_tpsl_%d_%d_tp%.2f_sl%.2f", config.FastWindow, config.SlowWindow, config.TakeProfitPct, config.StopLossPct)
	if config.DynamicTPSL {
		name += fmt.Sprintf("_atr_tp%.2fx_sl%.2fx", config.TakeProfitATRMult, config.StopLossATRMult)
	}
	if config.MinTrendSpreadPct > 0 || config.ConfirmBars > 1 || config.MinATRPct > 0 || config.MaxATRPct > 0 || config.MinVolumeRatio > 0 || config.MaxEntryExtensionPct > 0 || config.PullbackLookback > 0 {
		name += "_filtered"
	}
	return name
}

func scalpSignalAt(closes []float64, highs []float64, lows []float64, volumes []float64, index int, config ScalpTPSLConfig) string {
	if index <= 0 || index < config.SlowWindow || index >= len(closes) {
		return ""
	}
	currentPrice := closes[index]
	fast := sma(closes, index, config.FastWindow)
	slow := sma(closes, index, config.SlowWindow)
	if config.MinTrendSpreadPct > 0 {
		spreadPct := math.Abs(fast-slow) / currentPrice * 100
		if spreadPct < config.MinTrendSpreadPct {
			return ""
		}
	}
	if scalpUsesATR(config) {
		atrPct, ok := atrPercent(closes, highs, lows, index, config.ATRWindow)
		if !ok {
			return ""
		}
		if config.MinATRPct > 0 && atrPct < config.MinATRPct {
			return ""
		}
		if config.MaxATRPct > 0 && atrPct > config.MaxATRPct {
			return ""
		}
	}
	if scalpUsesVolume(config) {
		ratio, ok := volumeRatio(volumes, index, config.VolumeWindow)
		if !ok || ratio < config.MinVolumeRatio {
			return ""
		}
	}
	if fast > slow && confirmedDirection(closes, index, config.ConfirmBars, 1) {
		if !entryExtensionOK(currentPrice, fast, 1, config.MaxEntryExtensionPct) {
			return ""
		}
		if !recentPullbackOK(closes, highs, lows, index, config, 1) {
			return ""
		}
		return "long"
	}
	if config.AllowShort && fast < slow && confirmedDirection(closes, index, config.ConfirmBars, -1) {
		if !entryExtensionOK(currentPrice, fast, -1, config.MaxEntryExtensionPct) {
			return ""
		}
		if !recentPullbackOK(closes, highs, lows, index, config, -1) {
			return ""
		}
		return "short"
	}
	return ""
}

func scalpIntrabarExitPrice(positionSide string, takeProfitPrice float64, stopLossPrice float64, high float64, low float64) (float64, bool) {
	if positionSide == "short" {
		takeProfitHit := low <= takeProfitPrice
		stopLossHit := high >= stopLossPrice
		if stopLossHit {
			return stopLossPrice, true
		}
		if takeProfitHit {
			return takeProfitPrice, true
		}
		return 0, false
	}
	takeProfitHit := high >= takeProfitPrice
	stopLossHit := low <= stopLossPrice
	if stopLossHit {
		return stopLossPrice, true
	}
	if takeProfitHit {
		return takeProfitPrice, true
	}
	return 0, false
}

func scalpTPSLPercents(closes []float64, highs []float64, lows []float64, index int, config ScalpTPSLConfig) (float64, float64, bool) {
	takeProfitPct := config.TakeProfitPct
	stopLossPct := config.StopLossPct
	if config.DynamicTPSL {
		atrPct, ok := atrPercent(closes, highs, lows, index, config.ATRWindow)
		if !ok {
			return 0, 0, false
		}
		takeProfitPct = clampPositive(atrPct*config.TakeProfitATRMult, config.MinTakeProfitPct, config.MaxTakeProfitPct)
		stopLossPct = clampPositive(atrPct*config.StopLossATRMult, config.MinStopLossPct, config.MaxStopLossPct)
	}
	return takeProfitPct, stopLossPct, true
}

func clampPositive(value float64, minValue float64, maxValue float64) float64 {
	if minValue > 0 && value < minValue {
		value = minValue
	}
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	return value
}

func confirmedDirection(closes []float64, index int, bars int, direction int) bool {
	if bars <= 0 {
		bars = 1
	}
	if index-bars < 0 {
		return false
	}
	for offset := 0; offset < bars; offset++ {
		current := closes[index-offset]
		previous := closes[index-offset-1]
		if direction > 0 && current <= previous {
			return false
		}
		if direction < 0 && current >= previous {
			return false
		}
	}
	return true
}

func scalpFilterSeries(candles []marketdata.Candle, config ScalpTPSLConfig, requireRange bool) ([]float64, []float64, []float64, error) {
	var highs []float64
	var lows []float64
	var volumes []float64
	if requireRange || scalpUsesRangeData(config) {
		highs = make([]float64, 0, len(candles))
		lows = make([]float64, 0, len(candles))
		for _, candle := range candles {
			high, err := parsePositiveCandleField(candle.High, "high", candle)
			if err != nil {
				return nil, nil, nil, err
			}
			low, err := parsePositiveCandleField(candle.Low, "low", candle)
			if err != nil {
				return nil, nil, nil, err
			}
			if low > high {
				return nil, nil, nil, fmt.Errorf("invalid candle range %s %s", candle.Symbol, candle.OpenTime.Format(time.RFC3339))
			}
			highs = append(highs, high)
			lows = append(lows, low)
		}
	}
	if scalpUsesVolume(config) {
		volumes = make([]float64, 0, len(candles))
		for _, candle := range candles {
			volume, err := parseNonNegativeCandleField(candle.Volume, "volume", candle)
			if err != nil {
				return nil, nil, nil, err
			}
			volumes = append(volumes, volume)
		}
	}
	return highs, lows, volumes, nil
}

func scalpUsesRangeData(config ScalpTPSLConfig) bool {
	return config.DynamicTPSL || scalpUsesATR(config) || scalpUsesPullback(config)
}

func scalpUsesATR(config ScalpTPSLConfig) bool {
	return config.ATRWindow > 0 && (config.MinATRPct > 0 || config.MaxATRPct > 0)
}

func scalpUsesVolume(config ScalpTPSLConfig) bool {
	return config.VolumeWindow > 0 && config.MinVolumeRatio > 0
}

func scalpUsesPullback(config ScalpTPSLConfig) bool {
	return config.PullbackLookback > 0
}

func entryExtensionOK(price float64, fastAverage float64, direction int, maxPct float64) bool {
	if maxPct <= 0 {
		return true
	}
	if price <= 0 {
		return false
	}
	extensionPct := (price - fastAverage) / price * 100
	if direction < 0 {
		extensionPct = (fastAverage - price) / price * 100
	}
	return extensionPct <= maxPct
}

func recentPullbackOK(closes []float64, highs []float64, lows []float64, index int, config ScalpTPSLConfig, direction int) bool {
	if config.PullbackLookback <= 0 {
		return true
	}
	if len(highs) != len(closes) || len(lows) != len(closes) {
		return false
	}
	start := index - config.PullbackLookback + 1
	minStart := config.FastWindow - 1
	if start < minStart {
		start = minStart
	}
	if start > index {
		return false
	}
	tolerance := config.PullbackTolerancePct / 100
	for i := start; i <= index; i++ {
		fastAverage := sma(closes, i, config.FastWindow)
		if direction > 0 && lows[i] <= fastAverage*(1+tolerance) {
			return true
		}
		if direction < 0 && highs[i] >= fastAverage*(1-tolerance) {
			return true
		}
	}
	return false
}

func atrPercent(closes []float64, highs []float64, lows []float64, index int, window int) (float64, bool) {
	if window <= 0 || len(highs) != len(closes) || len(lows) != len(closes) || index-window+1 < 1 {
		return 0, false
	}
	total := 0.0
	for i := index - window + 1; i <= index; i++ {
		highLow := highs[i] - lows[i]
		highPrevClose := math.Abs(highs[i] - closes[i-1])
		lowPrevClose := math.Abs(lows[i] - closes[i-1])
		total += math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
	}
	atr := total / float64(window)
	if closes[index] <= 0 {
		return 0, false
	}
	return atr / closes[index] * 100, true
}

func volumeRatio(volumes []float64, index int, window int) (float64, bool) {
	if window <= 0 || len(volumes) <= index || index-window < 0 {
		return 0, false
	}
	total := 0.0
	for i := index - window; i < index; i++ {
		total += volumes[i]
	}
	avg := total / float64(window)
	if avg <= 0 {
		return 0, false
	}
	return volumes[index] / avg, true
}

func parsePositiveCandleField(value string, name string, candle marketdata.Candle) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid %s %s %s", name, candle.Symbol, candle.OpenTime.Format(time.RFC3339))
	}
	return parsed, nil
}

func parseNonNegativeCandleField(value string, name string, candle marketdata.Candle) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid %s %s %s", name, candle.Symbol, candle.OpenTime.Format(time.RFC3339))
	}
	return parsed, nil
}

func closePrices(candles []marketdata.Candle) ([]float64, error) {
	prices := make([]float64, 0, len(candles))
	for _, candle := range candles {
		price, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil {
			return nil, fmt.Errorf("parse close %s %s: %w", candle.Symbol, candle.OpenTime.Format(time.RFC3339), err)
		}
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return nil, fmt.Errorf("invalid close price %s %s", candle.Symbol, candle.OpenTime.Format(time.RFC3339))
		}
		prices = append(prices, price)
	}
	return prices, nil
}

func sma(values []float64, endInclusive int, window int) float64 {
	start := endInclusive - window + 1
	total := 0.0
	for i := start; i <= endInclusive; i++ {
		total += values[i]
	}
	return total / float64(window)
}

func winRate(trades []Trade) float64 {
	if len(trades) == 0 {
		return 0
	}
	wins := 0
	for _, trade := range trades {
		if trade.ReturnPct > 0 {
			wins++
		}
	}
	return float64(wins) / float64(len(trades)) * 100
}
