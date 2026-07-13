package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gogogo/internal/risk"

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
	CREATE TABLE IF NOT EXISTS orders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		strategy_id TEXT NOT NULL DEFAULT '',
		exchange TEXT NOT NULL,
		market_type TEXT NOT NULL CHECK (market_type IN ('spot', 'perpetual')),
		symbol TEXT NOT NULL,
		client_order_id TEXT NOT NULL,
		side TEXT NOT NULL,
		order_type TEXT NOT NULL,
		time_in_force TEXT NOT NULL DEFAULT '',
		reduce_only INTEGER NOT NULL DEFAULT 0,
		price REAL NOT NULL,
		quantity REAL NOT NULL,
		stop_price REAL NOT NULL DEFAULT 0,
		take_profit_price REAL NOT NULL DEFAULT 0,
		leverage REAL NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		risk_decision TEXT NOT NULL,
		risk_reason TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(exchange, account_id, client_order_id)
	);

	CREATE INDEX IF NOT EXISTS idx_orders_lookup
	ON orders (account_id, strategy_id, exchange, market_type, symbol, created_at);

	CREATE TABLE IF NOT EXISTS risk_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id TEXT NOT NULL,
		strategy_id TEXT NOT NULL DEFAULT '',
		order_id INTEGER NOT NULL,
		client_order_id TEXT NOT NULL,
		event_time DATETIME NOT NULL,
		severity TEXT NOT NULL,
		event_type TEXT NOT NULL,
		symbol TEXT NOT NULL,
		decision TEXT NOT NULL,
		message TEXT NOT NULL,
		context_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		FOREIGN KEY(order_id) REFERENCES orders(id)
	);

	CREATE INDEX IF NOT EXISTS idx_risk_events_lookup
	ON risk_events (account_id, strategy_id, symbol, event_time);
	`)
	if err != nil {
		return err
	}
	return addColumnIfMissing(ctx, db, "orders", "take_profit_price", "REAL NOT NULL DEFAULT 0")
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

func (r *SQLiteRepository) RecordDryRunOrder(ctx context.Context, request DryRunRequest) (DryRunResult, error) {
	request = normalizeDryRunRequest(request)
	riskResult, err := risk.EvaluateOrder(request.RiskConfig, request.Account, request.Order)
	if err != nil {
		return DryRunResult{}, err
	}

	status := orderStatusForDecision(riskResult.Decision)
	reason := riskReason(riskResult.Events)
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return DryRunResult{}, err
	}
	defer tx.Rollback()

	inserted, err := tx.ExecContext(ctx, `
	INSERT INTO orders (
		account_id, strategy_id, exchange, market_type, symbol, client_order_id,
		side, order_type, time_in_force, reduce_only, price, quantity, stop_price,
		take_profit_price, leverage, status, risk_decision, risk_reason, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(exchange, account_id, client_order_id) DO NOTHING;
	`,
		request.Account.AccountID,
		request.StrategyID,
		request.Order.Exchange,
		string(request.Order.MarketType),
		request.Order.Symbol,
		request.ClientOrderID,
		string(request.Order.Side),
		request.OrderType,
		request.TimeInForce,
		boolToInt(request.Order.ReduceOnly),
		request.Order.Price,
		request.Order.Quantity,
		request.Order.StopPrice,
		request.TakeProfitPrice,
		request.Order.Leverage,
		string(status),
		string(riskResult.Decision),
		reason,
		now,
		now,
	)
	if err != nil {
		return DryRunResult{}, err
	}

	rowsAffected, err := inserted.RowsAffected()
	if err != nil {
		return DryRunResult{}, err
	}
	if rowsAffected == 0 {
		order, err := getOrderByClientID(ctx, tx, request.Order.Exchange, request.Account.AccountID, request.ClientOrderID)
		if err != nil {
			return DryRunResult{}, err
		}
		events, err := listRiskEventsByOrderID(ctx, tx, order.ID)
		if err != nil {
			return DryRunResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return DryRunResult{}, err
		}
		return DryRunResult{Order: order, RiskResult: riskResult, Events: events}, nil
	}

	orderID, err := inserted.LastInsertId()
	if err != nil {
		return DryRunResult{}, err
	}
	events, err := insertRiskEvents(ctx, tx, request, orderID, riskResult, now)
	if err != nil {
		return DryRunResult{}, err
	}

	order, err := getOrderByID(ctx, tx, orderID)
	if err != nil {
		return DryRunResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DryRunResult{}, err
	}

	return DryRunResult{Order: order, RiskResult: riskResult, Events: events}, nil
}

func normalizeDryRunRequest(request DryRunRequest) DryRunRequest {
	request.Account.AccountID = strings.TrimSpace(request.Account.AccountID)
	if request.Account.AccountID == "" {
		request.Account.AccountID = "default"
	}
	if request.RiskConfig == (risk.Config{}) {
		request.RiskConfig = risk.DefaultConfig()
	}
	request.StrategyID = strings.TrimSpace(request.StrategyID)
	request.ClientOrderID = strings.TrimSpace(request.ClientOrderID)
	if request.ClientOrderID == "" {
		request.ClientOrderID = fmt.Sprintf("dryrun-%d", time.Now().UTC().UnixNano())
	}
	request.OrderType = strings.ToLower(strings.TrimSpace(request.OrderType))
	if request.OrderType == "" {
		request.OrderType = "limit"
	}
	request.TimeInForce = strings.ToUpper(strings.TrimSpace(request.TimeInForce))
	request.Order.Exchange = strings.ToLower(strings.TrimSpace(request.Order.Exchange))
	request.Order.Symbol = strings.ToUpper(strings.TrimSpace(request.Order.Symbol))
	request.Order.MarketType = risk.MarketType(strings.ToLower(strings.TrimSpace(string(request.Order.MarketType))))
	request.Order.Side = risk.Side(strings.ToLower(strings.TrimSpace(string(request.Order.Side))))
	if request.Order.MarketType == risk.MarketTypeSpot && request.Order.Leverage == 0 {
		request.Order.Leverage = 1
	}
	return request
}

func insertRiskEvents(ctx context.Context, tx *sql.Tx, request DryRunRequest, orderID int64, result risk.Result, now time.Time) ([]RiskEventRecord, error) {
	records := make([]RiskEventRecord, 0, len(result.Events))
	contextJSON, err := riskContextJSON(result)
	if err != nil {
		return nil, err
	}

	for _, event := range result.Events {
		inserted, err := tx.ExecContext(ctx, `
		INSERT INTO risk_events (
			account_id, strategy_id, order_id, client_order_id, event_time,
			severity, event_type, symbol, decision, message, context_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
		`,
			request.Account.AccountID,
			request.StrategyID,
			orderID,
			request.ClientOrderID,
			now,
			string(event.Severity),
			event.Type,
			request.Order.Symbol,
			string(result.Decision),
			event.Message,
			contextJSON,
			now,
		)
		if err != nil {
			return nil, err
		}
		eventID, err := inserted.LastInsertId()
		if err != nil {
			return nil, err
		}
		records = append(records, RiskEventRecord{
			ID:            eventID,
			AccountID:     request.Account.AccountID,
			StrategyID:    request.StrategyID,
			OrderID:       orderID,
			ClientOrderID: request.ClientOrderID,
			EventTime:     now,
			Severity:      event.Severity,
			EventType:     event.Type,
			Symbol:        request.Order.Symbol,
			Decision:      result.Decision,
			Message:       event.Message,
			ContextJSON:   contextJSON,
			CreatedAt:     now,
		})
	}

	return records, nil
}

func riskContextJSON(result risk.Result) (string, error) {
	payload := struct {
		OrderNotional  float64 `json:"order_notional"`
		OrderRisk      float64 `json:"order_risk"`
		TotalExposure  float64 `json:"total_exposure"`
		SymbolExposure float64 `json:"symbol_exposure"`
	}{
		OrderNotional:  result.OrderNotional,
		OrderRisk:      result.OrderRisk,
		TotalExposure:  result.TotalExposure,
		SymbolExposure: result.SymbolExposure,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func riskReason(events []risk.Event) string {
	reasons := make([]string, 0, len(events))
	for _, event := range events {
		reasons = append(reasons, event.Type)
	}
	return strings.Join(reasons, ",")
}

func orderStatusForDecision(decision risk.Decision) OrderStatus {
	switch decision {
	case risk.DecisionAllow:
		return OrderStatusDryRunAccepted
	case risk.DecisionHalt:
		return OrderStatusRiskHalted
	default:
		return OrderStatusRiskRejected
	}
}

func getOrderByID(ctx context.Context, tx *sql.Tx, id int64) (OrderRecord, error) {
	row := tx.QueryRowContext(ctx, `
	SELECT id, account_id, strategy_id, exchange, market_type, symbol, client_order_id,
		side, order_type, time_in_force, reduce_only, price, quantity, stop_price,
		take_profit_price, leverage, status, risk_decision, risk_reason, created_at, updated_at
	FROM orders
	WHERE id = ?;
	`, id)
	return scanOrder(row)
}

func getOrderByClientID(ctx context.Context, tx *sql.Tx, exchange string, accountID string, clientOrderID string) (OrderRecord, error) {
	row := tx.QueryRowContext(ctx, `
	SELECT id, account_id, strategy_id, exchange, market_type, symbol, client_order_id,
		side, order_type, time_in_force, reduce_only, price, quantity, stop_price,
		take_profit_price, leverage, status, risk_decision, risk_reason, created_at, updated_at
	FROM orders
	WHERE exchange = ? AND account_id = ? AND client_order_id = ?;
	`, exchange, accountID, clientOrderID)
	return scanOrder(row)
}

func listRiskEventsByOrderID(ctx context.Context, tx *sql.Tx, orderID int64) ([]RiskEventRecord, error) {
	rows, err := tx.QueryContext(ctx, `
	SELECT id, account_id, strategy_id, order_id, client_order_id, event_time,
		severity, event_type, symbol, decision, message, context_json, created_at
	FROM risk_events
	WHERE order_id = ?
	ORDER BY id ASC;
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]RiskEventRecord, 0)
	for rows.Next() {
		event, err := scanRiskEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOrder(scanner scanner) (OrderRecord, error) {
	var record OrderRecord
	var marketType string
	var side string
	var reduceOnly int
	var status string
	var decision string
	if err := scanner.Scan(
		&record.ID,
		&record.AccountID,
		&record.StrategyID,
		&record.Exchange,
		&marketType,
		&record.Symbol,
		&record.ClientOrderID,
		&side,
		&record.OrderType,
		&record.TimeInForce,
		&reduceOnly,
		&record.Price,
		&record.Quantity,
		&record.StopPrice,
		&record.TakeProfitPrice,
		&record.Leverage,
		&status,
		&decision,
		&record.RiskReason,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OrderRecord{}, err
		}
		return OrderRecord{}, err
	}
	record.MarketType = risk.MarketType(marketType)
	record.Side = risk.Side(side)
	record.ReduceOnly = reduceOnly != 0
	record.Status = OrderStatus(status)
	record.RiskDecision = risk.Decision(decision)
	return record, nil
}

func scanRiskEvent(scanner scanner) (RiskEventRecord, error) {
	var record RiskEventRecord
	var severity string
	var decision string
	if err := scanner.Scan(
		&record.ID,
		&record.AccountID,
		&record.StrategyID,
		&record.OrderID,
		&record.ClientOrderID,
		&record.EventTime,
		&severity,
		&record.EventType,
		&record.Symbol,
		&decision,
		&record.Message,
		&record.ContextJSON,
		&record.CreatedAt,
	); err != nil {
		return RiskEventRecord{}, err
	}
	record.Severity = risk.Severity(severity)
	record.Decision = risk.Decision(decision)
	return record, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
