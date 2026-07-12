package strategy

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func InitSQLiteSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS strategy_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		strategy_id TEXT NOT NULL,
		mode TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME,
		config_json TEXT NOT NULL DEFAULT '{}',
		summary_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_strategy_runs_lookup
	ON strategy_runs (strategy_id, mode, status, started_at);

	CREATE TABLE IF NOT EXISTS signals (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		strategy_id TEXT NOT NULL,
		run_id INTEGER NOT NULL DEFAULT 0,
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL,
		symbol TEXT NOT NULL,
		signal_time DATETIME NOT NULL,
		action TEXT NOT NULL,
		confidence REAL NOT NULL DEFAULT 0,
		reason TEXT NOT NULL DEFAULT '',
		raw_features_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_signals_lookup
	ON signals (strategy_id, exchange, market_type, symbol, signal_time);

	CREATE TABLE IF NOT EXISTS performance_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		strategy_id TEXT NOT NULL,
		run_id INTEGER NOT NULL DEFAULT 0,
		snapshot_time DATETIME NOT NULL,
		equity REAL NOT NULL,
		pnl REAL NOT NULL,
		drawdown_pct REAL NOT NULL,
		exposure REAL NOT NULL,
		metrics_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_performance_snapshots_lookup
	ON performance_snapshots (strategy_id, run_id, snapshot_time);
	`)
	return err
}

func (r *SQLiteRepository) StartRun(ctx context.Context, record RunRecord) (int64, error) {
	record.StrategyID = strings.TrimSpace(record.StrategyID)
	record.Mode = strings.TrimSpace(record.Mode)
	if record.StrategyID == "" || record.Mode == "" {
		return 0, errors.New("strategy id and mode are required")
	}
	if record.Status == "" {
		record.Status = RunStatusStarted
	}
	if record.ConfigJSON == "" {
		record.ConfigJSON = "{}"
	}
	if record.SummaryJSON == "" {
		record.SummaryJSON = "{}"
	}
	now := time.Now().UTC()
	if record.StartedAt.IsZero() {
		record.StartedAt = now
	}
	record.CreatedAt = now
	result, err := r.db.ExecContext(ctx, `
	INSERT INTO strategy_runs (
		strategy_id, mode, status, started_at, finished_at, config_json, summary_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`, record.StrategyID, record.Mode, string(record.Status), record.StartedAt.UTC(), nullableTime(record.FinishedAt), record.ConfigJSON, record.SummaryJSON, record.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SaveSignal(ctx context.Context, record SignalRecord) (int64, error) {
	record.StrategyID = strings.TrimSpace(record.StrategyID)
	record.Exchange = strings.ToLower(strings.TrimSpace(record.Exchange))
	record.MarketType = strings.ToLower(strings.TrimSpace(record.MarketType))
	record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
	if record.StrategyID == "" || record.Exchange == "" || record.MarketType == "" || record.Symbol == "" {
		return 0, errors.New("strategy id, exchange, market type and symbol are required")
	}
	if record.Action == "" {
		record.Action = SignalHold
	}
	if record.RawFeaturesJSON == "" {
		record.RawFeaturesJSON = "{}"
	}
	now := time.Now().UTC()
	if record.SignalTime.IsZero() {
		record.SignalTime = now
	}
	record.CreatedAt = now
	result, err := r.db.ExecContext(ctx, `
	INSERT INTO signals (
		strategy_id, run_id, exchange, market_type, symbol, signal_time,
		action, confidence, reason, raw_features_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, record.StrategyID, record.RunID, record.Exchange, record.MarketType, record.Symbol, record.SignalTime.UTC(), string(record.Action), record.Confidence, record.Reason, record.RawFeaturesJSON, record.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SavePerformanceSnapshot(ctx context.Context, record PerformanceSnapshot) (int64, error) {
	record.StrategyID = strings.TrimSpace(record.StrategyID)
	if record.StrategyID == "" {
		return 0, errors.New("strategy id is required")
	}
	if record.MetricsJSON == "" {
		record.MetricsJSON = "{}"
	}
	now := time.Now().UTC()
	if record.SnapshotTime.IsZero() {
		record.SnapshotTime = now
	}
	record.CreatedAt = now
	result, err := r.db.ExecContext(ctx, `
	INSERT INTO performance_snapshots (
		strategy_id, run_id, snapshot_time, equity, pnl, drawdown_pct, exposure, metrics_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, record.StrategyID, record.RunID, record.SnapshotTime.UTC(), record.Equity, record.PnL, record.DrawdownPct, record.Exposure, record.MetricsJSON, record.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
