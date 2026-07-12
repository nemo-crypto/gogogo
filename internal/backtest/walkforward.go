package backtest

import (
	"errors"
	"time"

	"gogogo/internal/marketdata"
)

type WalkForwardConfig struct {
	TrainWindow int
	TestWindow  int
	Configs     []SMAConfig
}

type WalkForwardStep struct {
	TrainStart time.Time
	TrainEnd   time.Time
	TestStart  time.Time
	TestEnd    time.Time
	Config     SMAConfig
	Train      Result
	Test       Result
}

type WalkForwardResult struct {
	Symbol              string
	Interval            string
	Steps               []WalkForwardStep
	AverageTestReturn   float64
	AverageExcessReturn float64
	WinningStepsPct     float64
}

func RunWalkForward(candles []marketdata.Candle, config WalkForwardConfig) (WalkForwardResult, error) {
	if config.TrainWindow <= 0 {
		return WalkForwardResult{}, errors.New("train window must be positive")
	}
	if config.TestWindow <= 0 {
		return WalkForwardResult{}, errors.New("test window must be positive")
	}
	if len(config.Configs) == 0 {
		return WalkForwardResult{}, errors.New("at least one SMA config is required")
	}
	if len(candles) < config.TrainWindow+config.TestWindow {
		return WalkForwardResult{}, ErrNotEnoughData
	}

	steps := make([]WalkForwardStep, 0)
	for trainStart := 0; trainStart+config.TrainWindow+config.TestWindow <= len(candles); trainStart += config.TestWindow {
		trainEnd := trainStart + config.TrainWindow
		testEnd := trainEnd + config.TestWindow

		bestConfig, trainResult, ok := bestConfig(candles[trainStart:trainEnd], config.Configs)
		if !ok {
			continue
		}

		testResult, err := RunSMACrossover(candles[trainEnd:testEnd], bestConfig)
		if err != nil {
			if errors.Is(err, ErrNotEnoughData) {
				continue
			}
			return WalkForwardResult{}, err
		}

		steps = append(steps, WalkForwardStep{
			TrainStart: candles[trainStart].OpenTime,
			TrainEnd:   candles[trainEnd-1].OpenTime,
			TestStart:  candles[trainEnd].OpenTime,
			TestEnd:    candles[testEnd-1].OpenTime,
			Config:     bestConfig,
			Train:      trainResult,
			Test:       testResult,
		})
	}
	if len(steps) == 0 {
		return WalkForwardResult{}, ErrNotEnoughData
	}

	return summarizeWalkForward(candles, steps), nil
}

func bestConfig(candles []marketdata.Candle, configs []SMAConfig) (SMAConfig, Result, bool) {
	var best SMAConfig
	var bestResult Result
	ok := false
	for _, config := range configs {
		result, err := RunSMACrossover(candles, config)
		if err != nil {
			continue
		}
		if !ok || result.ExcessReturnPct > bestResult.ExcessReturnPct {
			best = config
			bestResult = result
			ok = true
		}
	}
	return best, bestResult, ok
}

func summarizeWalkForward(candles []marketdata.Candle, steps []WalkForwardStep) WalkForwardResult {
	totalTestReturn := 0.0
	totalExcessReturn := 0.0
	winners := 0
	for _, step := range steps {
		totalTestReturn += step.Test.TotalReturnPct
		totalExcessReturn += step.Test.ExcessReturnPct
		if step.Test.TotalReturnPct > 0 {
			winners++
		}
	}

	return WalkForwardResult{
		Symbol:              candles[0].Symbol,
		Interval:            candles[0].Interval,
		Steps:               steps,
		AverageTestReturn:   totalTestReturn / float64(len(steps)),
		AverageExcessReturn: totalExcessReturn / float64(len(steps)),
		WinningStepsPct:     float64(winners) / float64(len(steps)) * 100,
	}
}
