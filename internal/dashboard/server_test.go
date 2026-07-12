package dashboard

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gogogo/internal/storage"

	_ "modernc.org/sqlite"
)

func TestDashboardAPI(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.InitSQLiteSchema(t.Context(), db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	seedDashboardData(t, db)

	server := NewServer(db, "")
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard?market=spot&symbol=BTCUSDT&interval=1h&limit=20", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var data Data
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := len(data.PriceSeries); got != 2 {
		t.Fatalf("price series length = %d, want 2", got)
	}
	if data.Counts["candles"] != 2 {
		t.Fatalf("candles count = %d, want 2", data.Counts["candles"])
	}
	if len(data.Backtests) != 1 {
		t.Fatalf("backtests length = %d, want 1", len(data.Backtests))
	}
}

func TestDashboardPage(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	server := NewServer(db, "")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
}

func seedDashboardData(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	_, err := db.ExecContext(t.Context(), `
INSERT INTO candles (
	exchange, market_type, symbol, interval, open_time, close_time,
	open_price, high_price, low_price, close_price, volume, quote_volume,
	trade_count, source, created_at, updated_at
) VALUES
	('binance', 'spot', 'BTCUSDT', '1h', ?, ?, '100', '110', '95', '105', '10', '1000', 10, 'test', ?, ?),
	('binance', 'spot', 'BTCUSDT', '1h', ?, ?, '105', '115', '100', '112', '12', '1300', 12, 'test', ?, ?);

INSERT INTO backtest_runs (
	strategy_name, exchange, market_type, symbol, interval, start_time, end_time,
	fast_window, slow_window, fee_rate, initial_equity, final_equity, total_return_pct,
	buy_hold_return_pct, excess_return_pct, max_drawdown_pct, trade_count, win_rate_pct, created_at
) VALUES (
	'sma_crossover_12_48', 'binance', 'spot', 'BTCUSDT', '1h', ?, ?, 12, 48,
	0.001, 10000, 10100, 1, 0.5, 0.5, 2, 3, 66.6, ?
);
`, now, now.Add(time.Hour), now, now, now.Add(time.Hour), now.Add(2*time.Hour), now, now, now, now, now)
	if err != nil {
		t.Fatalf("seed data: %v", err)
	}
}
