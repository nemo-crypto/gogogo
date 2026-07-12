package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gogogo/internal/marketdata"
)

func TestClientKlinesSpot(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/klines" {
			t.Fatalf("path = %s, want /api/v3/klines", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Fatalf("symbol = %s, want BTCUSDT", got)
		}
		if got := r.URL.Query().Get("interval"); got != "1h" {
			t.Fatalf("interval = %s, want 1h", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[[1655971200000,"43631.23","43631.24","43620.00","43625.10","12.5",1655974799999,"545312.50",42,"0","0","0"]]`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURLs(server.URL, server.URL), WithHTTPClient(server.Client()))

	candles, err := client.Klines(context.Background(), KlineRequest{
		MarketType: marketdata.MarketTypeSpot,
		Symbol:     "btc/usdt",
		Interval:   "1h",
		Limit:      5000,
	})
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("candles length = %d, want 1", len(candles))
	}
	if candles[0].Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", candles[0].Symbol)
	}
	if candles[0].MarketType != marketdata.MarketTypeSpot {
		t.Fatalf("market type = %q, want spot", candles[0].MarketType)
	}
	if candles[0].CloseTime != time.UnixMilli(1655974800000).UTC() {
		t.Fatalf("close time = %s, want adjusted close time", candles[0].CloseTime)
	}
}

func TestClientKlinesPerpetual(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/klines" {
			t.Fatalf("path = %s, want /fapi/v1/klines", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[[1655971200000,"43631.23","43631.24","43620.00","43625.10","12.5",1655974799999,"545312.50",42,"0","0","0"]]`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURLs(server.URL, server.URL), WithHTTPClient(server.Client()))

	candles, err := client.Klines(context.Background(), KlineRequest{
		MarketType: marketdata.MarketTypePerpetual,
		Symbol:     "ETHUSDT",
		Interval:   "15m",
	})
	if err != nil {
		t.Fatalf("klines: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("candles length = %d, want 1", len(candles))
	}
	if candles[0].MarketType != marketdata.MarketTypePerpetual {
		t.Fatalf("market type = %q, want perpetual", candles[0].MarketType)
	}
}

func TestClientFundingRates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/fundingRate" {
			t.Fatalf("path = %s, want /fapi/v1/fundingRate", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Fatalf("symbol = %s, want BTCUSDT", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","fundingRate":"0.00010000","fundingTime":1655971200000,"markPrice":"43625.10"}]`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURLs(server.URL, server.URL), WithHTTPClient(server.Client()))

	rates, err := client.FundingRates(context.Background(), FundingRateRequest{
		Symbol: "btcusdt",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("funding rates: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("rates length = %d, want 1", len(rates))
	}
	if rates[0].FundingRate != "0.00010000" {
		t.Fatalf("funding rate = %q, want 0.00010000", rates[0].FundingRate)
	}
	if rates[0].IndexPrice != "" {
		t.Fatalf("index price = %q, want empty", rates[0].IndexPrice)
	}
}

func TestClientServerTime(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/time" {
			t.Fatalf("path = %s, want /api/v3/time", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"serverTime":1655971200000}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURLs(server.URL, ""))
	got, err := client.ServerTime(context.Background(), marketdata.MarketTypeSpot)
	if err != nil {
		t.Fatalf("server time: %v", err)
	}
	if got.UnixMilli() != 1655971200000 {
		t.Fatalf("server time = %d, want 1655971200000", got.UnixMilli())
	}
}

func TestClientLatestMarkPrice(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/premiumIndex" {
			t.Fatalf("path = %s, want /fapi/v1/premiumIndex", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","markPrice":"43625.10","indexPrice":"43620.00","estimatedSettlePrice":"0.00000000","lastFundingRate":"0.00010000","nextFundingTime":1656000000000,"time":1655971200000}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURLs(server.URL, server.URL), WithHTTPClient(server.Client()))

	price, err := client.LatestMarkPrice(context.Background(), "btcusdt")
	if err != nil {
		t.Fatalf("latest mark price: %v", err)
	}
	if price.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", price.Symbol)
	}
	if price.MarkPrice != "43625.10" {
		t.Fatalf("mark price = %q, want 43625.10", price.MarkPrice)
	}
	if price.NextFundingTime != time.UnixMilli(1656000000000).UTC() {
		t.Fatalf("next funding time = %s, want parsed time", price.NextFundingTime)
	}
}
