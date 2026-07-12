package marketdata

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = errors.New("market data not found")

type MarketType string

const (
	MarketTypeSpot      MarketType = "spot"
	MarketTypePerpetual MarketType = "perpetual"
)

type Candle struct {
	Exchange    string
	MarketType  MarketType
	Symbol      string
	Interval    string
	OpenTime    time.Time
	CloseTime   time.Time
	Open        string
	High        string
	Low         string
	Close       string
	Volume      string
	QuoteVolume string
	TradeCount  int64
	Source      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type FundingRate struct {
	Exchange    string
	Symbol      string
	FundingTime time.Time
	FundingRate string
	MarkPrice   string
	IndexPrice  string
	CreatedAt   time.Time
}

type MarkPrice struct {
	Exchange             string
	Symbol               string
	EventTime            time.Time
	MarkPrice            string
	IndexPrice           string
	EstimatedSettlePrice string
	NextFundingTime      time.Time
	CreatedAt            time.Time
}

type CandleQuery struct {
	Exchange   string
	MarketType MarketType
	Symbol     string
	Interval   string
	Start      time.Time
	End        time.Time
	Limit      int
}

type CandleGap struct {
	Start        time.Time
	End          time.Time
	MissingCount int
}

type CandleCoverage struct {
	Exchange         string
	MarketType       MarketType
	Symbol           string
	Interval         string
	Start            time.Time
	End              time.Time
	IntervalDuration time.Duration
	CandleCount      int
	ExpectedCount    int
	MissingCount     int
	Gaps             []CandleGap
}

func (c CandleCoverage) Complete() bool {
	return c.MissingCount == 0
}

type CandleSnapshot struct {
	ID            int64
	Name          string
	Exchange      string
	MarketType    MarketType
	Symbol        string
	Interval      string
	Start         time.Time
	End           time.Time
	CandleCount   int
	ExpectedCount int
	MissingCount  int
	GapCount      int
	DataHash      string
	CreatedAt     time.Time
}

type CandleSnapshotRequest struct {
	Name            string
	Query           CandleQuery
	RequireComplete bool
}

type FundingRateQuery struct {
	Exchange string
	Symbol   string
	Start    time.Time
	End      time.Time
	Limit    int
}

func normalizeCandle(c Candle) (Candle, error) {
	c.Exchange = normalizeExchange(c.Exchange)
	c.MarketType = normalizeMarketType(c.MarketType)
	c.Symbol = normalizeSymbol(c.Symbol)
	c.Interval = strings.TrimSpace(c.Interval)
	c.Source = strings.TrimSpace(c.Source)
	c.OpenTime = c.OpenTime.UTC()
	c.CloseTime = c.CloseTime.UTC()
	c.CreatedAt = c.CreatedAt.UTC()
	c.UpdatedAt = c.UpdatedAt.UTC()

	if c.Source == "" {
		c.Source = c.Exchange
	}
	if err := validateMarket(c.Exchange, c.MarketType, c.Symbol); err != nil {
		return Candle{}, err
	}
	if c.Interval == "" {
		return Candle{}, errors.New("interval is required")
	}
	if c.OpenTime.IsZero() {
		return Candle{}, errors.New("open time is required")
	}
	if !c.CloseTime.After(c.OpenTime) {
		return Candle{}, errors.New("close time must be after open time")
	}
	if err := requirePositiveDecimal("open", c.Open); err != nil {
		return Candle{}, err
	}
	if err := requirePositiveDecimal("high", c.High); err != nil {
		return Candle{}, err
	}
	if err := requirePositiveDecimal("low", c.Low); err != nil {
		return Candle{}, err
	}
	if err := requirePositiveDecimal("close", c.Close); err != nil {
		return Candle{}, err
	}
	if err := requireNonNegativeDecimal("volume", c.Volume); err != nil {
		return Candle{}, err
	}
	if err := requireNonNegativeDecimal("quote volume", c.QuoteVolume); err != nil {
		return Candle{}, err
	}
	if c.TradeCount < 0 {
		return Candle{}, errors.New("trade count cannot be negative")
	}

	return c, nil
}

func normalizeFundingRate(rate FundingRate) (FundingRate, error) {
	rate.Exchange = normalizeExchange(rate.Exchange)
	rate.Symbol = normalizeSymbol(rate.Symbol)
	rate.FundingTime = rate.FundingTime.UTC()
	rate.CreatedAt = rate.CreatedAt.UTC()

	if rate.Exchange == "" {
		return FundingRate{}, errors.New("exchange is required")
	}
	if rate.Symbol == "" {
		return FundingRate{}, errors.New("symbol is required")
	}
	if rate.FundingTime.IsZero() {
		return FundingRate{}, errors.New("funding time is required")
	}
	if err := requireDecimal("funding rate", rate.FundingRate); err != nil {
		return FundingRate{}, err
	}
	if err := requirePositiveDecimal("mark price", rate.MarkPrice); err != nil {
		return FundingRate{}, err
	}
	rate.IndexPrice = strings.TrimSpace(rate.IndexPrice)
	if rate.IndexPrice != "" {
		if err := requirePositiveDecimal("index price", rate.IndexPrice); err != nil {
			return FundingRate{}, err
		}
	}

	return rate, nil
}

func normalizeMarkPrice(price MarkPrice) (MarkPrice, error) {
	price.Exchange = normalizeExchange(price.Exchange)
	price.Symbol = normalizeSymbol(price.Symbol)
	price.EstimatedSettlePrice = strings.TrimSpace(price.EstimatedSettlePrice)
	price.EventTime = price.EventTime.UTC()
	price.NextFundingTime = price.NextFundingTime.UTC()
	price.CreatedAt = price.CreatedAt.UTC()

	if price.Exchange == "" {
		return MarkPrice{}, errors.New("exchange is required")
	}
	if price.Symbol == "" {
		return MarkPrice{}, errors.New("symbol is required")
	}
	if price.EventTime.IsZero() {
		return MarkPrice{}, errors.New("event time is required")
	}
	if err := requirePositiveDecimal("mark price", price.MarkPrice); err != nil {
		return MarkPrice{}, err
	}
	if err := requirePositiveDecimal("index price", price.IndexPrice); err != nil {
		return MarkPrice{}, err
	}
	if price.EstimatedSettlePrice != "" {
		if err := requireNonNegativeDecimal("estimated settle price", price.EstimatedSettlePrice); err != nil {
			return MarkPrice{}, err
		}
	}

	return price, nil
}

func normalizeCandleQuery(query CandleQuery) (CandleQuery, error) {
	query.Exchange = normalizeExchange(query.Exchange)
	query.MarketType = normalizeMarketType(query.MarketType)
	query.Symbol = normalizeSymbol(query.Symbol)
	query.Interval = strings.TrimSpace(query.Interval)
	query.Start = query.Start.UTC()
	query.End = query.End.UTC()
	query.Limit = normalizeLimit(query.Limit)

	if err := validateMarket(query.Exchange, query.MarketType, query.Symbol); err != nil {
		return CandleQuery{}, err
	}
	if query.Interval == "" {
		return CandleQuery{}, errors.New("interval is required")
	}
	if query.Start.IsZero() || query.End.IsZero() {
		return CandleQuery{}, errors.New("start and end are required")
	}
	if !query.End.After(query.Start) {
		return CandleQuery{}, errors.New("end must be after start")
	}

	return query, nil
}

func normalizeFundingRateQuery(query FundingRateQuery) (FundingRateQuery, error) {
	query.Exchange = normalizeExchange(query.Exchange)
	query.Symbol = normalizeSymbol(query.Symbol)
	query.Start = query.Start.UTC()
	query.End = query.End.UTC()
	query.Limit = normalizeLimit(query.Limit)

	if query.Exchange == "" {
		return FundingRateQuery{}, errors.New("exchange is required")
	}
	if query.Symbol == "" {
		return FundingRateQuery{}, errors.New("symbol is required")
	}
	if query.Start.IsZero() || query.End.IsZero() {
		return FundingRateQuery{}, errors.New("start and end are required")
	}
	if !query.End.After(query.Start) {
		return FundingRateQuery{}, errors.New("end must be after start")
	}

	return query, nil
}

func validateMarket(exchange string, marketType MarketType, symbol string) error {
	if exchange == "" {
		return errors.New("exchange is required")
	}
	if symbol == "" {
		return errors.New("symbol is required")
	}
	if marketType != MarketTypeSpot && marketType != MarketTypePerpetual {
		return fmt.Errorf("unsupported market type %q", marketType)
	}
	return nil
}

func normalizeExchange(exchange string) string {
	return strings.ToLower(strings.TrimSpace(exchange))
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func normalizeMarketType(marketType MarketType) MarketType {
	return MarketType(strings.ToLower(strings.TrimSpace(string(marketType))))
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 1000
	}
	if limit > 10000 {
		return 10000
	}
	return limit
}

func requirePositiveDecimal(name string, value string) error {
	parsed, err := parseDecimal(name, value)
	if err != nil {
		return err
	}
	if parsed <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func requireNonNegativeDecimal(name string, value string) error {
	parsed, err := parseDecimal(name, value)
	if err != nil {
		return err
	}
	if parsed < 0 {
		return fmt.Errorf("%s cannot be negative", name)
	}
	return nil
}

func requireDecimal(name string, value string) error {
	_, err := parseDecimal(name, value)
	return err
}

func parseDecimal(name string, value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be decimal: %w", name, err)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s must be finite", name)
	}
	return parsed, nil
}
