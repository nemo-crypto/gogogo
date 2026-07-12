package marketdata

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

CREATE TABLE IF NOT EXISTS funding_rates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	exchange TEXT NOT NULL,
	symbol TEXT NOT NULL,
	funding_time DATETIME NOT NULL,
	funding_rate TEXT NOT NULL,
	mark_price TEXT NOT NULL,
	index_price TEXT NOT NULL DEFAULT '',
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
	created_at DATETIME NOT NULL,
	UNIQUE(exchange, symbol, event_time)
);

CREATE INDEX IF NOT EXISTS idx_mark_prices_lookup
ON mark_prices (exchange, symbol, event_time);

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
	exchange, symbol, funding_time, funding_rate, mark_price, index_price, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(exchange, symbol, funding_time)
DO UPDATE SET
	funding_rate = excluded.funding_rate,
	mark_price = excluded.mark_price,
	index_price = excluded.index_price;
`,
		rate.Exchange,
		rate.Symbol,
		rate.FundingTime,
		rate.FundingRate,
		rate.MarkPrice,
		rate.IndexPrice,
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
	estimated_settle_price, next_funding_time, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(exchange, symbol, event_time)
DO UPDATE SET
	mark_price = excluded.mark_price,
	index_price = excluded.index_price,
	estimated_settle_price = excluded.estimated_settle_price,
	next_funding_time = excluded.next_funding_time;
`,
		price.Exchange,
		price.Symbol,
		price.EventTime,
		price.MarkPrice,
		price.IndexPrice,
		price.EstimatedSettlePrice,
		nullableTime(price.NextFundingTime),
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
