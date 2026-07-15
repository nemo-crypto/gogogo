package onebullex

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	exchangemodel "gogogo/internal/exchange"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
)

const (
	ExchangeName    = "onebullex"
	DefaultBaseURL  = "https://futures-openapi.onebullex.com"
	TestnetBaseURL  = "https://futures-openapi.1bullex.com"
	defaultPageSize = 500
	maxPageSize     = 1500
)

var ErrLiveTradingDisabled = errors.New("onebullex live trading is disabled")

type Client struct {
	httpClient       *http.Client
	baseURL          string
	apiKey           string
	secretKey        string
	tradingEnabled   bool
	nonceFunc        func() (string, error)
	nowFunc          func() time.Time
	retryMaxAttempts int
	retryBaseDelay   time.Duration
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

func WithCredentials(apiKey string, secretKey string) Option {
	return func(c *Client) {
		c.apiKey = strings.TrimSpace(apiKey)
		c.secretKey = strings.TrimSpace(secretKey)
	}
}

func WithTradingEnabled(enabled bool) Option {
	return func(c *Client) {
		c.tradingEnabled = enabled
	}
}

func WithNonceFunc(nonceFunc func() (string, error)) Option {
	return func(c *Client) {
		if nonceFunc != nil {
			c.nonceFunc = nonceFunc
		}
	}
}

func WithNowFunc(nowFunc func() time.Time) Option {
	return func(c *Client) {
		if nowFunc != nil {
			c.nowFunc = nowFunc
		}
	}
}

func WithRetryPolicy(maxAttempts int, baseDelay time.Duration) Option {
	return func(c *Client) {
		if maxAttempts > 0 {
			c.retryMaxAttempts = maxAttempts
		}
		if baseDelay >= 0 {
			c.retryBaseDelay = baseDelay
		}
	}
}

func NewClient(options ...Option) *Client {
	client := &Client{
		httpClient:       &http.Client{Timeout: 15 * time.Second},
		baseURL:          DefaultBaseURL,
		nonceFunc:        randomNonce,
		nowFunc:          time.Now,
		retryMaxAttempts: 3,
		retryBaseDelay:   250 * time.Millisecond,
	}
	for _, option := range options {
		option(client)
	}
	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if client.nonceFunc == nil {
		client.nonceFunc = randomNonce
	}
	if client.nowFunc == nil {
		client.nowFunc = time.Now
	}
	if client.retryMaxAttempts <= 0 {
		client.retryMaxAttempts = 1
	}
	if client.retryBaseDelay < 0 {
		client.retryBaseDelay = 0
	}
	return client
}

func (c *Client) Klines(ctx context.Context, request exchangemodel.KlineRequest) ([]marketdata.Candle, error) {
	request, err := normalizeKlineRequest(request)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(request.Symbol))
	query.Set("interval", request.Interval)
	query.Set("limit", strconv.Itoa(request.Limit))
	if !request.StartTime.IsZero() {
		query.Set("startTime", strconv.FormatInt(request.StartTime.UnixMilli(), 10))
	}
	if !request.EndTime.IsZero() {
		query.Set("endTime", strconv.FormatInt(request.EndTime.UnixMilli(), 10))
	}

	var raw []rawKline
	if err := c.publicGET(ctx, "/v2/public/q/kline", query, &raw); err != nil {
		return nil, err
	}

	candles := make([]marketdata.Candle, 0, len(raw))
	for _, item := range raw {
		candle, err := item.toCandle(request.Symbol, request.Interval)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})
	return candles, nil
}

func (c *Client) FundingRates(ctx context.Context, request exchangemodel.FundingRateRequest) ([]marketdata.FundingRate, error) {
	request, err := normalizeFundingRateRequest(request)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(request.Symbol))
	query.Set("limit", strconv.Itoa(request.Limit))

	var page rawCursorPage[rawFundingRateRecord]
	if err := c.publicGET(ctx, "/v2/public/q/funding-rate-record", query, &page); err != nil {
		return nil, err
	}

	markPrice := ""
	if price, err := c.LatestMarkPrice(ctx, request.Symbol); err == nil {
		markPrice = price.MarkPrice
	}

	rates := make([]marketdata.FundingRate, 0, len(page.Items))
	for _, item := range page.Items {
		rate := item.toFundingRate(request.Symbol, markPrice)
		if !request.StartTime.IsZero() && rate.FundingTime.Before(request.StartTime) {
			continue
		}
		if !request.EndTime.IsZero() && !rate.FundingTime.Before(request.EndTime) {
			continue
		}
		rates = append(rates, rate)
	}
	if len(rates) > 0 {
		return rates, nil
	}

	var current rawFundingRate
	if err := c.publicGET(ctx, "/v2/public/q/funding-rate", query, &current); err != nil {
		return nil, err
	}
	return []marketdata.FundingRate{current.toFundingRate(request.Symbol, markPrice)}, nil
}

func (c *Client) LatestMarkPrice(ctx context.Context, symbol string) (marketdata.MarkPrice, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return marketdata.MarkPrice{}, errors.New("symbol is required")
	}

	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(symbol))

	var rawMark rawPrice
	if err := c.publicGET(ctx, "/v2/public/q/symbol-mark-price", query, &rawMark); err != nil {
		return marketdata.MarkPrice{}, err
	}

	indexPrice := rawMark.Price
	var rawIndexes rawPriceList
	if err := c.publicGET(ctx, "/v2/public/q/index-price", query, &rawIndexes); err == nil && len(rawIndexes) > 0 && rawIndexes[0].Price != "" {
		indexPrice = rawIndexes[0].Price
	}

	return marketdata.MarkPrice{
		Exchange:   ExchangeName,
		Symbol:     FromExchangeSymbol(firstNonEmpty(rawMark.Symbol, symbol)),
		EventTime:  time.UnixMilli(rawMark.Timestamp).UTC(),
		MarkPrice:  rawMark.Price,
		IndexPrice: indexPrice,
		RawJSON:    rawJSON(rawMark),
	}, nil
}

func (c *Client) IndexPrices(ctx context.Context, symbol string) ([]marketdata.IndexPrice, error) {
	query := url.Values{}
	if strings.TrimSpace(symbol) != "" {
		query.Set("symbol", ToExchangeSymbol(symbol))
	}
	var raw rawPriceList
	if err := c.publicGET(ctx, "/v2/public/q/index-price", query, &raw); err != nil {
		return nil, err
	}
	prices := make([]marketdata.IndexPrice, 0, len(raw))
	for _, item := range raw {
		if item.Timestamp <= 0 || item.Price == "" {
			continue
		}
		prices = append(prices, marketdata.IndexPrice{
			Exchange:   ExchangeName,
			Symbol:     FromExchangeSymbol(firstNonEmpty(item.Symbol, symbol)),
			EventTime:  time.UnixMilli(item.Timestamp).UTC(),
			IndexPrice: item.Price,
			RawJSON:    rawJSON(item),
		})
	}
	return prices, nil
}

func (c *Client) RecentTrades(ctx context.Context, symbol string, limit int) ([]marketdata.Trade, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, errors.New("symbol is required")
	}
	if limit <= 0 {
		limit = 50
	}
	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(symbol))
	query.Set("num", strconv.Itoa(limit))
	var raw []rawDeal
	if err := c.publicGET(ctx, "/v2/public/q/deal", query, &raw); err != nil {
		return nil, err
	}
	trades := make([]marketdata.Trade, 0, len(raw))
	for _, item := range raw {
		trade := item.toTrade(symbol)
		if trade.TradeID == "" {
			continue
		}
		trades = append(trades, trade)
	}
	return trades, nil
}

func (c *Client) OrderBook(ctx context.Context, symbol string, level int) (marketdata.OrderBook, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return marketdata.OrderBook{}, errors.New("symbol is required")
	}
	if level <= 0 {
		level = 20
	}
	if level > 50 {
		level = 50
	}
	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(symbol))
	query.Set("level", strconv.Itoa(level))
	var raw rawDepth
	if err := c.publicGET(ctx, "/v2/public/q/depth", query, &raw); err != nil {
		return marketdata.OrderBook{}, err
	}
	return raw.toOrderBook(symbol)
}

func (c *Client) SymbolSpecs(ctx context.Context) ([]portfolio.ContractSpec, error) {
	var raw []rawSymbolSpec
	if err := c.publicGET(ctx, "/v2/public/symbol/list", nil, &raw); err != nil {
		return nil, err
	}
	specs := make([]portfolio.ContractSpec, 0, len(raw))
	for _, item := range raw {
		spec := item.toContractSpec()
		if spec.Symbol == "" {
			continue
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func (c *Client) LeverageBrackets(ctx context.Context, symbol string) ([]portfolio.LeverageBracket, error) {
	query := url.Values{}
	if strings.TrimSpace(symbol) != "" {
		query.Set("symbol", ToExchangeSymbol(symbol))
		var raw rawSymbolBracket
		if err := c.publicGET(ctx, "/v2/public/leverage/bracket/detail", query, &raw); err != nil {
			return nil, err
		}
		return raw.toLeverageBrackets(), nil
	}
	var raw []rawSymbolBracket
	if err := c.publicGET(ctx, "/v2/public/leverage/bracket/list", nil, &raw); err != nil {
		return nil, err
	}
	brackets := make([]portfolio.LeverageBracket, 0)
	for _, item := range raw {
		brackets = append(brackets, item.toLeverageBrackets()...)
	}
	return brackets, nil
}

type PositionModeRequest struct {
	Symbol        string
	PositionType  string
	PositionModel string
}

func (c *Client) PositionConfigs(ctx context.Context, accountID string, symbol string) ([]portfolio.PositionConfig, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, errors.New("symbol is required")
	}
	query := url.Values{}
	query.Set("symbol", ToExchangeSymbol(symbol))
	var raw []rawPositionConfig
	if err := c.signedGET(ctx, "/v2/position/confs", query, &raw); err != nil {
		return nil, err
	}
	snapshotTime := c.nowFunc().UTC()
	configs := make([]portfolio.PositionConfig, 0, len(raw))
	for _, item := range raw {
		configs = append(configs, item.toPositionConfig(accountID, snapshotTime))
	}
	return configs, nil
}

func (c *Client) ChangePositionMode(ctx context.Context, request PositionModeRequest) error {
	symbol := normalizeSymbol(request.Symbol)
	positionType := strings.ToUpper(strings.TrimSpace(request.PositionType))
	positionModel := normalizePositionModel(request.PositionModel)
	if symbol == "" || positionType == "" || positionModel == "" {
		return errors.New("symbol, position type and position model are required")
	}
	body := map[string]any{
		"symbol":        ToExchangeSymbol(symbol),
		"positionType":  positionType,
		"positionModel": positionModel,
	}
	return c.signedPOST(ctx, "/v2/position/change-type", body, nil)
}

func (c *Client) ServerTime(ctx context.Context, _ marketdata.MarketType) (time.Time, error) {
	var timestamp int64
	if err := c.publicGET(ctx, "/v2/public/time", nil, &timestamp); err != nil {
		return time.Time{}, err
	}
	if timestamp > 0 && timestamp < 1_000_000_000_000 {
		return time.Unix(timestamp, 0).UTC(), nil
	}
	return time.UnixMilli(timestamp).UTC(), nil
}

func (c *Client) AccountSnapshot(ctx context.Context, accountID string) (exchangemodel.AccountSnapshot, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return exchangemodel.AccountSnapshot{}, errors.New("account id is required")
	}

	var rawBalances []rawBalance
	if err := c.signedGET(ctx, "/v2/balance/list", nil, &rawBalances); err != nil {
		return exchangemodel.AccountSnapshot{}, err
	}

	var rawPositions []rawPosition
	if err := c.signedGET(ctx, "/v2/position/list", nil, &rawPositions); err != nil {
		return exchangemodel.AccountSnapshot{}, err
	}

	snapshotTime := c.nowFunc().UTC()
	serverTime, err := c.ServerTime(ctx, marketdata.MarketTypePerpetual)
	if err != nil {
		serverTime = snapshotTime
	}

	balances := make([]exchangemodel.Balance, 0, len(rawBalances))
	for _, item := range rawBalances {
		balances = append(balances, item.toBalance(snapshotTime))
	}

	markPrices := make(map[string]string)
	positions := make([]exchangemodel.Position, 0, len(rawPositions))
	for _, item := range rawPositions {
		symbol := FromExchangeSymbol(item.Symbol)
		markPrice, ok := markPrices[symbol]
		if !ok {
			if price, err := c.LatestMarkPrice(ctx, symbol); err == nil {
				markPrice = price.MarkPrice
			}
			markPrices[symbol] = markPrice
		}
		positions = append(positions, item.toPosition(snapshotTime, markPrice))
	}

	return exchangemodel.AccountSnapshot{
		AccountID:    accountID,
		Exchange:     ExchangeName,
		Balances:     balances,
		Positions:    positions,
		ServerTime:   serverTime,
		SnapshotTime: snapshotTime,
		ReadOnly:     !c.tradingEnabled,
	}, nil
}

func (c *Client) SubmitOrder(ctx context.Context, request exchangemodel.OrderRequest) (exchangemodel.OrderStatus, error) {
	if !c.tradingEnabled {
		return exchangemodel.OrderStatus{}, ErrLiveTradingDisabled
	}
	body, err := c.orderBody(request)
	if err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	var orderID string
	if err := c.signedPOST(ctx, "/v2/order/create", body, &orderID); err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	status, err := c.orderStatusByID(ctx, orderID)
	if err != nil {
		return exchangemodel.OrderStatus{
			ClientOrderID:   request.ClientOrderID,
			ExchangeOrderID: orderID,
			Status:          "SUBMITTED",
			UpdatedAt:       c.nowFunc().UTC(),
		}, nil
	}
	return status, nil
}

func (c *Client) CancelOrder(ctx context.Context, accountID string, symbol string, clientOrderID string) (exchangemodel.OrderStatus, error) {
	if !c.tradingEnabled {
		return exchangemodel.OrderStatus{}, ErrLiveTradingDisabled
	}
	status, err := c.OrderStatus(ctx, accountID, symbol, clientOrderID)
	if err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	if status.ExchangeOrderID == "" {
		return exchangemodel.OrderStatus{}, errors.New("onebullex order id is required to cancel order")
	}
	body := map[string]any{"orderId": status.ExchangeOrderID}
	var raw any
	if err := c.signedPOST(ctx, "/v2/order/cancel", body, &raw); err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	return c.orderStatusByID(ctx, status.ExchangeOrderID)
}

func (c *Client) OrderStatus(ctx context.Context, _ string, symbol string, clientOrderID string) (exchangemodel.OrderStatus, error) {
	clientOrderID = strings.TrimSpace(clientOrderID)
	if clientOrderID == "" {
		return exchangemodel.OrderStatus{}, errors.New("client order id is required")
	}
	if isIntegerString(clientOrderID) {
		return c.orderStatusByID(ctx, clientOrderID)
	}

	query := url.Values{}
	if strings.TrimSpace(symbol) != "" {
		query.Set("symbol", ToExchangeSymbol(symbol))
	}
	query.Set("page", "1")
	query.Set("size", "100")

	var page rawCursorPage[rawOrder]
	if err := c.signedGET(ctx, "/v2/order/list", query, &page); err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	for _, order := range page.Items {
		if order.ClientOrderID == clientOrderID {
			return order.toOrderStatus(), nil
		}
	}
	return exchangemodel.OrderStatus{}, fmt.Errorf("onebullex order %q not found", clientOrderID)
}

func (c *Client) orderStatusByID(ctx context.Context, orderID string) (exchangemodel.OrderStatus, error) {
	query := url.Values{}
	query.Set("orderId", strings.TrimSpace(orderID))
	var raw rawOrder
	if err := c.signedGET(ctx, "/v2/order/detail", query, &raw); err != nil {
		return exchangemodel.OrderStatus{}, err
	}
	return raw.toOrderStatus(), nil
}

func (c *Client) orderBody(request exchangemodel.OrderRequest) (map[string]any, error) {
	symbol := normalizeSymbol(request.Symbol)
	side := strings.ToUpper(strings.TrimSpace(request.Side))
	orderType := strings.ToUpper(strings.TrimSpace(request.OrderType))
	quantity := strings.TrimSpace(request.Quantity)
	if symbol == "" || side == "" || orderType == "" || quantity == "" {
		return nil, errors.New("symbol, side, order type and quantity are required")
	}
	if orderType == "LIMIT" && strings.TrimSpace(request.Price) == "" {
		return nil, errors.New("price is required for limit order")
	}

	positionModel := normalizePositionModel(request.PositionModel)
	positionSide := normalizePositionSide(request.PositionSide)
	if positionSide == "" {
		if isAggregationPositionModel(positionModel) {
			positionSide = "BOTH"
		} else {
			var err error
			positionSide, err = inferPositionSide(side, request.ReduceOnly)
			if err != nil {
				return nil, err
			}
		}
	}

	body := map[string]any{
		"symbol":       ToExchangeSymbol(symbol),
		"orderType":    orderType,
		"orderSide":    side,
		"positionSide": positionSide,
		"origQty":      quantity,
	}
	if request.ClientOrderID != "" {
		body["clientOrderId"] = strings.TrimSpace(request.ClientOrderID)
	}
	if request.Price != "" {
		body["price"] = strings.TrimSpace(request.Price)
	}
	if request.TimeInForce != "" {
		body["timeInForce"] = strings.ToUpper(strings.TrimSpace(request.TimeInForce))
	}
	if request.PositionID != "" {
		body["positionId"] = strings.TrimSpace(request.PositionID)
	}
	if request.Leverage > 0 {
		body["leverage"] = request.Leverage
	}
	if request.ReduceOnly {
		body["reduceOnly"] = true
	}
	if request.TriggerProfitPrice != "" {
		body["triggerProfitPrice"] = strings.TrimSpace(request.TriggerProfitPrice)
	}
	if request.TriggerStopPrice != "" {
		body["triggerStopPrice"] = strings.TrimSpace(request.TriggerStopPrice)
	}
	if request.ProfitOrderType != "" {
		body["profitOrderType"] = strings.ToUpper(strings.TrimSpace(request.ProfitOrderType))
	}
	if request.StopOrderType != "" {
		body["stopOrderType"] = strings.ToUpper(strings.TrimSpace(request.StopOrderType))
	}
	if request.ProfitOrderPrice != "" {
		body["profitOrderPrice"] = strings.TrimSpace(request.ProfitOrderPrice)
	}
	if request.StopOrderPrice != "" {
		body["stopOrderPrice"] = strings.TrimSpace(request.StopOrderPrice)
	}
	if request.MarketOrderLevel > 0 {
		body["marketOrderLevel"] = request.MarketOrderLevel
	}
	return body, nil
}

func (c *Client) publicGET(ctx context.Context, path string, query url.Values, target any) error {
	return c.doRequest(ctx, http.MethodGet, path, query, nil, false, target)
}

func (c *Client) signedGET(ctx context.Context, path string, query url.Values, target any) error {
	return c.doRequest(ctx, http.MethodGet, path, query, nil, true, target)
}

func (c *Client) signedPOST(ctx context.Context, path string, body map[string]any, target any) error {
	return c.doRequest(ctx, http.MethodPost, path, nil, body, true, target)
}

func (c *Client) doRequest(ctx context.Context, method string, path string, query url.Values, body map[string]any, signed bool, target any) error {
	endpoint, err := c.endpoint(path, query)
	if err != nil {
		return err
	}

	var payload []byte
	if method == http.MethodPost {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	attempts := 1
	if retryableMethod(method) {
		attempts = c.retryMaxAttempts
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		retry, err := c.doRequestOnce(ctx, method, path, endpoint, query, payload, body, signed, target)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt == attempts {
			return err
		}
		if err := sleepBeforeRetry(ctx, c.retryDelay(attempt)); err != nil {
			return err
		}
	}
	return lastErr
}

func (c *Client) doRequestOnce(ctx context.Context, method string, path string, endpoint string, query url.Values, payload []byte, body map[string]any, signed bool, target any) (bool, error) {
	var bodyReader io.Reader
	if method == http.MethodPost {
		bodyReader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return false, err
	}
	request.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if signed {
		if err := c.signRequest(request, query, body); err != nil {
			return false, err
		}
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return retryableMethod(method) && ctx.Err() == nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return retryableStatus(method, response.StatusCode), fmt.Errorf("onebullex %s %s status %d", method, path, response.StatusCode)
	}

	var envelope responseEnvelope[json.RawMessage]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return retryableMethod(method), err
	}
	code := envelope.CodeValue()
	if code != 0 {
		msg := envelope.Message()
		if msg == "" {
			msg = "request failed"
		}
		return false, fmt.Errorf("onebullex %s %s code=%d msg=%s", method, path, code, msg)
	}
	if target == nil {
		return false, nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return false, nil
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return false, err
	}
	return false, nil
}

func retryableMethod(method string) bool {
	return method == http.MethodGet
}

func retryableStatus(method string, statusCode int) bool {
	return retryableMethod(method) && (statusCode == http.StatusTooManyRequests || statusCode >= 500)
}

func (c *Client) retryDelay(attempt int) time.Duration {
	if c.retryBaseDelay <= 0 {
		return 0
	}
	return c.retryBaseDelay * time.Duration(1<<(attempt-1))
}

func sleepBeforeRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) endpoint(path string, query url.Values) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	base.Path = path
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *Client) signRequest(request *http.Request, query url.Values, body map[string]any) error {
	if c.apiKey == "" || c.secretKey == "" {
		return errors.New("onebullex api key and secret key are required")
	}
	nonce, err := c.nonceFunc()
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(c.nowFunc().Unix(), 10)

	params := map[string]string{
		"nonce":     nonce,
		"timestamp": timestamp,
	}
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		params[key] = values[0]
	}
	for key, value := range body {
		valueString, ok := signatureValue(value)
		if !ok {
			continue
		}
		params[key] = valueString
	}

	request.Header.Set("X-API-KEY", c.apiKey)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", SignParams(params, c.secretKey))
	return nil
}

func SignParams(params map[string]string, secretKey string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(mac.Sum(nil))
}

func SignaturePayload(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&")
}

func signatureValue(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case json.Number:
		return typed.String(), true
	default:
		switch value.(type) {
		case map[string]any, []any:
			return "", false
		}
		return fmt.Sprint(typed), true
	}
}

func normalizeKlineRequest(request exchangemodel.KlineRequest) (exchangemodel.KlineRequest, error) {
	request.Symbol = normalizeSymbol(request.Symbol)
	request.Interval = strings.TrimSpace(request.Interval)
	request.StartTime = request.StartTime.UTC()
	request.EndTime = request.EndTime.UTC()
	if request.MarketType != marketdata.MarketTypePerpetual {
		return exchangemodel.KlineRequest{}, fmt.Errorf("onebullex supports perpetual market data only, got %q", request.MarketType)
	}
	if request.Symbol == "" {
		return exchangemodel.KlineRequest{}, errors.New("symbol is required")
	}
	if request.Interval == "" {
		return exchangemodel.KlineRequest{}, errors.New("interval is required")
	}
	if request.Limit <= 0 {
		request.Limit = defaultPageSize
	}
	if request.Limit > maxPageSize {
		request.Limit = maxPageSize
	}
	if !request.StartTime.IsZero() && !request.EndTime.IsZero() && !request.EndTime.After(request.StartTime) {
		return exchangemodel.KlineRequest{}, errors.New("end time must be after start time")
	}
	return request, nil
}

func normalizeFundingRateRequest(request exchangemodel.FundingRateRequest) (exchangemodel.FundingRateRequest, error) {
	request.Symbol = normalizeSymbol(request.Symbol)
	request.StartTime = request.StartTime.UTC()
	request.EndTime = request.EndTime.UTC()
	if request.Symbol == "" {
		return exchangemodel.FundingRateRequest{}, errors.New("symbol is required")
	}
	if request.Limit <= 0 {
		request.Limit = 100
	}
	if request.Limit > maxPageSize {
		request.Limit = maxPageSize
	}
	if !request.StartTime.IsZero() && !request.EndTime.IsZero() && !request.EndTime.After(request.StartTime) {
		return exchangemodel.FundingRateRequest{}, errors.New("end time must be after start time")
	}
	return request, nil
}

func ToExchangeSymbol(symbol string) string {
	normalized := normalizeSymbol(symbol)
	if normalized == "" {
		return ""
	}
	if strings.Contains(symbol, "_") {
		return strings.ToLower(strings.TrimSpace(symbol))
	}
	if strings.HasSuffix(normalized, "USDT") && len(normalized) > 4 {
		return strings.ToLower(normalized[:len(normalized)-4] + "_usdt")
	}
	if strings.HasSuffix(normalized, "USDC") && len(normalized) > 4 {
		return strings.ToLower(normalized[:len(normalized)-4] + "_usdc")
	}
	return strings.ToLower(normalized)
}

func FromExchangeSymbol(symbol string) string {
	return normalizeSymbol(symbol)
}

func normalizeSymbol(symbol string) string {
	normalized := ""
	for _, current := range strings.TrimSpace(symbol) {
		if current == '/' || current == '-' || current == '_' || current == ' ' {
			continue
		}
		normalized += strings.ToUpper(string(current))
	}
	return normalized
}

func inferPositionSide(orderSide string, reduceOnly bool) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(orderSide)) {
	case "BUY":
		if reduceOnly {
			return "SHORT", nil
		}
		return "LONG", nil
	case "SELL":
		if reduceOnly {
			return "LONG", nil
		}
		return "SHORT", nil
	default:
		return "", fmt.Errorf("unsupported order side %q", orderSide)
	}
}

func normalizePositionSide(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "ONE_WAY", "ONEWAY":
		return "BOTH"
	default:
		return normalized
	}
}

func normalizePositionModel(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "LONG_SHORT":
		return "DISAGGREGATION"
	case "ONE_WAY", "ONEWAY":
		return "AGGREGATION"
	default:
		return normalized
	}
}

func isAggregationPositionModel(value string) bool {
	return normalizePositionModel(value) == "AGGREGATION"
}

func randomNonce() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func parseDecimal(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func formatDecimal(value float64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isIntegerString(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

type responseEnvelope[T any] struct {
	ReturnCode *int   `json:"returnCode"`
	Code       *int   `json:"code"`
	MsgInfo    string `json:"msgInfo"`
	Msg        string `json:"msg"`
	Data       T      `json:"data"`
}

func (r responseEnvelope[T]) CodeValue() int {
	if r.ReturnCode != nil {
		return *r.ReturnCode
	}
	if r.Code != nil {
		return *r.Code
	}
	return 0
}

func (r responseEnvelope[T]) Message() string {
	if strings.TrimSpace(r.MsgInfo) != "" {
		return r.MsgInfo
	}
	return r.Msg
}

type rawKline struct {
	Symbol      string `json:"s"`
	Timestamp   int64  `json:"t"`
	Open        string `json:"o"`
	Close       string `json:"c"`
	High        string `json:"h"`
	Low         string `json:"l"`
	Volume      string `json:"a"`
	QuoteVolume string `json:"v"`
}

func (r rawKline) toCandle(symbol string, interval string) (marketdata.Candle, error) {
	if r.Timestamp <= 0 {
		return marketdata.Candle{}, errors.New("onebullex kline timestamp is required")
	}
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return marketdata.Candle{}, err
	}
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	openTime := time.UnixMilli(r.Timestamp).UTC()
	return marketdata.Candle{
		Exchange:    ExchangeName,
		MarketType:  marketdata.MarketTypePerpetual,
		Symbol:      symbol,
		Interval:    interval,
		OpenTime:    openTime,
		CloseTime:   openTime.Add(step),
		Open:        r.Open,
		High:        r.High,
		Low:         r.Low,
		Close:       r.Close,
		Volume:      r.Volume,
		QuoteVolume: r.QuoteVolume,
		Source:      ExchangeName,
	}, nil
}

type rawPrice struct {
	Symbol    string `json:"s"`
	Price     string `json:"p"`
	Timestamp int64  `json:"t"`
}

type rawPriceList []rawPrice

func (l *rawPriceList) UnmarshalJSON(data []byte) error {
	var list []rawPrice
	if err := json.Unmarshal(data, &list); err == nil {
		*l = list
		return nil
	}
	var single rawPrice
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*l = []rawPrice{single}
	return nil
}

type rawDeal struct {
	Timestamp int64  `json:"t"`
	Symbol    string `json:"s"`
	Price     string `json:"p"`
	Amount    string `json:"a"`
	Side      string `json:"m"`
}

func (r rawDeal) toTrade(symbol string) marketdata.Trade {
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	tradeID := fmt.Sprintf("%s-%d-%s-%s-%s", symbol, r.Timestamp, r.Price, r.Amount, strings.ToUpper(strings.TrimSpace(r.Side)))
	return marketdata.Trade{
		Exchange:   ExchangeName,
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     symbol,
		TradeID:    tradeID,
		Price:      r.Price,
		Quantity:   r.Amount,
		Side:       r.Side,
		TradeTime:  time.UnixMilli(r.Timestamp).UTC(),
		RawJSON:    rawJSON(r),
	}
}

type rawDepth struct {
	Timestamp int64      `json:"t"`
	Symbol    string     `json:"s"`
	UpdateID  int64      `json:"u"`
	Bids      [][]string `json:"b"`
	Asks      [][]string `json:"a"`
}

func (r rawDepth) toOrderBook(symbol string) (marketdata.OrderBook, error) {
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	bids, err := json.Marshal(r.Bids)
	if err != nil {
		return marketdata.OrderBook{}, err
	}
	asks, err := json.Marshal(r.Asks)
	if err != nil {
		return marketdata.OrderBook{}, err
	}
	return marketdata.OrderBook{
		Exchange:   ExchangeName,
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     symbol,
		EventTime:  time.UnixMilli(r.Timestamp).UTC(),
		UpdateID:   r.UpdateID,
		BidsJSON:   string(bids),
		AsksJSON:   string(asks),
		RawJSON:    rawJSON(r),
	}, nil
}

type rawFundingRate struct {
	Symbol             string `json:"symbol"`
	FundingRate        string `json:"fundingRate"`
	NextCollectionTime int64  `json:"nextCollectionTime"`
}

func (r rawFundingRate) toFundingRate(symbol string, markPrice string) marketdata.FundingRate {
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	return marketdata.FundingRate{
		Exchange:    ExchangeName,
		Symbol:      symbol,
		FundingTime: time.UnixMilli(r.NextCollectionTime).UTC(),
		FundingRate: r.FundingRate,
		MarkPrice:   markPrice,
		IndexPrice:  markPrice,
		RawJSON:     rawJSON(r),
	}
}

type rawFundingRateRecord struct {
	ID                 string `json:"id"`
	Symbol             string `json:"symbol"`
	FundingRate        string `json:"fundingRate"`
	CreatedTime        int64  `json:"createdTime"`
	CollectionInternal int64  `json:"collectionInternal"`
}

func (r rawFundingRateRecord) toFundingRate(symbol string, markPrice string) marketdata.FundingRate {
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	return marketdata.FundingRate{
		Exchange:    ExchangeName,
		Symbol:      symbol,
		FundingTime: time.UnixMilli(r.CreatedTime).UTC(),
		FundingRate: r.FundingRate,
		MarkPrice:   markPrice,
		IndexPrice:  markPrice,
		RawJSON:     rawJSON(r),
	}
}

type rawCursorPage[T any] struct {
	Items []T `json:"items"`
}

func (p *rawCursorPage[T]) UnmarshalJSON(data []byte) error {
	var wrapped struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Items != nil {
		p.Items = wrapped.Items
		return nil
	}
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	p.Items = items
	return nil
}

type rawBalance struct {
	Coin                  string `json:"coin"`
	WalletBalance         string `json:"walletBalance"`
	OpenOrderMarginFrozen string `json:"openOrderMarginFrozen"`
	IsolatedMargin        string `json:"isolatedMargin"`
	CrossedMargin         string `json:"crossedMargin"`
	AvailableBalance      string `json:"availableBalance"`
	Bonus                 string `json:"bonus"`
}

func (r rawBalance) toBalance(snapshotTime time.Time) exchangemodel.Balance {
	locked := parseDecimal(r.OpenOrderMarginFrozen) + parseDecimal(r.IsolatedMargin) + parseDecimal(r.CrossedMargin)
	total := firstNonEmpty(r.WalletBalance, formatDecimal(parseDecimal(r.AvailableBalance)+locked))
	return exchangemodel.Balance{
		Asset:                 strings.ToUpper(strings.TrimSpace(r.Coin)),
		Free:                  r.AvailableBalance,
		Locked:                formatDecimal(locked),
		Total:                 total,
		USDValue:              total,
		WalletBalance:         r.WalletBalance,
		OpenOrderMarginFrozen: r.OpenOrderMarginFrozen,
		IsolatedMargin:        r.IsolatedMargin,
		CrossedMargin:         r.CrossedMargin,
		AvailableBalance:      r.AvailableBalance,
		Bonus:                 r.Bonus,
		RawJSON:               rawJSON(r),
		UpdatedAt:             snapshotTime,
	}
}

type rawPosition struct {
	Symbol         string `json:"symbol"`
	PositionID     string `json:"positionId"`
	PositionType   string `json:"positionType"`
	PositionModel  string `json:"positionModel"`
	PositionSide   string `json:"positionSide"`
	PositionSize   string `json:"positionSize"`
	CloseOrderSize string `json:"closeOrderSize"`
	AvailableClose string `json:"availableCloseSize"`
	EntryPrice     string `json:"entryPrice"`
	IsolatedMargin string `json:"isolatedMargin"`
	OrderMargin    string `json:"openOrderMarginFrozen"`
	RealizedProfit string `json:"realizedProfit"`
	AutoMargin     bool   `json:"autoMargin"`
	Leverage       int    `json:"leverage"`
	ContractSize   string `json:"contractSize"`
}

func (r rawPosition) toPosition(snapshotTime time.Time, markPrice string) exchangemodel.Position {
	unrealized := ""
	quantity := parseDecimal(r.PositionSize)
	entry := parseDecimal(r.EntryPrice)
	mark := parseDecimal(markPrice)
	contractSize := parseDecimal(r.ContractSize)
	if contractSize == 0 {
		contractSize = 1
	}
	if quantity != 0 && entry != 0 && mark != 0 {
		pnl := (mark - entry) * quantity * contractSize
		if strings.EqualFold(r.PositionSide, "SHORT") {
			pnl = (entry - mark) * quantity * contractSize
		}
		unrealized = formatDecimal(pnl)
	}
	return exchangemodel.Position{
		Symbol:         FromExchangeSymbol(r.Symbol),
		PositionID:     r.PositionID,
		PositionSide:   strings.ToLower(strings.TrimSpace(r.PositionSide)),
		PositionModel:  normalizePositionModel(r.PositionModel),
		Quantity:       r.PositionSize,
		CloseOrderSize: r.CloseOrderSize,
		AvailableClose: r.AvailableClose,
		EntryPrice:     r.EntryPrice,
		MarkPrice:      markPrice,
		Leverage:       r.Leverage,
		MarginMode:     strings.ToLower(strings.TrimSpace(r.PositionType)),
		IsolatedMargin: r.IsolatedMargin,
		OrderMargin:    r.OrderMargin,
		RealizedProfit: r.RealizedProfit,
		AutoMargin:     r.AutoMargin,
		ContractSize:   r.ContractSize,
		UnrealizedPnL:  unrealized,
		RawJSON:        rawJSON(r),
		UpdatedAt:      snapshotTime,
	}
}

type rawPositionConfig struct {
	Symbol        string `json:"symbol"`
	PositionType  string `json:"positionType"`
	PositionSide  string `json:"positionSide"`
	PositionModel string `json:"positionModel"`
	AutoMargin    bool   `json:"autoMargin"`
	Leverage      int    `json:"leverage"`
}

func (r rawPositionConfig) toPositionConfig(accountID string, snapshotTime time.Time) portfolio.PositionConfig {
	return portfolio.PositionConfig{
		AccountID:     strings.TrimSpace(accountID),
		Exchange:      ExchangeName,
		Symbol:        FromExchangeSymbol(r.Symbol),
		PositionType:  strings.ToLower(strings.TrimSpace(r.PositionType)),
		PositionSide:  strings.ToLower(strings.TrimSpace(r.PositionSide)),
		PositionModel: normalizePositionModel(r.PositionModel),
		AutoMargin:    r.AutoMargin,
		Leverage:      r.Leverage,
		RawJSON:       rawJSON(r),
		SnapshotTime:  snapshotTime,
	}
}

type rawOrder struct {
	OrderID            string `json:"orderId"`
	PositionID         string `json:"positionId"`
	ClientOrderID      string `json:"clientOrderId"`
	Symbol             string `json:"symbol"`
	OrderType          string `json:"orderType"`
	OrderSide          string `json:"orderSide"`
	PositionSide       string `json:"positionSide"`
	TimeInForce        string `json:"timeInForce"`
	Price              string `json:"price"`
	OrigQty            string `json:"origQty"`
	AvgPrice           string `json:"avgPrice"`
	ExecutedQty        string `json:"executedQty"`
	MarginFrozen       string `json:"marginFrozen"`
	TriggerProfitPrice string `json:"triggerProfitPrice"`
	TriggerStopPrice   string `json:"triggerStopPrice"`
	SourceID           string `json:"sourceId"`
	ForceClose         bool   `json:"forceClose"`
	CloseProfit        string `json:"closeProfit"`
	State              string `json:"state"`
	CreatedTime        int64  `json:"createdTime"`
}

func (r rawOrder) toOrderStatus() exchangemodel.OrderStatus {
	updatedAt := time.Now().UTC()
	if r.CreatedTime > 0 {
		updatedAt = time.UnixMilli(r.CreatedTime).UTC()
	}
	return exchangemodel.OrderStatus{
		ClientOrderID:      r.ClientOrderID,
		ExchangeOrderID:    r.OrderID,
		PositionID:         r.PositionID,
		Symbol:             FromExchangeSymbol(r.Symbol),
		OrderType:          r.OrderType,
		OrderSide:          r.OrderSide,
		PositionSide:       r.PositionSide,
		TimeInForce:        r.TimeInForce,
		Price:              r.Price,
		OrigQty:            r.OrigQty,
		AvgPrice:           r.AvgPrice,
		ExecutedQty:        r.ExecutedQty,
		MarginFrozen:       r.MarginFrozen,
		TriggerProfitPrice: r.TriggerProfitPrice,
		TriggerStopPrice:   r.TriggerStopPrice,
		SourceID:           r.SourceID,
		ForceClose:         r.ForceClose,
		CloseProfit:        r.CloseProfit,
		Status:             r.State,
		RawJSON:            rawJSON(r),
		CreatedAt:          updatedAt,
		UpdatedAt:          updatedAt,
	}
}

type rawSymbolSpec struct {
	Symbol                    string   `json:"symbol"`
	ContractType              string   `json:"contractType"`
	UnderlyingType            string   `json:"underlyingType"`
	ContractSize              string   `json:"contractSize"`
	TradeSwitch               bool     `json:"tradeSwitch"`
	State                     int      `json:"state"`
	InitLeverage              int      `json:"initLeverage"`
	InitPositionType          string   `json:"initPositionType"`
	BaseCoin                  string   `json:"baseCoin"`
	QuoteCoin                 string   `json:"quoteCoin"`
	BaseCoinPrecision         int      `json:"baseCoinPrecision"`
	BaseCoinDisplayPrecision  int      `json:"baseCoinDisplayPrecision"`
	QuoteCoinPrecision        int      `json:"quoteCoinPrecision"`
	QuoteCoinDisplayPrecision int      `json:"quoteCoinDisplayPrecision"`
	QuantityPrecision         int      `json:"quantityPrecision"`
	PricePrecision            int      `json:"pricePrecision"`
	SupportOrderType          string   `json:"supportOrderType"`
	SupportTimeInForce        string   `json:"supportTimeInForce"`
	SupportEntrustType        string   `json:"supportEntrustType"`
	SupportPositionType       string   `json:"supportPositionType"`
	MinPrice                  string   `json:"minPrice"`
	MinQty                    string   `json:"minQty"`
	MinNotional               string   `json:"minNotional"`
	MaxNotional               string   `json:"maxNotional"`
	MultiplierDown            string   `json:"multiplierDown"`
	MultiplierUp              string   `json:"multiplierUp"`
	MaxOpenOrders             int      `json:"maxOpenOrders"`
	MaxEntrusts               int      `json:"maxEntrusts"`
	MakerFee                  string   `json:"makerFee"`
	TakerFee                  string   `json:"takerFee"`
	LiquidationFee            string   `json:"liquidationFee"`
	MarketTakeBound           string   `json:"marketTakeBound"`
	DepthPrecisionMerge       int      `json:"depthPrecisionMerge"`
	Labels                    []string `json:"labels"`
	OnboardDate               int64    `json:"onboardDate"`
	EnglishName               string   `json:"enName"`
	ChineseName               string   `json:"cnName"`
	MinStepPrice              string   `json:"minStepPrice"`
	BaseCoinName              string   `json:"baseCoinName"`
	QuoteCoinName             string   `json:"quoteCoinName"`
}

func (r rawSymbolSpec) toContractSpec() portfolio.ContractSpec {
	labels, _ := json.Marshal(r.Labels)
	return portfolio.ContractSpec{
		Exchange:                  ExchangeName,
		Symbol:                    FromExchangeSymbol(r.Symbol),
		ContractType:              r.ContractType,
		UnderlyingType:            r.UnderlyingType,
		ContractSize:              r.ContractSize,
		TradeSwitch:               r.TradeSwitch,
		State:                     r.State,
		InitLeverage:              r.InitLeverage,
		InitPositionType:          r.InitPositionType,
		BaseAsset:                 r.BaseCoin,
		QuoteAsset:                r.QuoteCoin,
		BaseCoinPrecision:         r.BaseCoinPrecision,
		BaseCoinDisplayPrecision:  r.BaseCoinDisplayPrecision,
		QuoteCoinPrecision:        r.QuoteCoinPrecision,
		QuoteCoinDisplayPrecision: r.QuoteCoinDisplayPrecision,
		QuantityPrecision:         r.QuantityPrecision,
		PricePrecision:            r.PricePrecision,
		SupportOrderType:          r.SupportOrderType,
		SupportTimeInForce:        r.SupportTimeInForce,
		SupportEntrustType:        r.SupportEntrustType,
		SupportPositionType:       r.SupportPositionType,
		MinPrice:                  r.MinPrice,
		MinQty:                    r.MinQty,
		MinNotional:               r.MinNotional,
		MaxNotional:               r.MaxNotional,
		MultiplierDown:            r.MultiplierDown,
		MultiplierUp:              r.MultiplierUp,
		MaxOpenOrders:             r.MaxOpenOrders,
		MaxEntrusts:               r.MaxEntrusts,
		MakerFee:                  r.MakerFee,
		TakerFee:                  r.TakerFee,
		LiquidationFee:            r.LiquidationFee,
		MarketTakeBound:           r.MarketTakeBound,
		DepthPrecisionMerge:       r.DepthPrecisionMerge,
		LabelsJSON:                string(labels),
		OnboardTime:               time.UnixMilli(r.OnboardDate).UTC(),
		EnglishName:               r.EnglishName,
		ChineseName:               r.ChineseName,
		MinStepPrice:              r.MinStepPrice,
		BaseCoinName:              r.BaseCoinName,
		QuoteCoinName:             r.QuoteCoinName,
		TickSize:                  r.MinStepPrice,
		StepSize:                  r.MinQty,
		RawJSON:                   rawJSON(r),
	}
}

type rawSymbolBracket struct {
	Symbol           string               `json:"symbol"`
	LeverageBrackets []rawLeverageBracket `json:"leverageBrackets"`
}

func (r rawSymbolBracket) toLeverageBrackets() []portfolio.LeverageBracket {
	symbol := FromExchangeSymbol(r.Symbol)
	brackets := make([]portfolio.LeverageBracket, 0, len(r.LeverageBrackets))
	for _, item := range r.LeverageBrackets {
		brackets = append(brackets, item.toLeverageBracket(symbol))
	}
	return brackets
}

type rawLeverageBracket struct {
	Symbol             string `json:"symbol"`
	Bracket            int    `json:"bracket"`
	MaxNominalValue    string `json:"maxNominalValue"`
	MaintMarginRate    string `json:"maintMarginRate"`
	StartMarginRate    string `json:"startMarginRate"`
	MaxStartMarginRate string `json:"maxStartMarginRate"`
	MaxLeverage        string `json:"maxLeverage"`
	MinLeverage        string `json:"minLeverage"`
}

func (r rawLeverageBracket) toLeverageBracket(symbol string) portfolio.LeverageBracket {
	if r.Symbol != "" {
		symbol = FromExchangeSymbol(r.Symbol)
	}
	return portfolio.LeverageBracket{
		Exchange:           ExchangeName,
		Symbol:             symbol,
		Bracket:            r.Bracket,
		MaxNominalValue:    r.MaxNominalValue,
		MaintMarginRate:    r.MaintMarginRate,
		StartMarginRate:    r.StartMarginRate,
		MaxStartMarginRate: r.MaxStartMarginRate,
		MaxLeverage:        r.MaxLeverage,
		MinLeverage:        r.MinLeverage,
		RawJSON:            rawJSON(r),
	}
}

func rawJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
