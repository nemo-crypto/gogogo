package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"gogogo/internal/marketdata"
)

const (
	DefaultSpotBaseURL    = "https://api.binance.com"
	DefaultFuturesBaseURL = "https://fapi.binance.com"
)

type Client struct {
	httpClient     *http.Client
	spotBaseURL    string
	futuresBaseURL string
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithBaseURLs(spotBaseURL string, futuresBaseURL string) Option {
	return func(c *Client) {
		if spotBaseURL != "" {
			c.spotBaseURL = spotBaseURL
		}
		if futuresBaseURL != "" {
			c.futuresBaseURL = futuresBaseURL
		}
	}
}

func NewClient(options ...Option) *Client {
	client := &Client{
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		spotBaseURL:    DefaultSpotBaseURL,
		futuresBaseURL: DefaultFuturesBaseURL,
	}

	for _, option := range options {
		option(client)
	}
	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	return client
}

type KlineRequest struct {
	MarketType marketdata.MarketType
	Symbol     string
	Interval   string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int
}

type FundingRateRequest struct {
	Symbol    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

func (c *Client) Klines(ctx context.Context, request KlineRequest) ([]marketdata.Candle, error) {
	request, err := normalizeKlineRequest(request)
	if err != nil {
		return nil, err
	}

	endpoint, err := c.klineEndpoint(request.MarketType)
	if err != nil {
		return nil, err
	}

	query := endpoint.Query()
	query.Set("symbol", request.Symbol)
	query.Set("interval", request.Interval)
	query.Set("limit", strconv.Itoa(request.Limit))
	if !request.StartTime.IsZero() {
		query.Set("startTime", strconv.FormatInt(request.StartTime.UnixMilli(), 10))
	}
	if !request.EndTime.IsZero() {
		query.Set("endTime", strconv.FormatInt(request.EndTime.UnixMilli(), 10))
	}
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance klines status %d", response.StatusCode)
	}

	var raw []rawKline
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, err
	}

	candles := make([]marketdata.Candle, 0, len(raw))
	for _, item := range raw {
		candle, err := item.toCandle(request.MarketType, request.Symbol, request.Interval)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}

	return candles, nil
}

func (c *Client) FundingRates(ctx context.Context, request FundingRateRequest) ([]marketdata.FundingRate, error) {
	request, err := normalizeFundingRateRequest(request)
	if err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(c.futuresBaseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = "/fapi/v1/fundingRate"

	query := endpoint.Query()
	query.Set("symbol", request.Symbol)
	query.Set("limit", strconv.Itoa(request.Limit))
	if !request.StartTime.IsZero() {
		query.Set("startTime", strconv.FormatInt(request.StartTime.UnixMilli(), 10))
	}
	if !request.EndTime.IsZero() {
		query.Set("endTime", strconv.FormatInt(request.EndTime.UnixMilli(), 10))
	}
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance funding rate status %d", response.StatusCode)
	}

	var raw []rawFundingRate
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, err
	}

	rates := make([]marketdata.FundingRate, 0, len(raw))
	for _, item := range raw {
		rates = append(rates, item.toFundingRate(request.Symbol))
	}
	return rates, nil
}

func (c *Client) LatestMarkPrice(ctx context.Context, symbol string) (marketdata.MarkPrice, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return marketdata.MarkPrice{}, errors.New("symbol is required")
	}

	endpoint, err := url.Parse(c.futuresBaseURL)
	if err != nil {
		return marketdata.MarkPrice{}, err
	}
	endpoint.Path = "/fapi/v1/premiumIndex"
	query := endpoint.Query()
	query.Set("symbol", symbol)
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return marketdata.MarkPrice{}, err
	}

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return marketdata.MarkPrice{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return marketdata.MarkPrice{}, fmt.Errorf("binance mark price status %d", response.StatusCode)
	}

	var raw rawMarkPrice
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return marketdata.MarkPrice{}, err
	}

	return raw.toMarkPrice(), nil
}

func (c *Client) klineEndpoint(marketType marketdata.MarketType) (*url.URL, error) {
	baseURL := c.spotBaseURL
	path := "/api/v3/klines"
	if marketType == marketdata.MarketTypePerpetual {
		baseURL = c.futuresBaseURL
		path = "/fapi/v1/klines"
	}

	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path
	return endpoint, nil
}

type rawKline []json.RawMessage

type rawFundingRate struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
	MarkPrice   string `json:"markPrice"`
}

type rawMarkPrice struct {
	Symbol               string `json:"symbol"`
	MarkPrice            string `json:"markPrice"`
	IndexPrice           string `json:"indexPrice"`
	EstimatedSettlePrice string `json:"estimatedSettlePrice"`
	LastFundingRate      string `json:"lastFundingRate"`
	NextFundingTime      int64  `json:"nextFundingTime"`
	Time                 int64  `json:"time"`
}

func (k rawKline) toCandle(marketType marketdata.MarketType, symbol string, interval string) (marketdata.Candle, error) {
	if len(k) < 11 {
		return marketdata.Candle{}, fmt.Errorf("binance kline length = %d, want at least 11", len(k))
	}

	openTime, err := decodeInt64(k[0])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode open time: %w", err)
	}
	closeTime, err := decodeInt64(k[6])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode close time: %w", err)
	}
	tradeCount, err := decodeInt64(k[8])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode trade count: %w", err)
	}

	open, err := decodeString(k[1])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode open: %w", err)
	}
	high, err := decodeString(k[2])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode high: %w", err)
	}
	low, err := decodeString(k[3])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode low: %w", err)
	}
	closePrice, err := decodeString(k[4])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode close: %w", err)
	}
	volume, err := decodeString(k[5])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode volume: %w", err)
	}
	quoteVolume, err := decodeString(k[7])
	if err != nil {
		return marketdata.Candle{}, fmt.Errorf("decode quote volume: %w", err)
	}

	closeAt := time.UnixMilli(closeTime).UTC()
	if closeAt.After(time.UnixMilli(openTime).UTC()) {
		closeAt = closeAt.Add(time.Millisecond)
	}

	return marketdata.Candle{
		Exchange:    "binance",
		MarketType:  marketType,
		Symbol:      symbol,
		Interval:    interval,
		OpenTime:    time.UnixMilli(openTime).UTC(),
		CloseTime:   closeAt,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       closePrice,
		Volume:      volume,
		QuoteVolume: quoteVolume,
		TradeCount:  tradeCount,
		Source:      "binance",
	}, nil
}

func normalizeKlineRequest(request KlineRequest) (KlineRequest, error) {
	request.Symbol = normalizeSymbol(request.Symbol)
	request.Interval = normalizeInterval(request.Interval)
	request.StartTime = request.StartTime.UTC()
	request.EndTime = request.EndTime.UTC()

	if request.MarketType != marketdata.MarketTypeSpot && request.MarketType != marketdata.MarketTypePerpetual {
		return KlineRequest{}, fmt.Errorf("unsupported market type %q", request.MarketType)
	}
	if request.Symbol == "" {
		return KlineRequest{}, errors.New("symbol is required")
	}
	if request.Interval == "" {
		return KlineRequest{}, errors.New("interval is required")
	}
	if request.Limit <= 0 {
		request.Limit = 1000
	}
	if request.Limit > 1000 {
		request.Limit = 1000
	}
	if !request.StartTime.IsZero() && !request.EndTime.IsZero() && !request.EndTime.After(request.StartTime) {
		return KlineRequest{}, errors.New("end time must be after start time")
	}

	return request, nil
}

func normalizeFundingRateRequest(request FundingRateRequest) (FundingRateRequest, error) {
	request.Symbol = normalizeSymbol(request.Symbol)
	request.StartTime = request.StartTime.UTC()
	request.EndTime = request.EndTime.UTC()

	if request.Symbol == "" {
		return FundingRateRequest{}, errors.New("symbol is required")
	}
	if request.Limit <= 0 {
		request.Limit = 1000
	}
	if request.Limit > 1000 {
		request.Limit = 1000
	}
	if !request.StartTime.IsZero() && !request.EndTime.IsZero() && !request.EndTime.After(request.StartTime) {
		return FundingRateRequest{}, errors.New("end time must be after start time")
	}

	return request, nil
}

func (r rawFundingRate) toFundingRate(symbol string) marketdata.FundingRate {
	if r.Symbol != "" {
		symbol = normalizeSymbol(r.Symbol)
	}
	return marketdata.FundingRate{
		Exchange:    "binance",
		Symbol:      symbol,
		FundingTime: time.UnixMilli(r.FundingTime).UTC(),
		FundingRate: r.FundingRate,
		MarkPrice:   r.MarkPrice,
	}
}

func (r rawMarkPrice) toMarkPrice() marketdata.MarkPrice {
	return marketdata.MarkPrice{
		Exchange:             "binance",
		Symbol:               normalizeSymbol(r.Symbol),
		EventTime:            time.UnixMilli(r.Time).UTC(),
		MarkPrice:            r.MarkPrice,
		IndexPrice:           r.IndexPrice,
		EstimatedSettlePrice: r.EstimatedSettlePrice,
		NextFundingTime:      time.UnixMilli(r.NextFundingTime).UTC(),
	}
}

func decodeString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func decodeInt64(raw json.RawMessage) (int64, error) {
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		return 0, err
	}
	return strconv.ParseInt(asString, 10, 64)
}

func normalizeSymbol(symbol string) string {
	normalized := ""
	for _, current := range symbol {
		if current == '/' || current == '-' || current == '_' || current == ' ' {
			continue
		}
		if current >= 'a' && current <= 'z' {
			current -= 'a' - 'A'
		}
		normalized += string(current)
	}
	return normalized
}

func normalizeInterval(interval string) string {
	return interval
}
