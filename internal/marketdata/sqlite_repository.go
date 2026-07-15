package marketdata

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gogogo/internal/sqliteutil"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func OpenSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	if err := sqliteutil.Configure(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	if err := InitSQLiteSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func InitSQLiteSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS candles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	exchange TEXT NOT NULL,
	market_type TEXT NOT NULL CHECK (market_type IN ('spot', 'perpetual')),
	symbol TEXT NOT NULL,
	interval TEXT NOT NULL,
	open_time DATETIME NOT NULL,
	close_time DATETIME NOT NULL,
	open_price TEXT NOT NULL,
	high_price TEXT NOT NULL,
	low_price TEXT NOT NULL,
	close_price TEXT NOT NULL,
	volume TEXT NOT NULL,
	quote_volume TEXT NOT NULL,
	trade_count INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	UNIQUE(exchange, market_type, symbol, interval, open_time)
);

	CREATE INDEX IF NOT EXISTS idx_candles_lookup
	ON candles (exchange, market_type, symbol, interval, open_time);

		CREATE TABLE IF NOT EXISTS trades (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			exchange TEXT NOT NULL,
			market_type TEXT NOT NULL CHECK (market_type IN ('spot', 'perpetual')),
			symbol TEXT NOT NULL,
			trade_id TEXT NOT NULL,
			price TEXT NOT NULL,
			quantity TEXT NOT NULL,
			quote_quantity TEXT NOT NULL DEFAULT '',
			side TEXT NOT NULL DEFAULT '',
			trade_time DATETIME NOT NULL,
			raw_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			UNIQUE(exchange, market_type, symbol, trade_id)
		);

	CREATE INDEX IF NOT EXISTS idx_trades_lookup
	ON trades (exchange, market_type, symbol, trade_time);

	CREATE TABLE IF NOT EXISTS order_books (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL,
			market_type TEXT NOT NULL CHECK (market_type IN ('spot', 'perpetual')),
			symbol TEXT NOT NULL,
			event_time DATETIME NOT NULL,
			update_id INTEGER NOT NULL DEFAULT 0,
			bids_json TEXT NOT NULL,
			asks_json TEXT NOT NULL,
			raw_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			UNIQUE(exchange, market_type, symbol, event_time)
		);

	CREATE INDEX IF NOT EXISTS idx_order_books_lookup
	ON order_books (exchange, market_type, symbol, event_time);

CREATE TABLE IF NOT EXISTS funding_rates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	exchange TEXT NOT NULL,
	symbol TEXT NOT NULL,
		funding_time DATETIME NOT NULL,
		funding_rate TEXT NOT NULL,
		mark_price TEXT NOT NULL,
		index_price TEXT NOT NULL DEFAULT '',
		raw_json TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(exchange, symbol, funding_time)
	);

CREATE INDEX IF NOT EXISTS idx_funding_rates_lookup
ON funding_rates (exchange, symbol, funding_time);

CREATE TABLE IF NOT EXISTS mark_prices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	exchange TEXT NOT NULL,
	symbol TEXT NOT NULL,
	event_time DATETIME NOT NULL,
	mark_price TEXT NOT NULL,
		index_price TEXT NOT NULL,
		estimated_settle_price TEXT NOT NULL DEFAULT '',
		next_funding_time DATETIME,
		raw_json TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(exchange, symbol, event_time)
	);

	CREATE INDEX IF NOT EXISTS idx_mark_prices_lookup
	ON mark_prices (exchange, symbol, event_time);

	CREATE TABLE IF NOT EXISTS index_prices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
			event_time DATETIME NOT NULL,
			index_price TEXT NOT NULL,
			raw_json TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			UNIQUE(exchange, symbol, event_time)
		);

	CREATE INDEX IF NOT EXISTS idx_index_prices_lookup
	ON index_prices (exchange, symbol, event_time);

	CREATE TABLE IF NOT EXISTS candle_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL CHECK (market_type IN ('spot', 'perpetual')),
		symbol TEXT NOT NULL,
		interval TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		candle_count INTEGER NOT NULL,
		expected_count INTEGER NOT NULL,
		missing_count INTEGER NOT NULL,
		gap_count INTEGER NOT NULL,
		data_hash TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_candle_snapshots_lookup
	ON candle_snapshots (name, exchange, market_type, symbol, interval, created_at);

	CREATE TABLE IF NOT EXISTS backtest_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		strategy_name TEXT NOT NULL,
	exchange TEXT NOT NULL,
	market_type TEXT NOT NULL,
	symbol TEXT NOT NULL,
	interval TEXT NOT NULL,
	start_time DATETIME NOT NULL,
	end_time DATETIME NOT NULL,
	fast_window INTEGER NOT NULL,
	slow_window INTEGER NOT NULL,
	fee_rate REAL NOT NULL,
	initial_equity REAL NOT NULL,
	final_equity REAL NOT NULL,
	total_return_pct REAL NOT NULL,
	buy_hold_return_pct REAL NOT NULL DEFAULT 0,
	excess_return_pct REAL NOT NULL DEFAULT 0,
	max_drawdown_pct REAL NOT NULL,
	trade_count INTEGER NOT NULL,
	win_rate_pct REAL NOT NULL,
	created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_backtest_runs_lookup
ON backtest_runs (strategy_name, exchange, market_type, symbol, interval, created_at);
`)
	if err != nil {
		return err
	}
	columns := []struct {
		table      string
		column     string
		definition string
	}{
		{"trades", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"order_books", "update_id", "INTEGER NOT NULL DEFAULT 0"},
		{"order_books", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"funding_rates", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"mark_prices", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"index_prices", "raw_json", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(ctx, db, column.table, column.column, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, table string, column string, definition string) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`);`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition+`;`)
	return err
}

func (r *SQLiteRepository) UpsertCandle(ctx context.Context, candle Candle) error {
	candle, err := normalizeCandle(candle)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if candle.CreatedAt.IsZero() {
		candle.CreatedAt = now
	}
	candle.UpdatedAt = now

	_, err = r.db.ExecContext(ctx, `
INSERT INTO candles (
	exchange, market_type, symbol, interval, open_time, close_time,
	open_price, high_price, low_price, close_price, volume, quote_volume,
	trade_count, source, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(exchange, market_type, symbol, interval, open_time)
DO UPDATE SET
	close_time = excluded.close_time,
	open_price = excluded.open_price,
	high_price = excluded.high_price,
	low_price = excluded.low_price,
	close_price = excluded.close_price,
	volume = excluded.volume,
	quote_volume = excluded.quote_volume,
	trade_count = excluded.trade_count,
	source = excluded.source,
	updated_at = excluded.updated_at;
`,
		candle.Exchange,
		string(candle.MarketType),
		candle.Symbol,
		candle.Interval,
		candle.OpenTime,
		candle.CloseTime,
		candle.Open,
		candle.High,
		candle.Low,
		candle.Close,
		candle.Volume,
		candle.QuoteVolume,
		candle.TradeCount,
		candle.Source,
		candle.CreatedAt,
		candle.UpdatedAt,
	)
	return err
}

func (r *SQLiteRepository) LatestCandle(ctx context.Context, exchange string, marketType MarketType, symbol string, interval string) (Candle, error) {
	exchange = normalizeExchange(exchange)
	marketType = normalizeMarketType(marketType)
	symbol = normalizeSymbol(symbol)
	interval = strings.TrimSpace(interval)
	if err := validateMarket(exchange, marketType, symbol); err != nil {
		return Candle{}, err
	}
	if interval == "" {
		return Candle{}, errors.New("interval is required")
	}

	row := r.db.QueryRowContext(ctx, `
SELECT exchange, market_type, symbol, interval, open_time, close_time,
	open_price, high_price, low_price, close_price, volume, quote_volume,
	trade_count, source, created_at, updated_at
FROM candles
WHERE exchange = ?
	AND market_type = ?
	AND symbol = ?
	AND interval = ?
ORDER BY open_time DESC
LIMIT 1;
`,
		exchange,
		string(marketType),
		symbol,
		interval,
	)
	candle, err := scanCandle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Candle{}, ErrNotFound
		}
		return Candle{}, err
	}
	return candle, nil
}

func (r *SQLiteRepository) UpsertTrade(ctx context.Context, trade Trade) error {
	trade, err := normalizeTrade(trade)
	if err != nil {
		return err
	}
	if trade.CreatedAt.IsZero() {
		trade.CreatedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
	INSERT INTO trades (
		exchange, market_type, symbol, trade_id, price, quantity, quote_quantity,
		side, trade_time, raw_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, market_type, symbol, trade_id)
	DO UPDATE SET
		price = excluded.price,
		quantity = excluded.quantity,
		quote_quantity = excluded.quote_quantity,
		side = excluded.side,
		trade_time = excluded.trade_time,
		raw_json = excluded.raw_json;
	`,
		trade.Exchange,
		string(trade.MarketType),
		trade.Symbol,
		trade.TradeID,
		trade.Price,
		trade.Quantity,
		trade.QuoteQuantity,
		trade.Side,
		trade.TradeTime,
		trade.RawJSON,
		trade.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) UpsertOrderBook(ctx context.Context, book OrderBook) error {
	book, err := normalizeOrderBook(book)
	if err != nil {
		return err
	}
	if book.CreatedAt.IsZero() {
		book.CreatedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
	INSERT INTO order_books (
		exchange, market_type, symbol, event_time, update_id, bids_json,
		asks_json, raw_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, market_type, symbol, event_time)
	DO UPDATE SET
		update_id = excluded.update_id,
		bids_json = excluded.bids_json,
		asks_json = excluded.asks_json,
		raw_json = excluded.raw_json;
	`,
		book.Exchange,
		string(book.MarketType),
		book.Symbol,
		book.EventTime,
		book.UpdateID,
		book.BidsJSON,
		book.AsksJSON,
		book.RawJSON,
		book.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) ListCandles(ctx context.Context, query CandleQuery) ([]Candle, error) {
	query, err := normalizeCandleQuery(query)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT exchange, market_type, symbol, interval, open_time, close_time,
	open_price, high_price, low_price, close_price, volume, quote_volume,
	trade_count, source, created_at, updated_at
FROM candles
WHERE exchange = ?
	AND market_type = ?
	AND symbol = ?
	AND interval = ?
	AND open_time >= ?
	AND open_time < ?
ORDER BY open_time ASC
LIMIT ?;
`,
		query.Exchange,
		string(query.MarketType),
		query.Symbol,
		query.Interval,
		query.Start,
		query.End,
		query.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candles := make([]Candle, 0)
	for rows.Next() {
		candle, err := scanCandle(rows)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return candles, nil
}

func (r *SQLiteRepository) ListCandlesFull(ctx context.Context, query CandleQuery) ([]Candle, error) {
	query, err := normalizeCandleQuery(query)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
	SELECT exchange, market_type, symbol, interval, open_time, close_time,
		open_price, high_price, low_price, close_price, volume, quote_volume,
		trade_count, source, created_at, updated_at
	FROM candles
	WHERE exchange = ?
		AND market_type = ?
		AND symbol = ?
		AND interval = ?
		AND open_time >= ?
		AND open_time < ?
	ORDER BY open_time ASC;
	`,
		query.Exchange,
		string(query.MarketType),
		query.Symbol,
		query.Interval,
		query.Start,
		query.End,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candles := make([]Candle, 0)
	for rows.Next() {
		candle, err := scanCandle(rows)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return candles, nil
}

func (r *SQLiteRepository) CreateCandleSnapshot(ctx context.Context, request CandleSnapshotRequest) (CandleSnapshot, CandleCoverage, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return CandleSnapshot{}, CandleCoverage{}, errors.New("snapshot name is required")
	}

	query, err := normalizeCandleQuery(request.Query)
	if err != nil {
		return CandleSnapshot{}, CandleCoverage{}, err
	}
	candles, err := r.ListCandlesFull(ctx, query)
	if err != nil {
		return CandleSnapshot{}, CandleCoverage{}, err
	}
	coverage, err := CheckCandleCoverage(candles, query)
	if err != nil {
		return CandleSnapshot{}, CandleCoverage{}, err
	}
	if request.RequireComplete && !coverage.Complete() {
		return CandleSnapshot{}, coverage, errors.New("candle coverage is incomplete")
	}

	now := time.Now().UTC()
	snapshot := CandleSnapshot{
		Name:          name,
		Exchange:      coverage.Exchange,
		MarketType:    coverage.MarketType,
		Symbol:        coverage.Symbol,
		Interval:      coverage.Interval,
		Start:         coverage.Start,
		End:           coverage.End,
		CandleCount:   coverage.CandleCount,
		ExpectedCount: coverage.ExpectedCount,
		MissingCount:  coverage.MissingCount,
		GapCount:      len(coverage.Gaps),
		DataHash:      CandleDataHash(candles),
		CreatedAt:     now,
	}

	inserted, err := r.db.ExecContext(ctx, `
	INSERT INTO candle_snapshots (
		name, exchange, market_type, symbol, interval, start_time, end_time,
		candle_count, expected_count, missing_count, gap_count, data_hash, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`,
		snapshot.Name,
		snapshot.Exchange,
		string(snapshot.MarketType),
		snapshot.Symbol,
		snapshot.Interval,
		snapshot.Start,
		snapshot.End,
		snapshot.CandleCount,
		snapshot.ExpectedCount,
		snapshot.MissingCount,
		snapshot.GapCount,
		snapshot.DataHash,
		snapshot.CreatedAt,
	)
	if err != nil {
		return CandleSnapshot{}, CandleCoverage{}, err
	}
	snapshot.ID, err = inserted.LastInsertId()
	if err != nil {
		return CandleSnapshot{}, CandleCoverage{}, err
	}

	return snapshot, coverage, nil
}

func (r *SQLiteRepository) UpsertFundingRate(ctx context.Context, rate FundingRate) error {
	rate, err := normalizeFundingRate(rate)
	if err != nil {
		return err
	}

	if rate.CreatedAt.IsZero() {
		rate.CreatedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
INSERT INTO funding_rates (
		exchange, symbol, funding_time, funding_rate, mark_price, index_price, raw_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, symbol, funding_time)
	DO UPDATE SET
		funding_rate = excluded.funding_rate,
		mark_price = excluded.mark_price,
		index_price = excluded.index_price,
		raw_json = excluded.raw_json;
	`,
		rate.Exchange,
		rate.Symbol,
		rate.FundingTime,
		rate.FundingRate,
		rate.MarkPrice,
		rate.IndexPrice,
		rate.RawJSON,
		rate.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) ListFundingRates(ctx context.Context, query FundingRateQuery) ([]FundingRate, error) {
	query, err := normalizeFundingRateQuery(query)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT exchange, symbol, funding_time, funding_rate, mark_price, index_price, created_at
FROM funding_rates
WHERE exchange = ?
	AND symbol = ?
	AND funding_time >= ?
	AND funding_time < ?
ORDER BY funding_time ASC
LIMIT ?;
`,
		query.Exchange,
		query.Symbol,
		query.Start,
		query.End,
		query.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rates := make([]FundingRate, 0)
	for rows.Next() {
		rate, err := scanFundingRate(rows)
		if err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rates, nil
}

func (r *SQLiteRepository) LatestFundingRate(ctx context.Context, exchange string, symbol string) (FundingRate, error) {
	exchange = normalizeExchange(exchange)
	symbol = normalizeSymbol(symbol)
	if exchange == "" {
		return FundingRate{}, errors.New("exchange is required")
	}
	if symbol == "" {
		return FundingRate{}, errors.New("symbol is required")
	}

	row := r.db.QueryRowContext(ctx, `
SELECT exchange, symbol, funding_time, funding_rate, mark_price, index_price, created_at
FROM funding_rates
WHERE exchange = ? AND symbol = ?
ORDER BY funding_time DESC
LIMIT 1;
`, exchange, symbol)

	rate, err := scanFundingRate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FundingRate{}, ErrNotFound
		}
		return FundingRate{}, err
	}
	return rate, nil
}

func (r *SQLiteRepository) UpsertMarkPrice(ctx context.Context, price MarkPrice) error {
	price, err := normalizeMarkPrice(price)
	if err != nil {
		return err
	}

	if price.CreatedAt.IsZero() {
		price.CreatedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
INSERT INTO mark_prices (
	exchange, symbol, event_time, mark_price, index_price,
		estimated_settle_price, next_funding_time, raw_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, symbol, event_time)
	DO UPDATE SET
		mark_price = excluded.mark_price,
		index_price = excluded.index_price,
		estimated_settle_price = excluded.estimated_settle_price,
		next_funding_time = excluded.next_funding_time,
		raw_json = excluded.raw_json;
	`,
		price.Exchange,
		price.Symbol,
		price.EventTime,
		price.MarkPrice,
		price.IndexPrice,
		price.EstimatedSettlePrice,
		nullableTime(price.NextFundingTime),
		price.RawJSON,
		price.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) DeleteMarkPricesBefore(ctx context.Context, exchange string, symbol string, cutoff time.Time) (int64, error) {
	exchange = normalizeExchange(exchange)
	symbol = normalizeSymbol(symbol)
	cutoff = cutoff.UTC()
	if exchange == "" {
		return 0, errors.New("exchange is required")
	}
	if symbol == "" {
		return 0, errors.New("symbol is required")
	}
	if cutoff.IsZero() {
		return 0, errors.New("cutoff is required")
	}

	result, err := r.db.ExecContext(ctx, `
DELETE FROM mark_prices
WHERE exchange = ? AND symbol = ? AND event_time < ?;
`, exchange, symbol, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SQLiteRepository) UpsertIndexPrice(ctx context.Context, price IndexPrice) error {
	price, err := normalizeIndexPrice(price)
	if err != nil {
		return err
	}
	if price.CreatedAt.IsZero() {
		price.CreatedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx, `
	INSERT INTO index_prices (
		exchange, symbol, event_time, index_price, raw_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, symbol, event_time)
	DO UPDATE SET
		index_price = excluded.index_price,
		raw_json = excluded.raw_json;
	`,
		price.Exchange,
		price.Symbol,
		price.EventTime,
		price.IndexPrice,
		price.RawJSON,
		price.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) LatestMarkPrice(ctx context.Context, exchange string, symbol string) (MarkPrice, error) {
	exchange = normalizeExchange(exchange)
	symbol = normalizeSymbol(symbol)
	if exchange == "" {
		return MarkPrice{}, errors.New("exchange is required")
	}
	if symbol == "" {
		return MarkPrice{}, errors.New("symbol is required")
	}

	row := r.db.QueryRowContext(ctx, `
SELECT exchange, symbol, event_time, mark_price, index_price,
	estimated_settle_price, next_funding_time, created_at
FROM mark_prices
WHERE exchange = ? AND symbol = ?
ORDER BY event_time DESC
LIMIT 1;
`, exchange, symbol)

	price, err := scanMarkPrice(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MarkPrice{}, ErrNotFound
		}
		return MarkPrice{}, err
	}
	return price, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCandle(scanner scanner) (Candle, error) {
	var candle Candle
	var marketType string
	if err := scanner.Scan(
		&candle.Exchange,
		&marketType,
		&candle.Symbol,
		&candle.Interval,
		&candle.OpenTime,
		&candle.CloseTime,
		&candle.Open,
		&candle.High,
		&candle.Low,
		&candle.Close,
		&candle.Volume,
		&candle.QuoteVolume,
		&candle.TradeCount,
		&candle.Source,
		&candle.CreatedAt,
		&candle.UpdatedAt,
	); err != nil {
		return Candle{}, err
	}
	candle.MarketType = MarketType(marketType)
	return candle, nil
}

func scanFundingRate(scanner scanner) (FundingRate, error) {
	var rate FundingRate
	if err := scanner.Scan(
		&rate.Exchange,
		&rate.Symbol,
		&rate.FundingTime,
		&rate.FundingRate,
		&rate.MarkPrice,
		&rate.IndexPrice,
		&rate.CreatedAt,
	); err != nil {
		return FundingRate{}, err
	}
	return rate, nil
}

func scanMarkPrice(scanner scanner) (MarkPrice, error) {
	var price MarkPrice
	var estimatedSettlePrice sql.NullString
	var nextFundingTime sql.NullTime
	if err := scanner.Scan(
		&price.Exchange,
		&price.Symbol,
		&price.EventTime,
		&price.MarkPrice,
		&price.IndexPrice,
		&estimatedSettlePrice,
		&nextFundingTime,
		&price.CreatedAt,
	); err != nil {
		return MarkPrice{}, err
	}
	if estimatedSettlePrice.Valid {
		price.EstimatedSettlePrice = estimatedSettlePrice.String
	}
	if nextFundingTime.Valid {
		price.NextFundingTime = nextFundingTime.Time
	}
	return price, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
