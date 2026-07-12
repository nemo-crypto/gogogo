package marketdata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func IntervalDuration(interval string) (time.Duration, error) {
	interval = strings.TrimSpace(interval)
	if len(interval) < 2 {
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}

	value, err := strconv.Atoi(interval[:len(interval)-1])
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}

	switch interval[len(interval)-1] {
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	case 'M':
		return 0, errorsUnsupportedMonthlyInterval(interval)
	default:
		return 0, fmt.Errorf("unsupported interval %q", interval)
	}
}

func CheckCandleCoverage(candles []Candle, query CandleQuery) (CandleCoverage, error) {
	query, err := normalizeCandleQuery(query)
	if err != nil {
		return CandleCoverage{}, err
	}
	step, err := IntervalDuration(query.Interval)
	if err != nil {
		return CandleCoverage{}, err
	}

	opens := make(map[time.Time]struct{}, len(candles))
	for _, candle := range candles {
		openTime := candle.OpenTime.UTC()
		if openTime.Before(query.Start) || !openTime.Before(query.End) {
			continue
		}
		opens[openTime] = struct{}{}
	}

	coverage := CandleCoverage{
		Exchange:         query.Exchange,
		MarketType:       query.MarketType,
		Symbol:           query.Symbol,
		Interval:         query.Interval,
		Start:            query.Start,
		End:              query.End,
		IntervalDuration: step,
		CandleCount:      len(opens),
		Gaps:             make([]CandleGap, 0),
	}

	var gapStart time.Time
	gapMissing := 0
	for current := query.Start; current.Before(query.End); current = current.Add(step) {
		coverage.ExpectedCount++
		if _, ok := opens[current]; ok {
			if gapMissing > 0 {
				coverage.Gaps = append(coverage.Gaps, CandleGap{
					Start:        gapStart,
					End:          gapStart.Add(step * time.Duration(gapMissing)),
					MissingCount: gapMissing,
				})
				gapMissing = 0
			}
			continue
		}

		if gapMissing == 0 {
			gapStart = current
		}
		gapMissing++
		coverage.MissingCount++
	}
	if gapMissing > 0 {
		coverage.Gaps = append(coverage.Gaps, CandleGap{
			Start:        gapStart,
			End:          gapStart.Add(step * time.Duration(gapMissing)),
			MissingCount: gapMissing,
		})
	}

	return coverage, nil
}

func CandleDataHash(candles []Candle) string {
	ordered := append([]Candle(nil), candles...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OpenTime.Equal(ordered[j].OpenTime) {
			return ordered[i].Symbol < ordered[j].Symbol
		}
		return ordered[i].OpenTime.Before(ordered[j].OpenTime)
	})

	hasher := sha256.New()
	for _, candle := range ordered {
		fmt.Fprintf(hasher, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%s\n",
			normalizeExchange(candle.Exchange),
			normalizeMarketType(candle.MarketType),
			normalizeSymbol(candle.Symbol),
			strings.TrimSpace(candle.Interval),
			candle.OpenTime.UTC().Format(time.RFC3339Nano),
			candle.CloseTime.UTC().Format(time.RFC3339Nano),
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
			candle.Volume,
			candle.QuoteVolume,
			candle.TradeCount,
			strings.TrimSpace(candle.Source),
		)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func errorsUnsupportedMonthlyInterval(interval string) error {
	return fmt.Errorf("unsupported interval %q: monthly candles do not have a fixed duration", interval)
}
