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
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard?exchange=onebullex&market=perpetual&symbol=BTCUSDT&interval=1h&limit=20", nil)
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
	if len(data.Positions) != 1 {
		t.Fatalf("positions length = %d, want 1", len(data.Positions))
	}
	if data.Positions[0].MarkPrice != 64000 {
		t.Fatalf("position mark price = %.2f, want latest mark 64000", data.Positions[0].MarkPrice)
	}
	if data.Positions[0].MarkPriceSource != "latest_mark_price" {
		t.Fatalf("position mark source = %q, want latest_mark_price", data.Positions[0].MarkPriceSource)
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

func TestDashboardFiltersLegacyPaperPositionSnapshots(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.InitSQLiteSchema(t.Context(), db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO positions (
	account_id, exchange, market_type, symbol, position_side, quantity, entry_price,
	mark_price, liquidation_price, leverage, margin_mode, unrealized_pnl, notional,
	liquidation_distance_pct, snapshot_time, created_at
) VALUES
	('paper', 'onebullex', 'perpetual', 'BTCUSDT', 'long', 0.01, 60000,
		61000, 50000, 2, 'isolated', 10, 610, 18, ?, ?);
`, now.Add(-time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("seed positions: %v", err)
	}

	server := NewServer(db, "")
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard?exchange=onebullex&market=perpetual&symbol=BTCUSDT&interval=1m", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var data Data
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(data.Positions) != 0 {
		t.Fatalf("positions length = %d, want 0; positions = %+v", len(data.Positions), data.Positions)
	}
}

func TestDashboardUsesLatestMarkForPerpetualPaperPosition(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := storage.InitSQLiteSchema(t.Context(), db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	now := time.Date(2026, 7, 13, 0, 45, 0, 0, time.UTC)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO mark_prices (
	exchange, symbol, event_time, mark_price, index_price, estimated_settle_price,
	next_funding_time, created_at
) VALUES (
	'onebullex', 'BTCUSDT', ?, '63650', '63670', '0', ?, ?
);

INSERT INTO positions (
	account_id, exchange, market_type, symbol, position_side, quantity, entry_price,
	mark_price, liquidation_price, leverage, margin_mode, unrealized_pnl, notional,
	liquidation_distance_pct, snapshot_time, created_at
) VALUES
	('paper', 'onebullex', 'perpetual', 'BTCUSDT', 'short', 0.01, 63713.8,
		63650, 0, 1, 'paper', 0.638, 636.5, 0, ?, ?);
`, now, now.Add(8*time.Hour), now, now, now)
	if err != nil {
		t.Fatalf("seed perpetual position: %v", err)
	}

	server := NewServer(db, "")
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard?exchange=onebullex&market=perpetual&symbol=BTCUSDT&interval=1m", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var data Data
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(data.Positions) != 1 {
		t.Fatalf("positions length = %d, want 1; positions = %+v", len(data.Positions), data.Positions)
	}
	if data.Positions[0].MarketType != "perpetual" {
		t.Fatalf("position market type = %q, want perpetual", data.Positions[0].MarketType)
	}
	if data.Positions[0].PositionSide != "short" {
		t.Fatalf("position side = %q, want short", data.Positions[0].PositionSide)
	}
	if data.Positions[0].MarkPriceSource != "latest_mark_price" {
		t.Fatalf("mark source = %q, want latest_mark_price", data.Positions[0].MarkPriceSource)
	}
}

func TestTablePreviewAPI(t *testing.T) {
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
	request := httptest.NewRequest(http.MethodGet, "/api/table?name=backtest_runs&limit=5", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var preview TablePreview
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if preview.Name != "backtest_runs" {
		t.Fatalf("table name = %q", preview.Name)
	}
	if preview.TotalRows != 1 {
		t.Fatalf("total rows = %d, want 1", preview.TotalRows)
	}
	if len(preview.Rows) != 1 {
		t.Fatalf("rows length = %d, want 1", len(preview.Rows))
	}
	if preview.Rows[0]["strategy_name"] != "sma_crossover_12_48" {
		t.Fatalf("strategy name = %q", preview.Rows[0]["strategy_name"])
	}
}

func TestTablePreviewRejectsUnsupportedTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	server := NewServer(db, "")
	request := httptest.NewRequest(http.MethodGet, "/api/table?name=sqlite_master", nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
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
	('onebullex', 'perpetual', 'BTCUSDT', '1h', ?, ?, '100', '110', '95', '105', '10', '1000', 10, 'test', ?, ?),
	('onebullex', 'perpetual', 'BTCUSDT', '1h', ?, ?, '105', '115', '100', '112', '12', '1300', 12, 'test', ?, ?);

INSERT INTO backtest_runs (
	strategy_name, exchange, market_type, symbol, interval, start_time, end_time,
	fast_window, slow_window, fee_rate, initial_equity, final_equity, total_return_pct,
	buy_hold_return_pct, excess_return_pct, max_drawdown_pct, trade_count, win_rate_pct, created_at
) VALUES (
	'sma_crossover_12_48', 'onebullex', 'perpetual', 'BTCUSDT', '1h', ?, ?, 12, 48,
	0.001, 10000, 10100, 1, 0.5, 0.5, 2, 3, 66.6, ?
);

INSERT INTO positions (
	account_id, exchange, market_type, symbol, position_side, quantity, entry_price,
	mark_price, liquidation_price, leverage, margin_mode, unrealized_pnl, notional,
	liquidation_distance_pct, snapshot_time, created_at
) VALUES (
	'live-readonly', 'onebullex', 'perpetual', 'BTCUSDT', 'long', 0.01, 60000,
	61000, 50000, 2, 'isolated', 10, 610, 18, ?, ?
);

INSERT INTO mark_prices (
	exchange, symbol, event_time, mark_price, index_price, estimated_settle_price,
	next_funding_time, created_at
) VALUES (
	'onebullex', 'BTCUSDT', ?, '64000', '63990', '0', ?, ?
);
`, now, now.Add(time.Hour), now, now,
		now.Add(time.Hour), now.Add(2*time.Hour), now, now,
		now, now, now,
		now, now,
		now, now.Add(8*time.Hour), now)
	if err != nil {
		t.Fatalf("seed data: %v", err)
	}
}
