package onebullex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gogogo/internal/exchange"
	"gogogo/internal/marketdata"
)

func TestSignaturePayloadMatchesDocsExamples(t *testing.T) {
	t.Parallel()

	getPayload := SignaturePayload(map[string]string{
		"nonce":         "abc123xyz",
		"timestamp":     "1711900000",
		"symbol":        "btc_usdt",
		"timeRangeType": "UTC8",
	})
	if getPayload != "nonce=abc123xyz&symbol=btc_usdt&timeRangeType=UTC8&timestamp=1711900000" {
		t.Fatalf("GET signature payload = %q", getPayload)
	}

	postPayload := SignaturePayload(map[string]string{
		"nonce":        "def456uvw",
		"timestamp":    "1711900000",
		"symbol":       "btc_usdt",
		"orderSide":    "BUY",
		"price":        "50000",
		"origQty":      "1",
		"orderType":    "LIMIT",
		"positionSide": "LONG",
		"timeInForce":  "GTC",
	})
	want := "nonce=def456uvw&orderSide=BUY&orderType=LIMIT&origQty=1&positionSide=LONG&price=50000&symbol=btc_usdt&timeInForce=GTC&timestamp=1711900000"
	if postPayload != want {
		t.Fatalf("POST signature payload = %q, want %q", postPayload, want)
	}
}

func TestSymbolConversion(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"BTCUSDT":  "btc_usdt",
		"btc/usdt": "btc_usdt",
		"eth_usdt": "eth_usdt",
		"SOL-USDT": "sol_usdt",
	}
	for input, want := range tests {
		if got := ToExchangeSymbol(input); got != want {
			t.Fatalf("ToExchangeSymbol(%q) = %q, want %q", input, got, want)
		}
	}
	if got := FromExchangeSymbol("btc_usdt"); got != "BTCUSDT" {
		t.Fatalf("FromExchangeSymbol = %q, want BTCUSDT", got)
	}
}

func TestClientKlines(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/public/q/kline" {
			t.Fatalf("path = %s, want /v2/public/q/kline", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "btc_usdt" {
			t.Fatalf("symbol = %s, want btc_usdt", got)
		}
		_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","t":1655971200000,"o":"43631.23","h":"43631.24","l":"43620.00","c":"43625.10","a":"12.5","v":"545312.50"}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	candles, err := client.Klines(context.Background(), exchange.KlineRequest{
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "1h",
	})
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("candles length = %d, want 1", len(candles))
	}
	if candles[0].Exchange != ExchangeName || candles[0].Symbol != "BTCUSDT" {
		t.Fatalf("candle identity = %s %s", candles[0].Exchange, candles[0].Symbol)
	}
	if candles[0].CloseTime != time.UnixMilli(1655971200000).UTC().Add(time.Hour) {
		t.Fatalf("close time = %s, want open + 1h", candles[0].CloseTime)
	}
}

func TestClientKlinesAcceptsCodeEnvelopeAndSortsCandles(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":[{"s":"btc_usdt","t":1655971500000,"o":"2","h":"2","l":"2","c":"2","a":"2","v":"2"},{"s":"btc_usdt","t":1655971200000,"o":"1","h":"1","l":"1","c":"1","a":"1","v":"1"}],"bizCode":null}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	candles, err := client.Klines(context.Background(), exchange.KlineRequest{
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "5m",
	})
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("candles length = %d, want 2", len(candles))
	}
	if !candles[0].OpenTime.Before(candles[1].OpenTime) {
		t.Fatalf("candles are not sorted ascending: %s >= %s", candles[0].OpenTime, candles[1].OpenTime)
	}
}

func TestClientKlinesRejectsCodeEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":-1,"msg":"invalid interval","data":null,"bizCode":"10102"}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	_, err := client.Klines(context.Background(), exchange.KlineRequest{
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "3m",
	})
	if err == nil {
		t.Fatal("klines error = nil, want invalid interval error")
	}
	if !strings.Contains(err.Error(), "code=-1") || !strings.Contains(err.Error(), "invalid interval") {
		t.Fatalf("klines error = %q, want code and message", err.Error())
	}
}

func TestClientRetriesTransientGET(t *testing.T) {
	t.Parallel()

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/public/q/kline" {
			t.Fatalf("path = %s, want /v2/public/q/kline", r.URL.Path)
		}
		if atomic.AddInt32(&attempts, 1) == 1 {
			http.Error(w, "temporary unavailable", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","t":1655971200000,"o":"43631.23","h":"43631.24","l":"43620.00","c":"43625.10","a":"12.5","v":"545312.50"}]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithRetryPolicy(3, 0))
	candles, err := client.Klines(context.Background(), exchange.KlineRequest{
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Interval:   "1h",
	})
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("candles length = %d, want 1", len(candles))
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestClientLatestMarkPrice(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/public/q/symbol-mark-price":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"s":"btc_usdt","p":"43625.10","t":1655971200000}}`))
		case "/v2/public/q/index-price":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","p":"43620.00","t":1655971200000}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	price, err := client.LatestMarkPrice(context.Background(), "btcusdt")
	if err != nil {
		t.Fatalf("latest mark price: %v", err)
	}
	if price.Exchange != ExchangeName || price.Symbol != "BTCUSDT" {
		t.Fatalf("price identity = %s %s", price.Exchange, price.Symbol)
	}
	if price.MarkPrice != "43625.10" || price.IndexPrice != "43620.00" {
		t.Fatalf("prices = mark %s index %s", price.MarkPrice, price.IndexPrice)
	}
}

func TestClientPublicMetadataDatasets(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/public/symbol/list":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"symbol":"btc_usdt","contractType":"PERPETUAL","underlyingType":"USDT","contractSize":"1","tradeSwitch":true,"state":1,"initLeverage":10,"initPositionType":"CROSSED","baseCoin":"btc","quoteCoin":"usdt","quantityPrecision":3,"pricePrecision":1,"supportOrderType":"LIMIT,MARKET","supportTimeInForce":"GTC,IOC","supportPositionType":"LONG,SHORT","minPrice":"0.1","minQty":"0.001","minNotional":"5","maxNotional":"100000","makerFee":"0.0002","takerFee":"0.0004","labels":["perp"],"onboardDate":1655971200000,"minStepPrice":"0.1"}]}`))
		case "/v2/public/leverage/bracket/detail":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"symbol":"btc_usdt","leverageBrackets":[{"symbol":"btc_usdt","bracket":1,"maxNominalValue":"100000","maintMarginRate":"0.005","startMarginRate":"0.01","maxStartMarginRate":"0.02","maxLeverage":"100","minLeverage":"1"}]}}`))
		case "/v2/public/q/index-price":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","p":"60000.00","t":1655971200000}]}`))
		case "/v2/public/q/deal":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","t":1655971200000,"p":"60001.00","a":"0.01","m":"BUY"}]}`))
		case "/v2/public/q/depth":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"s":"btc_usdt","t":1655971200000,"u":99,"b":[["60000","1"]],"a":[["60001","2"]]}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	specs, err := client.SymbolSpecs(context.Background())
	if err != nil {
		t.Fatalf("symbol specs: %v", err)
	}
	if len(specs) != 1 || specs[0].Symbol != "BTCUSDT" || specs[0].MinStepPrice != "0.1" {
		t.Fatalf("specs = %+v", specs)
	}
	brackets, err := client.LeverageBrackets(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("leverage brackets: %v", err)
	}
	if len(brackets) != 1 || brackets[0].MaxLeverage != "100" {
		t.Fatalf("brackets = %+v", brackets)
	}
	indexPrices, err := client.IndexPrices(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("index prices: %v", err)
	}
	if len(indexPrices) != 1 || indexPrices[0].IndexPrice != "60000.00" {
		t.Fatalf("index prices = %+v", indexPrices)
	}
	trades, err := client.RecentTrades(context.Background(), "BTCUSDT", 1)
	if err != nil {
		t.Fatalf("recent trades: %v", err)
	}
	if len(trades) != 1 || trades[0].Side != "BUY" {
		t.Fatalf("trades = %+v", trades)
	}
	book, err := client.OrderBook(context.Background(), "BTCUSDT", 1)
	if err != nil {
		t.Fatalf("order book: %v", err)
	}
	if book.UpdateID != 99 || book.BidsJSON == "" || book.AsksJSON == "" {
		t.Fatalf("book = %+v", book)
	}
}

func TestClientAccountSnapshotUsesSignedRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/balance/list":
			assertSignedHeaders(t, r)
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"coin":"USDT","walletBalance":"1000","openOrderMarginFrozen":"5","isolatedMargin":"20","crossedMargin":"10","availableBalance":"965","bonus":"0"}]}`))
		case "/v2/position/list":
			assertSignedHeaders(t, r)
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"symbol":"btc_usdt","positionId":"1","positionType":"ISOLATED","positionModel":"AGGREGATION","positionSide":"LONG","positionSize":"0.01","entryPrice":"43000","realizedProfit":"0","leverage":3,"contractSize":"1"}]}`))
		case "/v2/public/time":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":1655971200000}`))
		case "/v2/public/q/symbol-mark-price":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"s":"btc_usdt","p":"43625.10","t":1655971200000}}`))
		case "/v2/public/q/index-price":
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"s":"btc_usdt","p":"43620.00","t":1655971200000}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("key", "secret"),
		WithNonceFunc(func() (string, error) { return "nonce", nil }),
		WithNowFunc(func() time.Time { return time.Unix(1655971200, 0).UTC() }),
	)
	snapshot, err := client.AccountSnapshot(context.Background(), "paper")
	if err != nil {
		t.Fatalf("account snapshot: %v", err)
	}
	if snapshot.Exchange != ExchangeName || !snapshot.ReadOnly {
		t.Fatalf("snapshot exchange/readOnly = %s/%t", snapshot.Exchange, snapshot.ReadOnly)
	}
	if len(snapshot.Balances) != 1 || snapshot.Balances[0].Locked != "35" {
		t.Fatalf("balances = %+v", snapshot.Balances)
	}
	if len(snapshot.Positions) != 1 {
		t.Fatalf("positions length = %d, want 1", len(snapshot.Positions))
	}
	position := snapshot.Positions[0]
	if position.Symbol != "BTCUSDT" || position.PositionSide != "long" || position.PositionModel != "AGGREGATION" || position.MarkPrice != "43625.10" {
		t.Fatalf("position = %+v", position)
	}
}

func TestSubmitOrderDisabledByDefault(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.SubmitOrder(context.Background(), exchange.OrderRequest{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		OrderType: "MARKET",
		Quantity:  "1",
	})
	if err != ErrLiveTradingDisabled {
		t.Fatalf("submit order err = %v, want ErrLiveTradingDisabled", err)
	}
}

func TestSubmitOrderSendsNativeTPSL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/order/create":
			assertSignedHeaders(t, r)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode order body: %v", err)
			}
			if body["triggerProfitPrice"] != "60900" {
				t.Fatalf("triggerProfitPrice = %v, want 60900", body["triggerProfitPrice"])
			}
			if body["triggerStopPrice"] != "59700" {
				t.Fatalf("triggerStopPrice = %v, want 59700", body["triggerStopPrice"])
			}
			if body["profitOrderType"] != "MARKET" || body["stopOrderType"] != "MARKET" {
				t.Fatalf("order types = %v/%v, want MARKET/MARKET", body["profitOrderType"], body["stopOrderType"])
			}
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":"123456"}`))
		case "/v2/order/detail":
			if got := r.URL.Query().Get("orderId"); got != "123456" {
				t.Fatalf("orderId = %s, want 123456", got)
			}
			assertSignedHeaders(t, r)
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"orderId":"123456","clientOrderId":"native-tpsl","state":"FILLED","executedQty":"0.001","createdTime":1655971200000}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("key", "secret"),
		WithTradingEnabled(true),
		WithNonceFunc(func() (string, error) { return "nonce", nil }),
		WithNowFunc(func() time.Time { return time.Unix(1655971200, 0).UTC() }),
	)
	status, err := client.SubmitOrder(context.Background(), exchange.OrderRequest{
		Symbol:             "BTCUSDT",
		ClientOrderID:      "native-tpsl",
		Side:               "BUY",
		OrderType:          "MARKET",
		TimeInForce:        "IOC",
		Quantity:           "0.001",
		TriggerProfitPrice: "60900",
		TriggerStopPrice:   "59700",
		ProfitOrderType:    "MARKET",
		StopOrderType:      "MARKET",
	})
	if err != nil {
		t.Fatalf("submit order: %v", err)
	}
	if status.ExchangeOrderID != "123456" || status.Status != "FILLED" {
		t.Fatalf("status = %+v, want filled 123456", status)
	}
}

func TestClientPositionConfigs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/position/confs" {
			t.Fatalf("path = %s, want /v2/position/confs", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "btc_usdt" {
			t.Fatalf("symbol = %s, want btc_usdt", got)
		}
		assertSignedHeaders(t, r)
		_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":[{"symbol":"btc_usdt","positionType":"CROSSED","positionSide":"BOTH","positionModel":"AGGREGATION","autoMargin":true,"leverage":10}]}`))
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("key", "secret"),
		WithNonceFunc(func() (string, error) { return "nonce", nil }),
		WithNowFunc(func() time.Time { return time.Unix(1655971200, 0).UTC() }),
	)
	configs, err := client.PositionConfigs(context.Background(), "research", "BTCUSDT")
	if err != nil {
		t.Fatalf("position configs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("configs length = %d, want 1", len(configs))
	}
	config := configs[0]
	if config.Symbol != "BTCUSDT" || config.PositionSide != "both" || config.PositionModel != "AGGREGATION" || config.Leverage != 10 {
		t.Fatalf("config = %+v", config)
	}
}

func TestSubmitOrderUsesBothForAggregationPositionModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/order/create":
			assertSignedHeaders(t, r)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode order body: %v", err)
			}
			if body["positionSide"] != "BOTH" {
				t.Fatalf("positionSide = %v, want BOTH", body["positionSide"])
			}
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":"123456"}`))
		case "/v2/order/detail":
			assertSignedHeaders(t, r)
			_, _ = w.Write([]byte(`{"returnCode":0,"msgInfo":"success","data":{"orderId":"123456","clientOrderId":"oneway","positionSide":"BOTH","state":"NEW","createdTime":1655971200000}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("key", "secret"),
		WithTradingEnabled(true),
		WithNonceFunc(func() (string, error) { return "nonce", nil }),
		WithNowFunc(func() time.Time { return time.Unix(1655971200, 0).UTC() }),
	)
	status, err := client.SubmitOrder(context.Background(), exchange.OrderRequest{
		Symbol:        "BTCUSDT",
		ClientOrderID: "oneway",
		Side:          "BUY",
		OrderType:     "MARKET",
		PositionModel: "aggregation",
		Quantity:      "0.001",
	})
	if err != nil {
		t.Fatalf("submit order: %v", err)
	}
	if status.PositionSide != "BOTH" {
		t.Fatalf("status position side = %q, want BOTH", status.PositionSide)
	}
}

func TestSubmitOrderDoesNotRetryPOST(t *testing.T) {
	t.Parallel()

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/order/create" {
			t.Fatalf("path = %s, want /v2/order/create", r.URL.Path)
		}
		assertSignedHeaders(t, r)
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "temporary unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("key", "secret"),
		WithTradingEnabled(true),
		WithNonceFunc(func() (string, error) { return "nonce", nil }),
		WithNowFunc(func() time.Time { return time.Unix(1655971200, 0).UTC() }),
		WithRetryPolicy(3, 0),
	)
	_, err := client.SubmitOrder(context.Background(), exchange.OrderRequest{
		Symbol:    "BTCUSDT",
		Side:      "BUY",
		OrderType: "MARKET",
		Quantity:  "0.001",
	})
	if err == nil {
		t.Fatal("submit order err = nil, want status error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func assertSignedHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("X-API-KEY") != "key" {
		t.Fatalf("X-API-KEY = %q", r.Header.Get("X-API-KEY"))
	}
	if r.Header.Get("X-Nonce") != "nonce" {
		t.Fatalf("X-Nonce = %q", r.Header.Get("X-Nonce"))
	}
	if r.Header.Get("X-Timestamp") != "1655971200" {
		t.Fatalf("X-Timestamp = %q", r.Header.Get("X-Timestamp"))
	}
	if r.Header.Get("X-Signature") == "" {
		t.Fatal("X-Signature is empty")
	}
}
