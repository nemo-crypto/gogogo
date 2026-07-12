package storage

import (
	"context"
	"database/sql"

	"gogogo/internal/backtest"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/strategy"
)

func InitSQLiteSchema(ctx context.Context, db *sql.DB) error {
	if err := marketdata.InitSQLiteSchema(ctx, db); err != nil {
		return err
	}
	if err := backtest.InitSQLiteSchema(ctx, db); err != nil {
		return err
	}
	if err := execution.InitSQLiteSchema(ctx, db); err != nil {
		return err
	}
	if err := portfolio.InitSQLiteSchema(ctx, db); err != nil {
		return err
	}
	return strategy.InitSQLiteSchema(ctx, db)
}
