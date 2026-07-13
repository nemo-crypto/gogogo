package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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
		UNIQUE(account_id, exchange, market_type, symbol, position_side, snapshot_time)
	);

	CREATE INDEX IF NOT EXISTS idx_positions_lookup
	ON positions (account_id, exchange, market_type, symbol, position_side, snapshot_time);

	CREATE TABLE IF NOT EXISTS paper_positions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		strategy_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL,
		symbol TEXT NOT NULL,
		position_side TEXT NOT NULL,
		quantity REAL NOT NULL,
		entry_price REAL NOT NULL,
		mark_price REAL NOT NULL,
		take_profit_price REAL NOT NULL DEFAULT 0,
		stop_loss_price REAL NOT NULL DEFAULT 0,
		realized_pnl REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		opened_at DATETIME NOT NULL,
		closed_at DATETIME,
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_paper_positions_lookup
	ON paper_positions (account_id, strategy_id, exchange, market_type, symbol, status, updated_at);

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
	if err != nil {
		return err
	}
	return migratePositionSnapshotMarketTypeKey(ctx, db)
}

func migratePositionSnapshotMarketTypeKey(ctx context.Context, db *sql.DB) error {
	needsMigration, err := positionSnapshotKeyNeedsMigration(ctx, db)
	if err != nil {
		return err
	}
	if !needsMigration {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`ALTER TABLE positions RENAME TO positions_old;`,
		`CREATE TABLE positions (
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
			UNIQUE(account_id, exchange, market_type, symbol, position_side, snapshot_time)
		);`,
		`INSERT INTO positions (
			id, account_id, exchange, market_type, symbol, position_side, quantity,
			entry_price, mark_price, liquidation_price, leverage, margin_mode,
			unrealized_pnl, notional, liquidation_distance_pct, snapshot_time, created_at
		)
		SELECT id, account_id, exchange, market_type, symbol, position_side, quantity,
			entry_price, mark_price, liquidation_price, leverage, margin_mode,
			unrealized_pnl, notional, liquidation_distance_pct, snapshot_time, created_at
		FROM positions_old;`,
		`DROP TABLE positions_old;`,
		`CREATE INDEX IF NOT EXISTS idx_positions_lookup
			ON positions (account_id, exchange, market_type, symbol, position_side, snapshot_time);`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate positions market_type key: %w", err)
		}
	}
	return tx.Commit()
}

func positionSnapshotKeyNeedsMigration(ctx context.Context, db *sql.DB) (bool, error) {
	var tableSQL string
	err := db.QueryRowContext(ctx, `
	SELECT sql
	FROM sqlite_master
	WHERE type = 'table' AND name = 'positions';
	`).Scan(&tableSQL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	normalizedSQL := strings.ToLower(strings.Join(strings.Fields(tableSQL), " "))
	return !strings.Contains(normalizedSQL, "unique(account_id, exchange, market_type, symbol, position_side, snapshot_time)"), nil
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
	ON CONFLICT(account_id, exchange, market_type, symbol, position_side, snapshot_time) DO UPDATE SET
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

func (r *SQLiteRepository) OpenPaperPosition(ctx context.Context, record PaperPositionRecord) (int64, error) {
	record = normalizePaperPosition(record)
	if err := validatePaperPosition(record); err != nil {
		return 0, err
	}
	if record.Status == "" {
		record.Status = PaperPositionOpen
	}
	now := time.Now().UTC()
	if record.OpenedAt.IsZero() {
		record.OpenedAt = now
	}
	record.UpdatedAt = now
	var closedAt any
	if record.ClosedAt != nil {
		closedAt = record.ClosedAt.UTC()
	}

	result, err := r.db.ExecContext(ctx, `
	INSERT INTO paper_positions (
		account_id, strategy_id, exchange, market_type, symbol, position_side,
		quantity, entry_price, mark_price, take_profit_price, stop_loss_price,
		realized_pnl, status, opened_at, closed_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, record.AccountID, record.StrategyID, record.Exchange, record.MarketType, record.Symbol, record.PositionSide, record.Quantity, record.EntryPrice, record.MarkPrice, record.TakeProfitPrice, record.StopLossPrice, record.RealizedPnL, string(record.Status), record.OpenedAt, closedAt, record.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) LatestOpenPaperPosition(ctx context.Context, accountID string, strategyID string, exchange string, marketType string, symbol string) (PaperPositionRecord, error) {
	accountID = strings.TrimSpace(accountID)
	strategyID = strings.TrimSpace(strategyID)
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	marketType = strings.ToLower(strings.TrimSpace(marketType))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	row := r.db.QueryRowContext(ctx, `
	SELECT id, account_id, strategy_id, exchange, market_type, symbol, position_side,
		quantity, entry_price, mark_price, take_profit_price, stop_loss_price,
		realized_pnl, status, opened_at, closed_at, updated_at
	FROM paper_positions
	WHERE account_id = ? AND strategy_id = ? AND exchange = ? AND market_type = ?
		AND symbol = ? AND status = ?
	ORDER BY opened_at DESC, id DESC
	LIMIT 1;
	`, accountID, strategyID, exchange, marketType, symbol, string(PaperPositionOpen))
	return scanPaperPosition(row)
}

func (r *SQLiteRepository) SumClosedPaperPositionRealizedPnL(ctx context.Context, accountID string, strategyID string, exchange string, marketType string, symbol string) (float64, error) {
	accountID = strings.TrimSpace(accountID)
	strategyID = strings.TrimSpace(strategyID)
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	marketType = strings.ToLower(strings.TrimSpace(marketType))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	var total sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
	SELECT COALESCE(SUM(realized_pnl), 0)
	FROM paper_positions
	WHERE account_id = ? AND strategy_id = ? AND exchange = ? AND market_type = ?
		AND symbol = ? AND status = ?;
	`, accountID, strategyID, exchange, marketType, symbol, string(PaperPositionClosed)).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}

func (r *SQLiteRepository) UpdatePaperPositionMark(ctx context.Context, id int64, markPrice float64) error {
	if id <= 0 {
		return errors.New("paper position id is required")
	}
	if markPrice <= 0 {
		return errors.New("mark price must be positive")
	}
	_, err := r.db.ExecContext(ctx, `
	UPDATE paper_positions
	SET mark_price = ?, updated_at = ?
	WHERE id = ? AND status = ?;
	`, markPrice, time.Now().UTC(), id, string(PaperPositionOpen))
	return err
}

func (r *SQLiteRepository) ClosePaperPosition(ctx context.Context, id int64, exitPrice float64, closedAt time.Time) (PaperPositionRecord, error) {
	return r.closePaperPosition(ctx, id, exitPrice, closedAt, nil)
}

func (r *SQLiteRepository) ClosePaperPositionWithRealizedPnL(ctx context.Context, id int64, exitPrice float64, closedAt time.Time, realizedPnL float64) (PaperPositionRecord, error) {
	if math.IsNaN(realizedPnL) || math.IsInf(realizedPnL, 0) {
		return PaperPositionRecord{}, errors.New("realized pnl must be finite")
	}
	return r.closePaperPosition(ctx, id, exitPrice, closedAt, &realizedPnL)
}

func (r *SQLiteRepository) closePaperPosition(ctx context.Context, id int64, exitPrice float64, closedAt time.Time, realizedPnLOverride *float64) (PaperPositionRecord, error) {
	if id <= 0 {
		return PaperPositionRecord{}, errors.New("paper position id is required")
	}
	if exitPrice <= 0 {
		return PaperPositionRecord{}, errors.New("exit price must be positive")
	}
	position, err := r.paperPositionByID(ctx, id)
	if err != nil {
		return PaperPositionRecord{}, err
	}
	if position.Status != PaperPositionOpen {
		return position, nil
	}
	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}
	realizedPnL := paperPositionPnL(position, exitPrice)
	if realizedPnLOverride != nil {
		realizedPnL = *realizedPnLOverride
	}
	_, err = r.db.ExecContext(ctx, `
	UPDATE paper_positions
	SET mark_price = ?, realized_pnl = ?, status = ?, closed_at = ?, updated_at = ?
	WHERE id = ?;
	`, exitPrice, realizedPnL, string(PaperPositionClosed), closedAt.UTC(), time.Now().UTC(), id)
	if err != nil {
		return PaperPositionRecord{}, err
	}
	return r.paperPositionByID(ctx, id)
}

func (r *SQLiteRepository) paperPositionByID(ctx context.Context, id int64) (PaperPositionRecord, error) {
	row := r.db.QueryRowContext(ctx, `
	SELECT id, account_id, strategy_id, exchange, market_type, symbol, position_side,
		quantity, entry_price, mark_price, take_profit_price, stop_loss_price,
		realized_pnl, status, opened_at, closed_at, updated_at
	FROM paper_positions
	WHERE id = ?;
	`, id)
	return scanPaperPosition(row)
}

func normalizePaperPosition(record PaperPositionRecord) PaperPositionRecord {
	record.AccountID = strings.TrimSpace(record.AccountID)
	record.StrategyID = strings.TrimSpace(record.StrategyID)
	record.Exchange = strings.ToLower(strings.TrimSpace(record.Exchange))
	record.MarketType = strings.ToLower(strings.TrimSpace(record.MarketType))
	record.Symbol = strings.ToUpper(strings.TrimSpace(record.Symbol))
	record.PositionSide = strings.ToLower(strings.TrimSpace(record.PositionSide))
	return record
}

func validatePaperPosition(record PaperPositionRecord) error {
	if record.AccountID == "" || record.StrategyID == "" || record.Exchange == "" || record.MarketType == "" || record.Symbol == "" || record.PositionSide == "" {
		return errors.New("account id, strategy id, exchange, market type, symbol and position side are required")
	}
	if record.Quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if record.EntryPrice <= 0 || record.MarkPrice <= 0 {
		return errors.New("entry price and mark price must be positive")
	}
	return nil
}

func scanPaperPosition(scanner interface{ Scan(dest ...any) error }) (PaperPositionRecord, error) {
	var record PaperPositionRecord
	var status string
	var closedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.AccountID,
		&record.StrategyID,
		&record.Exchange,
		&record.MarketType,
		&record.Symbol,
		&record.PositionSide,
		&record.Quantity,
		&record.EntryPrice,
		&record.MarkPrice,
		&record.TakeProfitPrice,
		&record.StopLossPrice,
		&record.RealizedPnL,
		&status,
		&record.OpenedAt,
		&closedAt,
		&record.UpdatedAt,
	); err != nil {
		return PaperPositionRecord{}, err
	}
	record.Status = PaperPositionStatus(status)
	if closedAt.Valid {
		record.ClosedAt = &closedAt.Time
	}
	return record, nil
}

func paperPositionPnL(position PaperPositionRecord, markPrice float64) float64 {
	qty := abs(position.Quantity)
	if strings.EqualFold(position.PositionSide, "short") {
		return (position.EntryPrice - markPrice) * qty
	}
	return (markPrice - position.EntryPrice) * qty
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
