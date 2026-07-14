package main

import (
	"context"
	"errors"
	"fmt"
	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
	"math"
	"strconv"
	"strings"
	"time"
)

func latestPaperMarketSnapshot(ctx context.Context, repo *marketdata.SQLiteRepository, config paperRunConfig, candles []marketdata.Candle) (paperMarketSnapshot, error) {
	candlePrice, candleTime, err := latestCandlePrice(candles)
	if err != nil {
		return paperMarketSnapshot{}, err
	}
	snapshot := paperMarketSnapshot{
		Price:             candlePrice,
		PriceTime:         candleTime,
		PriceSource:       "candle_close_fallback",
		CandleClosePrice:  candlePrice,
		CandleCloseTime:   candleTime,
		FundingRateSource: "missing",
	}

	mark, err := repo.LatestMarkPrice(ctx, config.Exchange, config.Symbol)
	if err == nil {
		markPrice, parseErr := parsePositiveFloat(mark.MarkPrice, "latest mark price")
		if parseErr != nil {
			return paperMarketSnapshot{}, parseErr
		}
		snapshot.Price = markPrice
		snapshot.PriceTime = mark.EventTime
		snapshot.PriceSource = "latest_mark_price"
		snapshot.MarkPrice = markPrice
		snapshot.MarkPriceTime = mark.EventTime
	} else if config.RequireMarkPrice || !errors.Is(err, marketdata.ErrNotFound) {
		return paperMarketSnapshot{}, fmt.Errorf("load latest mark price: %w", err)
	}

	rate, err := repo.LatestFundingRate(ctx, config.Exchange, config.Symbol)
	if err == nil {
		fundingRatePct, parseErr := parseFundingRatePct(rate.FundingRate)
		if parseErr != nil {
			return paperMarketSnapshot{}, parseErr
		}
		snapshot.LatestFundingRatePct = fundingRatePct
		snapshot.FundingRateTime = rate.FundingTime
		snapshot.FundingRateSource = "latest_funding_rate"
	} else if !errors.Is(err, marketdata.ErrNotFound) {
		return paperMarketSnapshot{}, fmt.Errorf("load latest funding rate: %w", err)
	}

	if err := validatePaperMarketFreshness(snapshot, time.Now().UTC(), config); err != nil {
		return paperMarketSnapshot{}, err
	}
	return snapshot, nil
}

func validatePaperMarketFreshness(snapshot paperMarketSnapshot, now time.Time, config paperRunConfig) error {
	candleAgeLimit := config.MaxCandleAge
	markAgeLimit := config.MaxMarkPriceAge
	if config.MaxMarketDataAge > 0 {
		candleAgeLimit = config.MaxMarketDataAge
		markAgeLimit = config.MaxMarketDataAge
	}
	if candleAgeLimit > 0 {
		if err := validatePaperDataAge("latest candle close", snapshot.CandleCloseTime, now, candleAgeLimit); err != nil {
			return err
		}
	}
	if snapshot.PriceSource == "latest_mark_price" && markAgeLimit > 0 {
		return validatePaperDataAge(snapshot.PriceSource, snapshot.PriceTime, now, markAgeLimit)
	}
	return nil
}

func validatePaperDataAge(label string, eventTime time.Time, now time.Time, maxAge time.Duration) error {
	if eventTime.IsZero() {
		return fmt.Errorf("%s time is empty", label)
	}
	age := now.Sub(eventTime)
	if age < 0 {
		age = -age
	}
	if age > maxAge {
		return fmt.Errorf("%s is stale: age=%s max=%s", label, age.Truncate(time.Second), maxAge)
	}
	return nil
}

func latestCandlePrice(candles []marketdata.Candle) (float64, time.Time, error) {
	if len(candles) == 0 {
		return 0, time.Time{}, backtest.ErrNotEnoughData
	}
	last := candles[len(candles)-1]
	price, err := parsePositiveFloat(last.Close, "latest close price")
	if err != nil {
		return 0, time.Time{}, err
	}
	eventTime := last.CloseTime
	if eventTime.IsZero() {
		eventTime = last.OpenTime
	}
	return price, eventTime, nil
}

func parsePositiveFloat(value string, name string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid %s %s", name, value)
	}
	return parsed, nil
}

func parseFundingRatePct(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("parse funding rate: %w", err)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid funding rate %s", value)
	}
	if math.Abs(parsed) <= 1 {
		return parsed * 100, nil
	}
	return parsed, nil
}

func paperLookbackDuration(interval string, candles int) time.Duration {
	if candles <= 0 {
		candles = 120
	}
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return 2 * time.Hour
	}
	return step * time.Duration(candles)
}
