package dashboard

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed static/*
var content embed.FS

type Server struct {
	db       *sql.DB
	haltFile string
}

func NewServer(db *sql.DB, haltFile string) *Server {
	if haltFile == "" {
		haltFile = ".runtime/halt"
	}
	return &Server{db: db, haltFile: haltFile}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /favicon.ico", s.favicon)
	mux.HandleFunc("GET /api/dashboard", s.dashboard)
	mux.HandleFunc("GET /api/table", s.table)

	staticFS, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return logRequests(mux)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := content.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "dashboard page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	query := dashboardQuery{
		Exchange:   firstNonEmpty(r.URL.Query().Get("exchange"), "binance"),
		MarketType: firstNonEmpty(r.URL.Query().Get("market"), "spot"),
		Symbol:     strings.ToUpper(firstNonEmpty(r.URL.Query().Get("symbol"), "BTCUSDT")),
		Interval:   firstNonEmpty(r.URL.Query().Get("interval"), "1h"),
		Limit:      parseLimit(r.URL.Query().Get("limit"), 240, 1000),
	}

	data, err := s.collect(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) table(w http.ResponseWriter, r *http.Request) {
	tableName := strings.TrimSpace(r.URL.Query().Get("name"))
	if tableName == "" {
		tableName = "backtest_runs"
	}
	if !isAllowedTable(tableName) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported table"})
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 50, 200)
	data, err := s.loadTablePreview(r.Context(), tableName, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, data)
}

type dashboardQuery struct {
	Exchange   string `json:"exchange"`
	MarketType string `json:"market_type"`
	Symbol     string `json:"symbol"`
	Interval   string `json:"interval"`
	Limit      int    `json:"limit"`
}

type Data struct {
	GeneratedAt          time.Time             `json:"generated_at"`
	Query                dashboardQuery        `json:"query"`
	Runtime              RuntimeState          `json:"runtime"`
	Counts               map[string]int64      `json:"counts"`
	MarketCoverage       []MarketCoverage      `json:"market_coverage"`
	PriceSeries          []CandlePoint         `json:"price_series"`
	Backtests            []BacktestRun         `json:"backtests"`
	CandleSnapshots      []CandleSnapshot      `json:"candle_snapshots"`
	Orders               []OrderRecord         `json:"orders"`
	RiskEvents           []RiskEvent           `json:"risk_events"`
	Balances             []BalanceSnapshot     `json:"balances"`
	Positions            []PositionSnapshot    `json:"positions"`
	Margins              []MarginSnapshot      `json:"margins"`
	StrategyRuns         []StrategyRun         `json:"strategy_runs"`
	Signals              []SignalRecord        `json:"signals"`
	PerformanceSnapshots []PerformanceSnapshot `json:"performance_snapshots"`
	FundingRates         []FundingRate         `json:"funding_rates"`
	MarkPrices           []MarkPrice           `json:"mark_prices"`
	Warnings             []string              `json:"warnings,omitempty"`
}

type TablePreview struct {
	Name       string              `json:"name"`
	Columns    []string            `json:"columns"`
	Rows       []map[string]string `json:"rows"`
	TotalRows  int64               `json:"total_rows"`
	Limit      int                 `json:"limit"`
	LoadedAt   time.Time           `json:"loaded_at"`
	SortColumn string              `json:"sort_column"`
}

type RuntimeState struct {
	Halted   bool   `json:"halted"`
	HaltFile string `json:"halt_file"`
}

type MarketCoverage struct {
	Exchange   string  `json:"exchange"`
	MarketType string  `json:"market_type"`
	Symbol     string  `json:"symbol"`
	Interval   string  `json:"interval"`
	Candles    int64   `json:"candles"`
	FirstTime  string  `json:"first_time"`
	LastTime   string  `json:"last_time"`
	LastClose  float64 `json:"last_close"`
}

type CandlePoint struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

type BacktestRun struct {
	ID               int64     `json:"id"`
	StrategyName     string    `json:"strategy_name"`
	Exchange         string    `json:"exchange"`
	MarketType       string    `json:"market_type"`
	Symbol           string    `json:"symbol"`
	Interval         string    `json:"interval"`
	FastWindow       int64     `json:"fast_window"`
	SlowWindow       int64     `json:"slow_window"`
	FeeRate          float64   `json:"fee_rate"`
	TotalReturnPct   float64   `json:"total_return_pct"`
	BuyHoldReturnPct float64   `json:"buy_hold_return_pct"`
	ExcessReturnPct  float64   `json:"excess_return_pct"`
	MaxDrawdownPct   float64   `json:"max_drawdown_pct"`
	TradeCount       int64     `json:"trade_count"`
	WinRatePct       float64   `json:"win_rate_pct"`
	CreatedAt        time.Time `json:"created_at"`
}

type CandleSnapshot struct {
	Name          string    `json:"name"`
	Exchange      string    `json:"exchange"`
	MarketType    string    `json:"market_type"`
	Symbol        string    `json:"symbol"`
	Interval      string    `json:"interval"`
	CandleCount   int64     `json:"candle_count"`
	ExpectedCount int64     `json:"expected_count"`
	MissingCount  int64     `json:"missing_count"`
	GapCount      int64     `json:"gap_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type OrderRecord struct {
	ID           int64     `json:"id"`
	AccountID    string    `json:"account_id"`
	StrategyID   string    `json:"strategy_id"`
	Exchange     string    `json:"exchange"`
	MarketType   string    `json:"market_type"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"`
	OrderType    string    `json:"order_type"`
	ReduceOnly   bool      `json:"reduce_only"`
	Price        float64   `json:"price"`
	Quantity     float64   `json:"quantity"`
	Status       string    `json:"status"`
	RiskDecision string    `json:"risk_decision"`
	RiskReason   string    `json:"risk_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type RiskEvent struct {
	ID            int64     `json:"id"`
	AccountID     string    `json:"account_id"`
	StrategyID    string    `json:"strategy_id"`
	ClientOrderID string    `json:"client_order_id"`
	EventTime     time.Time `json:"event_time"`
	Severity      string    `json:"severity"`
	EventType     string    `json:"event_type"`
	Symbol        string    `json:"symbol"`
	Decision      string    `json:"decision"`
	Message       string    `json:"message"`
}

type BalanceSnapshot struct {
	AccountID    string    `json:"account_id"`
	Exchange     string    `json:"exchange"`
	Asset        string    `json:"asset"`
	Free         float64   `json:"free"`
	Locked       float64   `json:"locked"`
	Total        float64   `json:"total"`
	USDValue     float64   `json:"usd_value"`
	SnapshotTime time.Time `json:"snapshot_time"`
}

type PositionSnapshot struct {
	AccountID              string    `json:"account_id"`
	Exchange               string    `json:"exchange"`
	MarketType             string    `json:"market_type"`
	Symbol                 string    `json:"symbol"`
	PositionSide           string    `json:"position_side"`
	Quantity               float64   `json:"quantity"`
	EntryPrice             float64   `json:"entry_price"`
	MarkPrice              float64   `json:"mark_price"`
	LiquidationPrice       float64   `json:"liquidation_price"`
	Leverage               float64   `json:"leverage"`
	MarginMode             string    `json:"margin_mode"`
	UnrealizedPnL          float64   `json:"unrealized_pnl"`
	Notional               float64   `json:"notional"`
	LiquidationDistancePct float64   `json:"liquidation_distance_pct"`
	SnapshotTime           time.Time `json:"snapshot_time"`
}

type MarginSnapshot struct {
	AccountID         string    `json:"account_id"`
	Exchange          string    `json:"exchange"`
	MarketType        string    `json:"market_type"`
	Equity            float64   `json:"equity"`
	MarginBalance     float64   `json:"margin_balance"`
	InitialMargin     float64   `json:"initial_margin"`
	MaintenanceMargin float64   `json:"maintenance_margin"`
	MarginRatio       float64   `json:"margin_ratio"`
	AvailableBalance  float64   `json:"available_balance"`
	SnapshotTime      time.Time `json:"snapshot_time"`
}

type StrategyRun struct {
	ID         int64      `json:"id"`
	StrategyID string     `json:"strategy_id"`
	Mode       string     `json:"mode"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type SignalRecord struct {
	ID         int64     `json:"id"`
	StrategyID string    `json:"strategy_id"`
	RunID      int64     `json:"run_id"`
	Exchange   string    `json:"exchange"`
	MarketType string    `json:"market_type"`
	Symbol     string    `json:"symbol"`
	Action     string    `json:"action"`
	Confidence float64   `json:"confidence"`
	Reason     string    `json:"reason"`
	SignalTime time.Time `json:"signal_time"`
}

type PerformanceSnapshot struct {
	StrategyID   string    `json:"strategy_id"`
	RunID        int64     `json:"run_id"`
	SnapshotTime time.Time `json:"snapshot_time"`
	Equity       float64   `json:"equity"`
	PnL          float64   `json:"pnl"`
	DrawdownPct  float64   `json:"drawdown_pct"`
	Exposure     float64   `json:"exposure"`
}

type FundingRate struct {
	Symbol      string    `json:"symbol"`
	FundingTime time.Time `json:"funding_time"`
	FundingRate float64   `json:"funding_rate"`
	MarkPrice   float64   `json:"mark_price"`
	IndexPrice  float64   `json:"index_price"`
}

type MarkPrice struct {
	Symbol          string     `json:"symbol"`
	EventTime       time.Time  `json:"event_time"`
	MarkPrice       float64    `json:"mark_price"`
	IndexPrice      float64    `json:"index_price"`
	NextFundingTime *time.Time `json:"next_funding_time,omitempty"`
}

func (s *Server) collect(ctx context.Context, query dashboardQuery) (Data, error) {
	data := Data{
		GeneratedAt: time.Now().UTC(),
		Query:       query,
		Runtime: RuntimeState{
			Halted:   fileExists(s.haltFile),
			HaltFile: s.haltFile,
		},
		Counts: make(map[string]int64),
	}

	loaders := []struct {
		name string
		run  func() error
	}{
		{"counts", func() error { return s.loadCounts(ctx, data.Counts) }},
		{"market coverage", func() error {
			records, err := s.loadMarketCoverage(ctx)
			data.MarketCoverage = records
			return err
		}},
		{"price series", func() error {
			records, err := s.loadPriceSeries(ctx, query)
			data.PriceSeries = records
			return err
		}},
		{"backtests", func() error {
			records, err := s.loadBacktests(ctx)
			data.Backtests = records
			return err
		}},
		{"candle snapshots", func() error {
			records, err := s.loadCandleSnapshots(ctx)
			data.CandleSnapshots = records
			return err
		}},
		{"orders", func() error {
			records, err := s.loadOrders(ctx)
			data.Orders = records
			return err
		}},
		{"risk events", func() error {
			records, err := s.loadRiskEvents(ctx)
			data.RiskEvents = records
			return err
		}},
		{"balances", func() error {
			records, err := s.loadBalances(ctx)
			data.Balances = records
			return err
		}},
		{"positions", func() error {
			records, err := s.loadPositions(ctx)
			data.Positions = records
			return err
		}},
		{"margins", func() error {
			records, err := s.loadMargins(ctx)
			data.Margins = records
			return err
		}},
		{"strategy runs", func() error {
			records, err := s.loadStrategyRuns(ctx)
			data.StrategyRuns = records
			return err
		}},
		{"signals", func() error {
			records, err := s.loadSignals(ctx)
			data.Signals = records
			return err
		}},
		{"performance snapshots", func() error {
			records, err := s.loadPerformanceSnapshots(ctx)
			data.PerformanceSnapshots = records
			return err
		}},
		{"funding rates", func() error {
			records, err := s.loadFundingRates(ctx)
			data.FundingRates = records
			return err
		}},
		{"mark prices", func() error {
			records, err := s.loadMarkPrices(ctx)
			data.MarkPrices = records
			return err
		}},
	}

	for _, loader := range loaders {
		if err := loader.run(); err != nil {
			if isMissingTable(err) {
				data.Warnings = append(data.Warnings, fmt.Sprintf("%s unavailable: %v", loader.name, err))
				continue
			}
			return Data{}, fmt.Errorf("%s: %w", loader.name, err)
		}
	}
	return data, nil
}

func (s *Server) loadCounts(ctx context.Context, counts map[string]int64) error {
	tables := []string{
		"candles", "funding_rates", "mark_prices", "index_prices", "candle_snapshots",
		"backtest_runs", "orders", "risk_events", "balances", "positions",
		"margin_snapshots", "strategy_runs", "signals", "performance_snapshots",
	}
	for _, table := range tables {
		value, err := s.countTable(ctx, table)
		if err != nil {
			return err
		}
		counts[table] = value
	}
	return nil
}

func (s *Server) countTable(ctx context.Context, table string) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Server) loadMarketCoverage(ctx context.Context) ([]MarketCoverage, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT exchange, market_type, symbol, interval, COUNT(*), MIN(open_time), MAX(open_time)
FROM candles
GROUP BY exchange, market_type, symbol, interval
ORDER BY market_type ASC, symbol ASC, interval ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]MarketCoverage, 0)
	for rows.Next() {
		var record MarketCoverage
		if err := rows.Scan(&record.Exchange, &record.MarketType, &record.Symbol, &record.Interval, &record.Candles, &record.FirstTime, &record.LastTime); err != nil {
			return nil, err
		}
		record.LastClose = s.latestClose(ctx, record.Exchange, record.MarketType, record.Symbol, record.Interval)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) latestClose(ctx context.Context, exchange string, marketType string, symbol string, interval string) float64 {
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT close_price
FROM candles
WHERE exchange = ? AND market_type = ? AND symbol = ? AND interval = ?
ORDER BY open_time DESC
LIMIT 1;
`, exchange, marketType, symbol, interval).Scan(&raw)
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseFloat(raw, 64)
	return value
}

func (s *Server) loadPriceSeries(ctx context.Context, query dashboardQuery) ([]CandlePoint, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT open_time, open_price, high_price, low_price, close_price, volume
FROM candles
WHERE exchange = ? AND market_type = ? AND symbol = ? AND interval = ?
ORDER BY open_time DESC
LIMIT ?;
`, query.Exchange, query.MarketType, query.Symbol, query.Interval, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]CandlePoint, 0, query.Limit)
	for rows.Next() {
		var record CandlePoint
		var openRaw, highRaw, lowRaw, closeRaw, volumeRaw string
		if err := rows.Scan(&record.Time, &openRaw, &highRaw, &lowRaw, &closeRaw, &volumeRaw); err != nil {
			return nil, err
		}
		record.Open = parseFloat(openRaw)
		record.High = parseFloat(highRaw)
		record.Low = parseFloat(lowRaw)
		record.Close = parseFloat(closeRaw)
		record.Volume = parseFloat(volumeRaw)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverse(records)
	return records, nil
}

func (s *Server) loadTablePreview(ctx context.Context, tableName string, limit int) (TablePreview, error) {
	columns, err := s.tableColumns(ctx, tableName)
	if err != nil {
		return TablePreview{}, err
	}
	sortColumn := tableSortColumn(tableName, columns)
	totalRows, err := s.countTable(ctx, tableName)
	if err != nil {
		return TablePreview{}, err
	}

	query := fmt.Sprintf("SELECT * FROM %s ORDER BY %s DESC LIMIT ?;", tableName, sortColumn)
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return TablePreview{}, err
	}
	defer rows.Close()

	values := make([]sql.NullString, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}

	records := make([]map[string]string, 0, limit)
	for rows.Next() {
		for i := range values {
			values[i] = sql.NullString{}
		}
		if err := rows.Scan(dest...); err != nil {
			return TablePreview{}, err
		}
		record := make(map[string]string, len(columns))
		for i, column := range columns {
			if values[i].Valid {
				record[column] = values[i].String
			} else {
				record[column] = ""
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return TablePreview{}, err
	}

	return TablePreview{
		Name:       tableName,
		Columns:    columns,
		Rows:       records,
		TotalRows:  totalRows,
		Limit:      limit,
		LoadedAt:   time.Now().UTC(),
		SortColumn: sortColumn,
	}, nil
}

func (s *Server) tableColumns(ctx context.Context, tableName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+tableName+");")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s has no columns", tableName)
	}
	return columns, nil
}

func (s *Server) loadBacktests(ctx context.Context) ([]BacktestRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, strategy_name, exchange, market_type, symbol, interval,
	fast_window, slow_window, fee_rate, total_return_pct, buy_hold_return_pct, excess_return_pct, max_drawdown_pct,
	trade_count, win_rate_pct, created_at
FROM backtest_runs
ORDER BY created_at DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]BacktestRun, 0)
	for rows.Next() {
		var record BacktestRun
		if err := rows.Scan(&record.ID, &record.StrategyName, &record.Exchange, &record.MarketType, &record.Symbol, &record.Interval, &record.FastWindow, &record.SlowWindow, &record.FeeRate, &record.TotalReturnPct, &record.BuyHoldReturnPct, &record.ExcessReturnPct, &record.MaxDrawdownPct, &record.TradeCount, &record.WinRatePct, &record.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadCandleSnapshots(ctx context.Context) ([]CandleSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name, exchange, market_type, symbol, interval, candle_count, expected_count,
	missing_count, gap_count, created_at
FROM candle_snapshots
ORDER BY created_at DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]CandleSnapshot, 0)
	for rows.Next() {
		var record CandleSnapshot
		if err := rows.Scan(&record.Name, &record.Exchange, &record.MarketType, &record.Symbol, &record.Interval, &record.CandleCount, &record.ExpectedCount, &record.MissingCount, &record.GapCount, &record.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadOrders(ctx context.Context) ([]OrderRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, account_id, strategy_id, exchange, market_type, symbol, side, order_type,
	reduce_only, price, quantity, status, risk_decision, risk_reason, created_at
FROM orders
ORDER BY created_at DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]OrderRecord, 0)
	for rows.Next() {
		var record OrderRecord
		var reduceOnly int
		if err := rows.Scan(&record.ID, &record.AccountID, &record.StrategyID, &record.Exchange, &record.MarketType, &record.Symbol, &record.Side, &record.OrderType, &reduceOnly, &record.Price, &record.Quantity, &record.Status, &record.RiskDecision, &record.RiskReason, &record.CreatedAt); err != nil {
			return nil, err
		}
		record.ReduceOnly = reduceOnly == 1
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadRiskEvents(ctx context.Context) ([]RiskEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, account_id, strategy_id, client_order_id, event_time, severity,
	event_type, symbol, decision, message
FROM risk_events
ORDER BY event_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]RiskEvent, 0)
	for rows.Next() {
		var record RiskEvent
		if err := rows.Scan(&record.ID, &record.AccountID, &record.StrategyID, &record.ClientOrderID, &record.EventTime, &record.Severity, &record.EventType, &record.Symbol, &record.Decision, &record.Message); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadBalances(ctx context.Context) ([]BalanceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT account_id, exchange, asset, free, locked, total, usd_value, snapshot_time
FROM balances
ORDER BY snapshot_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]BalanceSnapshot, 0)
	for rows.Next() {
		var record BalanceSnapshot
		if err := rows.Scan(&record.AccountID, &record.Exchange, &record.Asset, &record.Free, &record.Locked, &record.Total, &record.USDValue, &record.SnapshotTime); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadPositions(ctx context.Context) ([]PositionSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT account_id, exchange, market_type, symbol, position_side, quantity, entry_price,
	mark_price, liquidation_price, leverage, margin_mode, unrealized_pnl, notional,
	liquidation_distance_pct, snapshot_time
FROM positions
ORDER BY snapshot_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]PositionSnapshot, 0)
	for rows.Next() {
		var record PositionSnapshot
		if err := rows.Scan(&record.AccountID, &record.Exchange, &record.MarketType, &record.Symbol, &record.PositionSide, &record.Quantity, &record.EntryPrice, &record.MarkPrice, &record.LiquidationPrice, &record.Leverage, &record.MarginMode, &record.UnrealizedPnL, &record.Notional, &record.LiquidationDistancePct, &record.SnapshotTime); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadMargins(ctx context.Context) ([]MarginSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT account_id, exchange, market_type, equity, margin_balance, initial_margin,
	maintenance_margin, margin_ratio, available_balance, snapshot_time
FROM margin_snapshots
ORDER BY snapshot_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]MarginSnapshot, 0)
	for rows.Next() {
		var record MarginSnapshot
		if err := rows.Scan(&record.AccountID, &record.Exchange, &record.MarketType, &record.Equity, &record.MarginBalance, &record.InitialMargin, &record.MaintenanceMargin, &record.MarginRatio, &record.AvailableBalance, &record.SnapshotTime); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadStrategyRuns(ctx context.Context) ([]StrategyRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, strategy_id, mode, status, started_at, finished_at, created_at
FROM strategy_runs
ORDER BY created_at DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]StrategyRun, 0)
	for rows.Next() {
		var record StrategyRun
		var finished sql.NullTime
		if err := rows.Scan(&record.ID, &record.StrategyID, &record.Mode, &record.Status, &record.StartedAt, &finished, &record.CreatedAt); err != nil {
			return nil, err
		}
		if finished.Valid {
			record.FinishedAt = &finished.Time
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadSignals(ctx context.Context) ([]SignalRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, strategy_id, run_id, exchange, market_type, symbol, action, confidence,
	reason, signal_time
FROM signals
ORDER BY signal_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]SignalRecord, 0)
	for rows.Next() {
		var record SignalRecord
		if err := rows.Scan(&record.ID, &record.StrategyID, &record.RunID, &record.Exchange, &record.MarketType, &record.Symbol, &record.Action, &record.Confidence, &record.Reason, &record.SignalTime); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadPerformanceSnapshots(ctx context.Context) ([]PerformanceSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT strategy_id, run_id, snapshot_time, equity, pnl, drawdown_pct, exposure
FROM performance_snapshots
ORDER BY snapshot_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]PerformanceSnapshot, 0)
	for rows.Next() {
		var record PerformanceSnapshot
		if err := rows.Scan(&record.StrategyID, &record.RunID, &record.SnapshotTime, &record.Equity, &record.PnL, &record.DrawdownPct, &record.Exposure); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadFundingRates(ctx context.Context) ([]FundingRate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT symbol, funding_time, funding_rate, mark_price, index_price
FROM funding_rates
ORDER BY funding_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]FundingRate, 0)
	for rows.Next() {
		var record FundingRate
		var fundingRateRaw, markPriceRaw, indexPriceRaw string
		if err := rows.Scan(&record.Symbol, &record.FundingTime, &fundingRateRaw, &markPriceRaw, &indexPriceRaw); err != nil {
			return nil, err
		}
		record.FundingRate = parseFloat(fundingRateRaw)
		record.MarkPrice = parseFloat(markPriceRaw)
		record.IndexPrice = parseFloat(indexPriceRaw)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) loadMarkPrices(ctx context.Context) ([]MarkPrice, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT symbol, event_time, mark_price, index_price, next_funding_time
FROM mark_prices
ORDER BY event_time DESC
LIMIT 12;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]MarkPrice, 0)
	for rows.Next() {
		var record MarkPrice
		var markPriceRaw, indexPriceRaw string
		var nextFunding sql.NullTime
		if err := rows.Scan(&record.Symbol, &record.EventTime, &markPriceRaw, &indexPriceRaw, &nextFunding); err != nil {
			return nil, err
		}
		record.MarkPrice = parseFloat(markPriceRaw)
		record.IndexPrice = parseFloat(indexPriceRaw)
		if nextFunding.Valid {
			record.NextFundingTime = &nextFunding.Time
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func parseLimit(raw string, fallback int, maxValue int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func isAllowedTable(tableName string) bool {
	_, ok := allowedTables()[tableName]
	return ok
}

func allowedTables() map[string]struct{} {
	return map[string]struct{}{
		"account_modes":         {},
		"backtest_runs":         {},
		"balances":              {},
		"candle_snapshots":      {},
		"candles":               {},
		"contract_specs":        {},
		"funding_rates":         {},
		"index_prices":          {},
		"items":                 {},
		"leverage_brackets":     {},
		"margin_snapshots":      {},
		"mark_prices":           {},
		"order_books":           {},
		"orders":                {},
		"performance_snapshots": {},
		"positions":             {},
		"risk_events":           {},
		"signals":               {},
		"strategy_runs":         {},
		"trades":                {},
	}
}

func tableSortColumn(tableName string, columns []string) string {
	preferred := map[string]string{
		"backtest_runs":         "created_at",
		"balances":              "snapshot_time",
		"candle_snapshots":      "created_at",
		"candles":               "open_time",
		"funding_rates":         "funding_time",
		"index_prices":          "event_time",
		"margin_snapshots":      "snapshot_time",
		"mark_prices":           "event_time",
		"order_books":           "event_time",
		"orders":                "created_at",
		"performance_snapshots": "snapshot_time",
		"positions":             "snapshot_time",
		"risk_events":           "event_time",
		"signals":               "signal_time",
		"strategy_runs":         "created_at",
		"trades":                "trade_time",
	}
	if column, ok := preferred[tableName]; ok && hasColumn(columns, column) {
		return column
	}
	if hasColumn(columns, "id") {
		return "id"
	}
	return columns[0]
}

func hasColumn(columns []string, target string) bool {
	for _, column := range columns {
		if column == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseFloat(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func reverse(records []CandlePoint) {
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isMissingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" && !strings.HasPrefix(r.URL.Path, "/static/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
