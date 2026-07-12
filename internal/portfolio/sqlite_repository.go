package portfolio

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
	CREATE TABLE IF NOT EXISTS balances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		asset TEXT NOT NULL,
		free REAL NOT NULL,
		locked REAL NOT NULL,
		total REAL NOT NULL,
		usd_value REAL NOT NULL DEFAULT 0,
		snapshot_time DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(account_id, exchange, asset, snapshot_time)
	);

	CREATE INDEX IF NOT EXISTS idx_balances_lookup
	ON balances (account_id, exchange, asset, snapshot_time);

	CREATE TABLE IF NOT EXISTS positions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL,
		symbol TEXT NOT NULL,
		position_side TEXT NOT NULL,
		quantity REAL NOT NULL,
		entry_price REAL NOT NULL,
		mark_price REAL NOT NULL,
		liquidation_price REAL NOT NULL DEFAULT 0,
		leverage REAL NOT NULL DEFAULT 1,
		margin_mode TEXT NOT NULL DEFAULT '',
		unrealized_pnl REAL NOT NULL DEFAULT 0,
		notional REAL NOT NULL DEFAULT 0,
		liquidation_distance_pct REAL NOT NULL DEFAULT 0,
		snapshot_time DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(account_id, exchange, symbol, position_side, snapshot_time)
	);

	CREATE INDEX IF NOT EXISTS idx_positions_lookup
	ON positions (account_id, exchange, symbol, position_side, snapshot_time);

	CREATE TABLE IF NOT EXISTS margin_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL,
		equity REAL NOT NULL,
		margin_balance REAL NOT NULL,
		initial_margin REAL NOT NULL,
		maintenance_margin REAL NOT NULL,
		margin_ratio REAL NOT NULL,
		available_balance REAL NOT NULL,
		snapshot_time DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(account_id, exchange, market_type, snapshot_time)
	);

	CREATE INDEX IF NOT EXISTS idx_margin_snapshots_lookup
	ON margin_snapshots (account_id, exchange, market_type, snapshot_time);

	CREATE TABLE IF NOT EXISTS contract_specs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		contract_type TEXT NOT NULL,
		base_asset TEXT NOT NULL,
		quote_asset TEXT NOT NULL,
		tick_size TEXT NOT NULL,
		step_size TEXT NOT NULL,
		min_qty TEXT NOT NULL,
		min_notional TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(exchange, symbol, contract_type)
	);

	CREATE TABLE IF NOT EXISTS leverage_brackets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		bracket INTEGER NOT NULL,
		initial_leverage INTEGER NOT NULL,
		notional_cap REAL NOT NULL,
		notional_floor REAL NOT NULL,
		maintenance_margin_ratio REAL NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(exchange, symbol, bracket)
	);

	CREATE TABLE IF NOT EXISTS account_modes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		margin_mode TEXT NOT NULL,
		position_mode TEXT NOT NULL,
		max_leverage REAL NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(account_id, exchange)
	);
	`)
	return err
}

func (r *SQLiteRepository) SaveBalanceSnapshot(ctx context.Context, snapshot BalanceSnapshot) (int64, error) {
	snapshot.AccountID = strings.TrimSpace(snapshot.AccountID)
	snapshot.Exchange = strings.ToLower(strings.TrimSpace(snapshot.Exchange))
	snapshot.Asset = strings.ToUpper(strings.TrimSpace(snapshot.Asset))
	if snapshot.AccountID == "" || snapshot.Exchange == "" || snapshot.Asset == "" {
		return 0, errors.New("account id, exchange and asset are required")
	}
	if snapshot.Total == 0 {
		snapshot.Total = snapshot.Free + snapshot.Locked
	}
	now := time.Now().UTC()
	if snapshot.SnapshotTime.IsZero() {
		snapshot.SnapshotTime = now
	}
	snapshot.CreatedAt = now

	result, err := r.db.ExecContext(ctx, `
	INSERT INTO balances (
		account_id, exchange, asset, free, locked, total, usd_value, snapshot_time, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(account_id, exchange, asset, snapshot_time) DO UPDATE SET
		free = excluded.free,
		locked = excluded.locked,
		total = excluded.total,
		usd_value = excluded.usd_value;
	`, snapshot.AccountID, snapshot.Exchange, snapshot.Asset, snapshot.Free, snapshot.Locked, snapshot.Total, snapshot.USDValue, snapshot.SnapshotTime.UTC(), snapshot.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SavePositionSnapshot(ctx context.Context, snapshot PositionSnapshot) (int64, error) {
	snapshot.AccountID = strings.TrimSpace(snapshot.AccountID)
	snapshot.Exchange = strings.ToLower(strings.TrimSpace(snapshot.Exchange))
	snapshot.MarketType = strings.ToLower(strings.TrimSpace(snapshot.MarketType))
	snapshot.Symbol = strings.ToUpper(strings.TrimSpace(snapshot.Symbol))
	snapshot.PositionSide = strings.ToLower(strings.TrimSpace(snapshot.PositionSide))
	if snapshot.AccountID == "" || snapshot.Exchange == "" || snapshot.MarketType == "" || snapshot.Symbol == "" || snapshot.PositionSide == "" {
		return 0, errors.New("account id, exchange, market type, symbol and position side are required")
	}
	if snapshot.Notional == 0 {
		snapshot.Notional = abs(snapshot.Quantity * snapshot.MarkPrice)
	}
	if snapshot.LiquidationDistance == 0 && snapshot.MarkPrice > 0 && snapshot.LiquidationPrice > 0 {
		snapshot.LiquidationDistance = abs(snapshot.MarkPrice-snapshot.LiquidationPrice) / snapshot.MarkPrice * 100
	}
	now := time.Now().UTC()
	if snapshot.SnapshotTime.IsZero() {
		snapshot.SnapshotTime = now
	}
	snapshot.CreatedAt = now

	result, err := r.db.ExecContext(ctx, `
	INSERT INTO positions (
		account_id, exchange, market_type, symbol, position_side, quantity,
		entry_price, mark_price, liquidation_price, leverage, margin_mode,
		unrealized_pnl, notional, liquidation_distance_pct, snapshot_time, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(account_id, exchange, symbol, position_side, snapshot_time) DO UPDATE SET
		quantity = excluded.quantity,
		entry_price = excluded.entry_price,
		mark_price = excluded.mark_price,
		liquidation_price = excluded.liquidation_price,
		leverage = excluded.leverage,
		margin_mode = excluded.margin_mode,
		unrealized_pnl = excluded.unrealized_pnl,
		notional = excluded.notional,
		liquidation_distance_pct = excluded.liquidation_distance_pct;
	`, snapshot.AccountID, snapshot.Exchange, snapshot.MarketType, snapshot.Symbol, snapshot.PositionSide, snapshot.Quantity, snapshot.EntryPrice, snapshot.MarkPrice, snapshot.LiquidationPrice, snapshot.Leverage, snapshot.MarginMode, snapshot.UnrealizedPnL, snapshot.Notional, snapshot.LiquidationDistance, snapshot.SnapshotTime.UTC(), snapshot.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SaveMarginSnapshot(ctx context.Context, snapshot MarginSnapshot) (int64, error) {
	snapshot.AccountID = strings.TrimSpace(snapshot.AccountID)
	snapshot.Exchange = strings.ToLower(strings.TrimSpace(snapshot.Exchange))
	snapshot.MarketType = strings.ToLower(strings.TrimSpace(snapshot.MarketType))
	if snapshot.AccountID == "" || snapshot.Exchange == "" || snapshot.MarketType == "" {
		return 0, errors.New("account id, exchange and market type are required")
	}
	now := time.Now().UTC()
	if snapshot.SnapshotTime.IsZero() {
		snapshot.SnapshotTime = now
	}
	snapshot.CreatedAt = now
	result, err := r.db.ExecContext(ctx, `
	INSERT INTO margin_snapshots (
		account_id, exchange, market_type, equity, margin_balance, initial_margin,
		maintenance_margin, margin_ratio, available_balance, snapshot_time, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(account_id, exchange, market_type, snapshot_time) DO UPDATE SET
		equity = excluded.equity,
		margin_balance = excluded.margin_balance,
		initial_margin = excluded.initial_margin,
		maintenance_margin = excluded.maintenance_margin,
		margin_ratio = excluded.margin_ratio,
		available_balance = excluded.available_balance;
	`, snapshot.AccountID, snapshot.Exchange, snapshot.MarketType, snapshot.Equity, snapshot.MarginBalance, snapshot.InitialMargin, snapshot.MaintenanceMargin, snapshot.MarginRatio, snapshot.AvailableBalance, snapshot.SnapshotTime.UTC(), snapshot.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
