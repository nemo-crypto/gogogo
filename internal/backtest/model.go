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
