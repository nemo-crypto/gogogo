package backtest

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func InitSQLiteSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
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
	if err := addColumnIfMissing(ctx, db, "backtest_runs", "buy_hold_return_pct", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return addColumnIfMissing(ctx, db, "backtest_runs", "excess_return_pct", "REAL NOT NULL DEFAULT 0")
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

type SaveRunRequest struct {
	Exchange   string
	MarketType string
	Config     SMAConfig
	Result     Result
}

type RunRecord struct {
	ID               int64
	StrategyName     string
	Exchange         string
	MarketType       string
	Symbol           string
	Interval         string
	Start            time.Time
	End              time.Time
	FastWindow       int
	SlowWindow       int
	FeeRate          float64
	InitialEquity    float64
	FinalEquity      float64
	TotalReturnPct   float64
	BuyHoldReturnPct float64
	ExcessReturnPct  float64
	MaxDrawdownPct   float64
	TradeCount       int
	WinRatePct       float64
	CreatedAt        time.Time
}

type ListRunsQuery struct {
	Symbol   string
	Interval string
	SortBy   string
	Limit    int
}

func (r *SQLiteRepository) SaveRun(ctx context.Context, request SaveRunRequest) (int64, error) {
	result := request.Result
	inserted, err := r.db.ExecContext(ctx, `
INSERT INTO backtest_runs (
	strategy_name, exchange, market_type, symbol, interval, start_time, end_time,
	fast_window, slow_window, fee_rate, initial_equity, final_equity,
	total_return_pct, buy_hold_return_pct, excess_return_pct, max_drawdown_pct,
	trade_count, win_rate_pct, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`,
		result.StrategyName,
		request.Exchange,
		request.MarketType,
		result.Symbol,
		result.Interval,
		result.Start,
		result.End,
		request.Config.FastWindow,
		request.Config.SlowWindow,
		request.Config.FeeRate,
		result.InitialEquity,
		result.FinalEquity,
		result.TotalReturnPct,
		result.BuyHoldReturnPct,
		result.ExcessReturnPct,
		result.MaxDrawdownPct,
		len(result.Trades),
		result.WinRatePct,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return inserted.LastInsertId()
}

func (r *SQLiteRepository) ListRuns(ctx context.Context, query ListRunsQuery) ([]RunRecord, error) {
	sortBy := normalizeSortBy(query.SortBy)
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	where := ""
	args := make([]any, 0, 3)
	if query.Symbol != "" {
		where = "WHERE symbol = ?"
		args = append(args, query.Symbol)
	}
	if query.Interval != "" {
		if where == "" {
			where = "WHERE interval = ?"
		} else {
			where += " AND interval = ?"
		}
		args = append(args, query.Interval)
	}
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, strategy_name, exchange, market_type, symbol, interval, start_time, end_time,
	fast_window, slow_window, fee_rate, initial_equity, final_equity,
	total_return_pct, buy_hold_return_pct, excess_return_pct, max_drawdown_pct,
	trade_count, win_rate_pct, created_at
FROM backtest_runs
%s
ORDER BY %s DESC
LIMIT ?;
`, where, sortBy), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]RunRecord, 0)
	for rows.Next() {
		var record RunRecord
		if err := rows.Scan(
			&record.ID,
			&record.StrategyName,
			&record.Exchange,
			&record.MarketType,
			&record.Symbol,
			&record.Interval,
			&record.Start,
			&record.End,
			&record.FastWindow,
			&record.SlowWindow,
			&record.FeeRate,
			&record.InitialEquity,
			&record.FinalEquity,
			&record.TotalReturnPct,
			&record.BuyHoldReturnPct,
			&record.ExcessReturnPct,
			&record.MaxDrawdownPct,
			&record.TradeCount,
			&record.WinRatePct,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func normalizeSortBy(sortBy string) string {
	switch sortBy {
	case "total_return_pct", "total":
		return "total_return_pct"
	case "excess_return_pct", "excess":
		return "excess_return_pct"
	case "win_rate_pct", "win-rate":
		return "win_rate_pct"
	case "trade_count", "trades":
		return "trade_count"
	case "max_drawdown_pct", "drawdown":
		return "max_drawdown_pct"
	default:
		return "excess_return_pct"
	}
}
