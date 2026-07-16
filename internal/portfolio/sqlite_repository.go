package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
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
			wallet_balance TEXT NOT NULL DEFAULT '',
			open_order_margin_frozen TEXT NOT NULL DEFAULT '',
			isolated_margin TEXT NOT NULL DEFAULT '',
			crossed_margin TEXT NOT NULL DEFAULT '',
			available_balance_raw TEXT NOT NULL DEFAULT '',
			bonus TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
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
		position_model TEXT NOT NULL DEFAULT '',
		quantity REAL NOT NULL,
		entry_price REAL NOT NULL,
		mark_price REAL NOT NULL,
		liquidation_price REAL NOT NULL DEFAULT 0,
		leverage REAL NOT NULL DEFAULT 1,
		margin_mode TEXT NOT NULL DEFAULT '',
			unrealized_pnl REAL NOT NULL DEFAULT 0,
			notional REAL NOT NULL DEFAULT 0,
			liquidation_distance_pct REAL NOT NULL DEFAULT 0,
			exchange_position_id TEXT NOT NULL DEFAULT '',
			close_order_size TEXT NOT NULL DEFAULT '',
			available_close_size TEXT NOT NULL DEFAULT '',
			isolated_margin_raw TEXT NOT NULL DEFAULT '',
			open_order_margin_raw TEXT NOT NULL DEFAULT '',
			realized_profit_raw TEXT NOT NULL DEFAULT '',
			auto_margin INTEGER NOT NULL DEFAULT 0,
			contract_size TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
			snapshot_time DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			UNIQUE(account_id, exchange, market_type, symbol, position_side, snapshot_time)
		);

	CREATE INDEX IF NOT EXISTS idx_positions_lookup
	ON positions (account_id, exchange, market_type, symbol, position_side, snapshot_time);

	CREATE TABLE IF NOT EXISTS position_configs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		exchange TEXT NOT NULL,
		symbol TEXT NOT NULL,
		position_type TEXT NOT NULL DEFAULT '',
		position_side TEXT NOT NULL DEFAULT '',
		position_model TEXT NOT NULL DEFAULT '',
		auto_margin INTEGER NOT NULL DEFAULT 0,
		leverage INTEGER NOT NULL DEFAULT 0,
		raw_json TEXT NOT NULL DEFAULT '',
		snapshot_time DATETIME NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(account_id, exchange, symbol, position_side, snapshot_time)
	);

	CREATE INDEX IF NOT EXISTS idx_position_configs_lookup
	ON position_configs (account_id, exchange, symbol, position_side, snapshot_time);

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
		initial_stop_loss_price REAL NOT NULL DEFAULT 0,
		realized_pnl REAL NOT NULL DEFAULT 0,
		close_reason TEXT NOT NULL DEFAULT '',
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
			underlying_type TEXT NOT NULL DEFAULT '',
			contract_size TEXT NOT NULL DEFAULT '',
			trade_switch INTEGER NOT NULL DEFAULT 0,
			state INTEGER NOT NULL DEFAULT 0,
			init_leverage INTEGER NOT NULL DEFAULT 0,
			init_position_type TEXT NOT NULL DEFAULT '',
			base_asset TEXT NOT NULL,
			quote_asset TEXT NOT NULL,
			base_coin_precision INTEGER NOT NULL DEFAULT 0,
			base_coin_display_precision INTEGER NOT NULL DEFAULT 0,
			quote_coin_precision INTEGER NOT NULL DEFAULT 0,
			quote_coin_display_precision INTEGER NOT NULL DEFAULT 0,
			quantity_precision INTEGER NOT NULL DEFAULT 0,
			price_precision INTEGER NOT NULL DEFAULT 0,
			support_order_type TEXT NOT NULL DEFAULT '',
			support_time_in_force TEXT NOT NULL DEFAULT '',
			support_entrust_type TEXT NOT NULL DEFAULT '',
			support_position_type TEXT NOT NULL DEFAULT '',
			min_price TEXT NOT NULL DEFAULT '',
			tick_size TEXT NOT NULL,
			step_size TEXT NOT NULL,
			min_qty TEXT NOT NULL,
			min_notional TEXT NOT NULL,
			max_notional TEXT NOT NULL DEFAULT '',
			multiplier_down TEXT NOT NULL DEFAULT '',
			multiplier_up TEXT NOT NULL DEFAULT '',
			max_open_orders INTEGER NOT NULL DEFAULT 0,
			max_entrusts INTEGER NOT NULL DEFAULT 0,
			maker_fee TEXT NOT NULL DEFAULT '',
			taker_fee TEXT NOT NULL DEFAULT '',
			liquidation_fee TEXT NOT NULL DEFAULT '',
			market_take_bound TEXT NOT NULL DEFAULT '',
			depth_precision_merge INTEGER NOT NULL DEFAULT 0,
			labels_json TEXT NOT NULL DEFAULT '[]',
			onboard_time DATETIME,
			english_name TEXT NOT NULL DEFAULT '',
			chinese_name TEXT NOT NULL DEFAULT '',
			min_step_price TEXT NOT NULL DEFAULT '',
			base_coin_name TEXT NOT NULL DEFAULT '',
			quote_coin_name TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
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
			max_nominal_value TEXT NOT NULL DEFAULT '',
			maint_margin_rate TEXT NOT NULL DEFAULT '',
			start_margin_rate TEXT NOT NULL DEFAULT '',
			max_start_margin_rate TEXT NOT NULL DEFAULT '',
			max_leverage TEXT NOT NULL DEFAULT '',
			min_leverage TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
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
	if err := addPortfolioColumnIfMissing(ctx, db, "paper_positions", "close_reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addPortfolioColumnIfMissing(ctx, db, "paper_positions", "initial_stop_loss_price", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	columns := []struct {
		table      string
		column     string
		definition string
	}{
		{"balances", "wallet_balance", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "open_order_margin_frozen", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "isolated_margin", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "crossed_margin", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "available_balance_raw", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "bonus", "TEXT NOT NULL DEFAULT ''"},
		{"balances", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "position_model", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "exchange_position_id", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "close_order_size", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "available_close_size", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "isolated_margin_raw", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "open_order_margin_raw", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "realized_profit_raw", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "auto_margin", "INTEGER NOT NULL DEFAULT 0"},
		{"positions", "contract_size", "TEXT NOT NULL DEFAULT ''"},
		{"positions", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "underlying_type", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "contract_size", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "trade_switch", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "state", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "init_leverage", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "init_position_type", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "base_coin_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "base_coin_display_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "quote_coin_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "quote_coin_display_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "quantity_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "price_precision", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "support_order_type", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "support_time_in_force", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "support_entrust_type", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "support_position_type", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "min_price", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "max_notional", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "multiplier_down", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "multiplier_up", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "max_open_orders", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "max_entrusts", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "maker_fee", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "taker_fee", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "liquidation_fee", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "market_take_bound", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "depth_precision_merge", "INTEGER NOT NULL DEFAULT 0"},
		{"contract_specs", "labels_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"contract_specs", "onboard_time", "DATETIME"},
		{"contract_specs", "english_name", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "chinese_name", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "min_step_price", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "base_coin_name", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "quote_coin_name", "TEXT NOT NULL DEFAULT ''"},
		{"contract_specs", "raw_json", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "max_nominal_value", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "maint_margin_rate", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "start_margin_rate", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "max_start_margin_rate", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "max_leverage", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "min_leverage", "TEXT NOT NULL DEFAULT ''"},
		{"leverage_brackets", "raw_json", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := addPortfolioColumnIfMissing(ctx, db, column.table, column.column, column.definition); err != nil {
			return err
		}
	}
	return migratePositionSnapshotMarketTypeKey(ctx, db)
}

func addPortfolioColumnIfMissing(ctx context.Context, db *sql.DB, table string, column string, definition string) error {
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
			position_model TEXT NOT NULL DEFAULT '',
			quantity REAL NOT NULL,
			entry_price REAL NOT NULL,
			mark_price REAL NOT NULL,
			liquidation_price REAL NOT NULL DEFAULT 0,
			leverage REAL NOT NULL DEFAULT 1,
				margin_mode TEXT NOT NULL DEFAULT '',
				unrealized_pnl REAL NOT NULL DEFAULT 0,
				notional REAL NOT NULL DEFAULT 0,
				liquidation_distance_pct REAL NOT NULL DEFAULT 0,
				exchange_position_id TEXT NOT NULL DEFAULT '',
				close_order_size TEXT NOT NULL DEFAULT '',
				available_close_size TEXT NOT NULL DEFAULT '',
				isolated_margin_raw TEXT NOT NULL DEFAULT '',
				open_order_margin_raw TEXT NOT NULL DEFAULT '',
				realized_profit_raw TEXT NOT NULL DEFAULT '',
				auto_margin INTEGER NOT NULL DEFAULT 0,
				contract_size TEXT NOT NULL DEFAULT '',
				raw_json TEXT NOT NULL DEFAULT '',
				snapshot_time DATETIME NOT NULL,
				created_at DATETIME NOT NULL,
				UNIQUE(account_id, exchange, market_type, symbol, position_side, snapshot_time)
			);`,
		`INSERT INTO positions (
				id, account_id, exchange, market_type, symbol, position_side, position_model, quantity,
				entry_price, mark_price, liquidation_price, leverage, margin_mode,
				unrealized_pnl, notional, liquidation_distance_pct, exchange_position_id,
				close_order_size, available_close_size, isolated_margin_raw, open_order_margin_raw,
				realized_profit_raw, auto_margin, contract_size, raw_json, snapshot_time, created_at
			)
			SELECT id, account_id, exchange, market_type, symbol, position_side, position_model, quantity,
				entry_price, mark_price, liquidation_price, leverage, margin_mode,
				unrealized_pnl, notional, liquidation_distance_pct, exchange_position_id,
				close_order_size, available_close_size, isolated_margin_raw, open_order_margin_raw,
				realized_profit_raw, auto_margin, contract_size, raw_json, snapshot_time, created_at
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
	snapshot.WalletBalance = strings.TrimSpace(snapshot.WalletBalance)
	snapshot.OpenOrderMarginFrozen = strings.TrimSpace(snapshot.OpenOrderMarginFrozen)
	snapshot.IsolatedMargin = strings.TrimSpace(snapshot.IsolatedMargin)
	snapshot.CrossedMargin = strings.TrimSpace(snapshot.CrossedMargin)
	snapshot.AvailableBalance = strings.TrimSpace(snapshot.AvailableBalance)
	snapshot.Bonus = strings.TrimSpace(snapshot.Bonus)
	snapshot.RawJSON = strings.TrimSpace(snapshot.RawJSON)
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
			account_id, exchange, asset, free, locked, total, usd_value,
			wallet_balance, open_order_margin_frozen, isolated_margin, crossed_margin,
			available_balance_raw, bonus, raw_json, snapshot_time, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, exchange, asset, snapshot_time) DO UPDATE SET
			free = excluded.free,
			locked = excluded.locked,
			total = excluded.total,
			usd_value = excluded.usd_value,
			wallet_balance = excluded.wallet_balance,
			open_order_margin_frozen = excluded.open_order_margin_frozen,
			isolated_margin = excluded.isolated_margin,
			crossed_margin = excluded.crossed_margin,
			available_balance_raw = excluded.available_balance_raw,
			bonus = excluded.bonus,
			raw_json = excluded.raw_json;
		`, snapshot.AccountID, snapshot.Exchange, snapshot.Asset, snapshot.Free, snapshot.Locked, snapshot.Total, snapshot.USDValue, snapshot.WalletBalance, snapshot.OpenOrderMarginFrozen, snapshot.IsolatedMargin, snapshot.CrossedMargin, snapshot.AvailableBalance, snapshot.Bonus, snapshot.RawJSON, snapshot.SnapshotTime.UTC(), snapshot.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) LatestLiveBalanceSnapshot(ctx context.Context, accountID string, exchange string, asset string) (BalanceSnapshot, error) {
	accountID = strings.TrimSpace(accountID)
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if accountID == "" || exchange == "" || asset == "" {
		return BalanceSnapshot{}, errors.New("account id, exchange and asset are required")
	}
	row := r.db.QueryRowContext(ctx, `
	SELECT id, account_id, exchange, asset, free, locked, total, usd_value,
		wallet_balance, open_order_margin_frozen, isolated_margin, crossed_margin,
		available_balance_raw, bonus, raw_json, snapshot_time, created_at
	FROM balances
	WHERE account_id = ?
		AND exchange = ?
		AND asset = ?
		AND (wallet_balance <> '' OR available_balance_raw <> '' OR raw_json <> '')
	ORDER BY snapshot_time DESC, id DESC
	LIMIT 1;
	`, accountID, exchange, asset)
	return scanBalanceSnapshot(row)
}

func scanBalanceSnapshot(row interface {
	Scan(dest ...any) error
}) (BalanceSnapshot, error) {
	var snapshot BalanceSnapshot
	err := row.Scan(
		&snapshot.ID,
		&snapshot.AccountID,
		&snapshot.Exchange,
		&snapshot.Asset,
		&snapshot.Free,
		&snapshot.Locked,
		&snapshot.Total,
		&snapshot.USDValue,
		&snapshot.WalletBalance,
		&snapshot.OpenOrderMarginFrozen,
		&snapshot.IsolatedMargin,
		&snapshot.CrossedMargin,
		&snapshot.AvailableBalance,
		&snapshot.Bonus,
		&snapshot.RawJSON,
		&snapshot.SnapshotTime,
		&snapshot.CreatedAt,
	)
	if err != nil {
		return BalanceSnapshot{}, err
	}
	return snapshot, nil
}

func (r *SQLiteRepository) SavePositionSnapshot(ctx context.Context, snapshot PositionSnapshot) (int64, error) {
	snapshot.AccountID = strings.TrimSpace(snapshot.AccountID)
	snapshot.Exchange = strings.ToLower(strings.TrimSpace(snapshot.Exchange))
	snapshot.MarketType = strings.ToLower(strings.TrimSpace(snapshot.MarketType))
	snapshot.Symbol = strings.ToUpper(strings.TrimSpace(snapshot.Symbol))
	snapshot.PositionSide = strings.ToLower(strings.TrimSpace(snapshot.PositionSide))
	snapshot.PositionModel = normalizePositionModel(snapshot.PositionModel)
	snapshot.ExchangePositionID = strings.TrimSpace(snapshot.ExchangePositionID)
	snapshot.CloseOrderSize = strings.TrimSpace(snapshot.CloseOrderSize)
	snapshot.AvailableCloseSize = strings.TrimSpace(snapshot.AvailableCloseSize)
	snapshot.IsolatedMargin = strings.TrimSpace(snapshot.IsolatedMargin)
	snapshot.OpenOrderMargin = strings.TrimSpace(snapshot.OpenOrderMargin)
	snapshot.RealizedProfit = strings.TrimSpace(snapshot.RealizedProfit)
	snapshot.ContractSize = strings.TrimSpace(snapshot.ContractSize)
	snapshot.RawJSON = strings.TrimSpace(snapshot.RawJSON)
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
			account_id, exchange, market_type, symbol, position_side, position_model, quantity,
			entry_price, mark_price, liquidation_price, leverage, margin_mode,
			unrealized_pnl, notional, liquidation_distance_pct, exchange_position_id,
			close_order_size, available_close_size, isolated_margin_raw, open_order_margin_raw,
			realized_profit_raw, auto_margin, contract_size, raw_json, snapshot_time, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, exchange, market_type, symbol, position_side, snapshot_time) DO UPDATE SET
			position_model = excluded.position_model,
			quantity = excluded.quantity,
			entry_price = excluded.entry_price,
			mark_price = excluded.mark_price,
			liquidation_price = excluded.liquidation_price,
			leverage = excluded.leverage,
			margin_mode = excluded.margin_mode,
			unrealized_pnl = excluded.unrealized_pnl,
			notional = excluded.notional,
			liquidation_distance_pct = excluded.liquidation_distance_pct,
			exchange_position_id = excluded.exchange_position_id,
			close_order_size = excluded.close_order_size,
			available_close_size = excluded.available_close_size,
			isolated_margin_raw = excluded.isolated_margin_raw,
			open_order_margin_raw = excluded.open_order_margin_raw,
			realized_profit_raw = excluded.realized_profit_raw,
			auto_margin = excluded.auto_margin,
			contract_size = excluded.contract_size,
			raw_json = excluded.raw_json;
		`, snapshot.AccountID, snapshot.Exchange, snapshot.MarketType, snapshot.Symbol, snapshot.PositionSide, snapshot.PositionModel, snapshot.Quantity, snapshot.EntryPrice, snapshot.MarkPrice, snapshot.LiquidationPrice, snapshot.Leverage, snapshot.MarginMode, snapshot.UnrealizedPnL, snapshot.Notional, snapshot.LiquidationDistance, snapshot.ExchangePositionID, snapshot.CloseOrderSize, snapshot.AvailableCloseSize, snapshot.IsolatedMargin, snapshot.OpenOrderMargin, snapshot.RealizedProfit, boolToInt(snapshot.AutoMargin), snapshot.ContractSize, snapshot.RawJSON, snapshot.SnapshotTime.UTC(), snapshot.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SavePositionConfig(ctx context.Context, config PositionConfig) (int64, error) {
	config.AccountID = strings.TrimSpace(config.AccountID)
	config.Exchange = strings.ToLower(strings.TrimSpace(config.Exchange))
	config.Symbol = strings.ToUpper(strings.TrimSpace(config.Symbol))
	config.PositionType = strings.ToLower(strings.TrimSpace(config.PositionType))
	config.PositionSide = strings.ToLower(strings.TrimSpace(config.PositionSide))
	config.PositionModel = normalizePositionModel(config.PositionModel)
	config.RawJSON = strings.TrimSpace(config.RawJSON)
	if config.AccountID == "" || config.Exchange == "" || config.Symbol == "" || config.PositionSide == "" {
		return 0, errors.New("account id, exchange, symbol and position side are required")
	}
	now := time.Now().UTC()
	if config.SnapshotTime.IsZero() {
		config.SnapshotTime = now
	}
	config.CreatedAt = now

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO position_configs (
			account_id, exchange, symbol, position_type, position_side, position_model,
			auto_margin, leverage, raw_json, snapshot_time, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, exchange, symbol, position_side, snapshot_time) DO UPDATE SET
			position_type = excluded.position_type,
			position_model = excluded.position_model,
			auto_margin = excluded.auto_margin,
			leverage = excluded.leverage,
			raw_json = excluded.raw_json;
	`, config.AccountID, config.Exchange, config.Symbol, config.PositionType, config.PositionSide, config.PositionModel, boolToInt(config.AutoMargin), config.Leverage, config.RawJSON, config.SnapshotTime.UTC(), config.CreatedAt)
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

func (r *SQLiteRepository) SaveContractSpec(ctx context.Context, spec ContractSpec) (int64, error) {
	spec.Exchange = strings.ToLower(strings.TrimSpace(spec.Exchange))
	spec.Symbol = strings.ToUpper(strings.TrimSpace(spec.Symbol))
	spec.ContractType = strings.ToUpper(strings.TrimSpace(spec.ContractType))
	spec.UnderlyingType = strings.TrimSpace(spec.UnderlyingType)
	spec.ContractSize = strings.TrimSpace(spec.ContractSize)
	spec.InitPositionType = strings.TrimSpace(spec.InitPositionType)
	spec.BaseAsset = strings.ToUpper(strings.TrimSpace(spec.BaseAsset))
	spec.QuoteAsset = strings.ToUpper(strings.TrimSpace(spec.QuoteAsset))
	spec.SupportOrderType = strings.TrimSpace(spec.SupportOrderType)
	spec.SupportTimeInForce = strings.TrimSpace(spec.SupportTimeInForce)
	spec.SupportEntrustType = strings.TrimSpace(spec.SupportEntrustType)
	spec.SupportPositionType = strings.TrimSpace(spec.SupportPositionType)
	spec.MinPrice = strings.TrimSpace(spec.MinPrice)
	spec.MinQty = strings.TrimSpace(spec.MinQty)
	spec.MinNotional = strings.TrimSpace(spec.MinNotional)
	spec.MaxNotional = strings.TrimSpace(spec.MaxNotional)
	spec.MultiplierDown = strings.TrimSpace(spec.MultiplierDown)
	spec.MultiplierUp = strings.TrimSpace(spec.MultiplierUp)
	spec.MakerFee = strings.TrimSpace(spec.MakerFee)
	spec.TakerFee = strings.TrimSpace(spec.TakerFee)
	spec.LiquidationFee = strings.TrimSpace(spec.LiquidationFee)
	spec.MarketTakeBound = strings.TrimSpace(spec.MarketTakeBound)
	spec.LabelsJSON = strings.TrimSpace(spec.LabelsJSON)
	spec.EnglishName = strings.TrimSpace(spec.EnglishName)
	spec.ChineseName = strings.TrimSpace(spec.ChineseName)
	spec.MinStepPrice = strings.TrimSpace(spec.MinStepPrice)
	spec.BaseCoinName = strings.TrimSpace(spec.BaseCoinName)
	spec.QuoteCoinName = strings.TrimSpace(spec.QuoteCoinName)
	spec.TickSize = strings.TrimSpace(spec.TickSize)
	spec.StepSize = strings.TrimSpace(spec.StepSize)
	spec.RawJSON = strings.TrimSpace(spec.RawJSON)
	if spec.TickSize == "" {
		spec.TickSize = spec.MinStepPrice
	}
	if spec.StepSize == "" {
		spec.StepSize = spec.MinQty
	}
	if spec.LabelsJSON == "" {
		spec.LabelsJSON = "[]"
	}
	if spec.Exchange == "" || spec.Symbol == "" || spec.ContractType == "" || spec.BaseAsset == "" || spec.QuoteAsset == "" {
		return 0, errors.New("exchange, symbol, contract type, base asset and quote asset are required")
	}
	now := time.Now().UTC()
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	}
	spec.UpdatedAt = now

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO contract_specs (
			exchange, symbol, contract_type, underlying_type, contract_size, trade_switch,
			state, init_leverage, init_position_type, base_asset, quote_asset,
			base_coin_precision, base_coin_display_precision, quote_coin_precision,
			quote_coin_display_precision, quantity_precision, price_precision,
			support_order_type, support_time_in_force, support_entrust_type,
			support_position_type, min_price, tick_size, step_size, min_qty, min_notional,
			max_notional, multiplier_down, multiplier_up, max_open_orders, max_entrusts,
			maker_fee, taker_fee, liquidation_fee, market_take_bound, depth_precision_merge,
			labels_json, onboard_time, english_name, chinese_name, min_step_price,
			base_coin_name, quote_coin_name, raw_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(exchange, symbol, contract_type) DO UPDATE SET
			underlying_type = excluded.underlying_type,
			contract_size = excluded.contract_size,
			trade_switch = excluded.trade_switch,
			state = excluded.state,
			init_leverage = excluded.init_leverage,
			init_position_type = excluded.init_position_type,
			base_asset = excluded.base_asset,
			quote_asset = excluded.quote_asset,
			base_coin_precision = excluded.base_coin_precision,
			base_coin_display_precision = excluded.base_coin_display_precision,
			quote_coin_precision = excluded.quote_coin_precision,
			quote_coin_display_precision = excluded.quote_coin_display_precision,
			quantity_precision = excluded.quantity_precision,
			price_precision = excluded.price_precision,
			support_order_type = excluded.support_order_type,
			support_time_in_force = excluded.support_time_in_force,
			support_entrust_type = excluded.support_entrust_type,
			support_position_type = excluded.support_position_type,
			min_price = excluded.min_price,
			tick_size = excluded.tick_size,
			step_size = excluded.step_size,
			min_qty = excluded.min_qty,
			min_notional = excluded.min_notional,
			max_notional = excluded.max_notional,
			multiplier_down = excluded.multiplier_down,
			multiplier_up = excluded.multiplier_up,
			max_open_orders = excluded.max_open_orders,
			max_entrusts = excluded.max_entrusts,
			maker_fee = excluded.maker_fee,
			taker_fee = excluded.taker_fee,
			liquidation_fee = excluded.liquidation_fee,
			market_take_bound = excluded.market_take_bound,
			depth_precision_merge = excluded.depth_precision_merge,
			labels_json = excluded.labels_json,
			onboard_time = excluded.onboard_time,
			english_name = excluded.english_name,
			chinese_name = excluded.chinese_name,
			min_step_price = excluded.min_step_price,
			base_coin_name = excluded.base_coin_name,
			quote_coin_name = excluded.quote_coin_name,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at;
	`, spec.Exchange, spec.Symbol, spec.ContractType, spec.UnderlyingType, spec.ContractSize, boolToInt(spec.TradeSwitch), spec.State, spec.InitLeverage, spec.InitPositionType, spec.BaseAsset, spec.QuoteAsset, spec.BaseCoinPrecision, spec.BaseCoinDisplayPrecision, spec.QuoteCoinPrecision, spec.QuoteCoinDisplayPrecision, spec.QuantityPrecision, spec.PricePrecision, spec.SupportOrderType, spec.SupportTimeInForce, spec.SupportEntrustType, spec.SupportPositionType, spec.MinPrice, spec.TickSize, spec.StepSize, spec.MinQty, spec.MinNotional, spec.MaxNotional, spec.MultiplierDown, spec.MultiplierUp, spec.MaxOpenOrders, spec.MaxEntrusts, spec.MakerFee, spec.TakerFee, spec.LiquidationFee, spec.MarketTakeBound, spec.DepthPrecisionMerge, spec.LabelsJSON, nullableTime(spec.OnboardTime), spec.EnglishName, spec.ChineseName, spec.MinStepPrice, spec.BaseCoinName, spec.QuoteCoinName, spec.RawJSON, spec.CreatedAt, spec.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SQLiteRepository) SaveLeverageBracket(ctx context.Context, bracket LeverageBracket) (int64, error) {
	bracket.Exchange = strings.ToLower(strings.TrimSpace(bracket.Exchange))
	bracket.Symbol = strings.ToUpper(strings.TrimSpace(bracket.Symbol))
	bracket.MaxNominalValue = strings.TrimSpace(bracket.MaxNominalValue)
	bracket.MaintMarginRate = strings.TrimSpace(bracket.MaintMarginRate)
	bracket.StartMarginRate = strings.TrimSpace(bracket.StartMarginRate)
	bracket.MaxStartMarginRate = strings.TrimSpace(bracket.MaxStartMarginRate)
	bracket.MaxLeverage = strings.TrimSpace(bracket.MaxLeverage)
	bracket.MinLeverage = strings.TrimSpace(bracket.MinLeverage)
	bracket.RawJSON = strings.TrimSpace(bracket.RawJSON)
	if bracket.Exchange == "" || bracket.Symbol == "" || bracket.Bracket <= 0 {
		return 0, errors.New("exchange, symbol and positive bracket are required")
	}
	if bracket.CreatedAt.IsZero() {
		bracket.CreatedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO leverage_brackets (
			exchange, symbol, bracket, initial_leverage, notional_cap, notional_floor,
			maintenance_margin_ratio, max_nominal_value, maint_margin_rate,
			start_margin_rate, max_start_margin_rate, max_leverage, min_leverage,
			raw_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(exchange, symbol, bracket) DO UPDATE SET
			initial_leverage = excluded.initial_leverage,
			notional_cap = excluded.notional_cap,
			notional_floor = excluded.notional_floor,
			maintenance_margin_ratio = excluded.maintenance_margin_ratio,
			max_nominal_value = excluded.max_nominal_value,
			maint_margin_rate = excluded.maint_margin_rate,
			start_margin_rate = excluded.start_margin_rate,
			max_start_margin_rate = excluded.max_start_margin_rate,
			max_leverage = excluded.max_leverage,
			min_leverage = excluded.min_leverage,
			raw_json = excluded.raw_json,
			created_at = excluded.created_at;
	`, bracket.Exchange, bracket.Symbol, bracket.Bracket, parseIntDefault(bracket.MaxLeverage), parseFloatDefault(bracket.MaxNominalValue), 0, parseFloatDefault(bracket.MaintMarginRate), bracket.MaxNominalValue, bracket.MaintMarginRate, bracket.StartMarginRate, bracket.MaxStartMarginRate, bracket.MaxLeverage, bracket.MinLeverage, bracket.RawJSON, bracket.CreatedAt)
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
	if record.InitialStopLoss <= 0 {
		record.InitialStopLoss = record.StopLossPrice
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
		initial_stop_loss_price, realized_pnl, close_reason, status, opened_at, closed_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, record.AccountID, record.StrategyID, record.Exchange, record.MarketType, record.Symbol, record.PositionSide, record.Quantity, record.EntryPrice, record.MarkPrice, record.TakeProfitPrice, record.StopLossPrice, record.InitialStopLoss, record.RealizedPnL, strings.TrimSpace(record.CloseReason), string(record.Status), record.OpenedAt, closedAt, record.UpdatedAt)
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
		initial_stop_loss_price, realized_pnl, close_reason, status, opened_at, closed_at, updated_at
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

func (r *SQLiteRepository) SumClosedPaperPositionRealizedPnLSince(ctx context.Context, accountID string, strategyID string, exchange string, marketType string, symbol string, since time.Time) (float64, error) {
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
		AND symbol = ? AND status = ? AND closed_at >= ?;
	`, accountID, strategyID, exchange, marketType, symbol, string(PaperPositionClosed), since.UTC()).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}

func (r *SQLiteRepository) CountConsecutiveClosedPaperPositionLosses(ctx context.Context, accountID string, strategyID string, exchange string, marketType string, symbol string) (int, error) {
	accountID = strings.TrimSpace(accountID)
	strategyID = strings.TrimSpace(strategyID)
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	marketType = strings.ToLower(strings.TrimSpace(marketType))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	rows, err := r.db.QueryContext(ctx, `
	SELECT realized_pnl
	FROM paper_positions
	WHERE account_id = ? AND strategy_id = ? AND exchange = ? AND market_type = ?
		AND symbol = ? AND status = ?
	ORDER BY closed_at DESC, id DESC;
	`, accountID, strategyID, exchange, marketType, symbol, string(PaperPositionClosed))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	losses := 0
	for rows.Next() {
		var realizedPnL float64
		if err := rows.Scan(&realizedPnL); err != nil {
			return 0, err
		}
		if realizedPnL >= 0 {
			break
		}
		losses++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return losses, nil
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

func (r *SQLiteRepository) UpdatePaperPositionStopLoss(ctx context.Context, id int64, stopLossPrice float64) error {
	if id <= 0 {
		return errors.New("paper position id is required")
	}
	if stopLossPrice <= 0 {
		return errors.New("stop loss price must be positive")
	}
	_, err := r.db.ExecContext(ctx, `
	UPDATE paper_positions
	SET stop_loss_price = ?, updated_at = ?
	WHERE id = ? AND status = ?;
	`, stopLossPrice, time.Now().UTC(), id, string(PaperPositionOpen))
	return err
}

func (r *SQLiteRepository) ClosePaperPosition(ctx context.Context, id int64, exitPrice float64, closedAt time.Time) (PaperPositionRecord, error) {
	return r.closePaperPosition(ctx, id, exitPrice, closedAt, nil, "")
}

func (r *SQLiteRepository) ClosePaperPositionWithRealizedPnL(ctx context.Context, id int64, exitPrice float64, closedAt time.Time, realizedPnL float64) (PaperPositionRecord, error) {
	if math.IsNaN(realizedPnL) || math.IsInf(realizedPnL, 0) {
		return PaperPositionRecord{}, errors.New("realized pnl must be finite")
	}
	return r.closePaperPosition(ctx, id, exitPrice, closedAt, &realizedPnL, "")
}

func (r *SQLiteRepository) ClosePaperPositionWithRealizedPnLAndReason(ctx context.Context, id int64, exitPrice float64, closedAt time.Time, realizedPnL float64, closeReason string) (PaperPositionRecord, error) {
	if math.IsNaN(realizedPnL) || math.IsInf(realizedPnL, 0) {
		return PaperPositionRecord{}, errors.New("realized pnl must be finite")
	}
	return r.closePaperPosition(ctx, id, exitPrice, closedAt, &realizedPnL, closeReason)
}

func (r *SQLiteRepository) closePaperPosition(ctx context.Context, id int64, exitPrice float64, closedAt time.Time, realizedPnLOverride *float64, closeReason string) (PaperPositionRecord, error) {
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
	SET mark_price = ?, realized_pnl = ?, close_reason = ?, status = ?, closed_at = ?, updated_at = ?
	WHERE id = ?;
	`, exitPrice, realizedPnL, strings.TrimSpace(closeReason), string(PaperPositionClosed), closedAt.UTC(), time.Now().UTC(), id)
	if err != nil {
		return PaperPositionRecord{}, err
	}
	return r.paperPositionByID(ctx, id)
}

func (r *SQLiteRepository) paperPositionByID(ctx context.Context, id int64) (PaperPositionRecord, error) {
	row := r.db.QueryRowContext(ctx, `
	SELECT id, account_id, strategy_id, exchange, market_type, symbol, position_side,
		quantity, entry_price, mark_price, take_profit_price, stop_loss_price,
		initial_stop_loss_price, realized_pnl, close_reason, status, opened_at, closed_at, updated_at
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
		&record.InitialStopLoss,
		&record.RealizedPnL,
		&record.CloseReason,
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizePositionModel(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "LONG_SHORT":
		return "DISAGGREGATION"
	case "ONE_WAY", "ONEWAY":
		return "AGGREGATION"
	default:
		return normalized
	}
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func parseFloatDefault(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseIntDefault(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}
