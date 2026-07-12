package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dsn              = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID        = flag.String("account", "research", "account id")
		exchange         = flag.String("exchange", "binance", "exchange name")
		market           = flag.String("market", "perpetual", "market type")
		asset            = flag.String("asset", "USDT", "balance asset")
		free             = flag.Float64("free", 0, "free balance")
		locked           = flag.Float64("locked", 0, "locked balance")
		usdValue         = flag.Float64("usd-value", 0, "balance USD value")
		symbol           = flag.String("symbol", "", "optional position symbol")
		positionSide     = flag.String("position-side", "long", "position side")
		quantity         = flag.Float64("quantity", 0, "position quantity")
		entryPrice       = flag.Float64("entry-price", 0, "entry price")
		markPrice        = flag.Float64("mark-price", 0, "mark price")
		liquidationPrice = flag.Float64("liquidation-price", 0, "liquidation price")
		leverage         = flag.Float64("leverage", 1, "leverage")
		marginMode       = flag.String("margin-mode", "isolated", "margin mode")
		unrealizedPnL    = flag.Float64("unrealized-pnl", 0, "unrealized PnL")
		equity           = flag.Float64("equity", 0, "account equity")
		marginBalance    = flag.Float64("margin-balance", 0, "margin balance")
		initialMargin    = flag.Float64("initial-margin", 0, "initial margin")
		maintenance      = flag.Float64("maintenance-margin", 0, "maintenance margin")
		marginRatio      = flag.Float64("margin-ratio", 0, "margin ratio")
		available        = flag.Float64("available-balance", 0, "available balance")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	repo := portfolio.NewSQLiteRepository(db)
	now := time.Now().UTC()

	if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
		AccountID:    *accountID,
		Exchange:     *exchange,
		Asset:        *asset,
		Free:         *free,
		Locked:       *locked,
		USDValue:     *usdValue,
		SnapshotTime: now,
	}); err != nil {
		return fmt.Errorf("save balance snapshot: %w", err)
	}
	if *symbol != "" {
		if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
			AccountID:        *accountID,
			Exchange:         *exchange,
			MarketType:       *market,
			Symbol:           *symbol,
			PositionSide:     *positionSide,
			Quantity:         *quantity,
			EntryPrice:       *entryPrice,
			MarkPrice:        *markPrice,
			LiquidationPrice: *liquidationPrice,
			Leverage:         *leverage,
			MarginMode:       *marginMode,
			UnrealizedPnL:    *unrealizedPnL,
			SnapshotTime:     now,
		}); err != nil {
			return fmt.Errorf("save position snapshot: %w", err)
		}
	}
	if _, err := repo.SaveMarginSnapshot(ctx, portfolio.MarginSnapshot{
		AccountID:         *accountID,
		Exchange:          *exchange,
		MarketType:        *market,
		Equity:            *equity,
		MarginBalance:     *marginBalance,
		InitialMargin:     *initialMargin,
		MaintenanceMargin: *maintenance,
		MarginRatio:       *marginRatio,
		AvailableBalance:  *available,
		SnapshotTime:      now,
	}); err != nil {
		return fmt.Errorf("save margin snapshot: %w", err)
	}

	fmt.Printf("account_snapshot_saved account=%s exchange=%s time=%s\n", *accountID, *exchange, now.Format(time.RFC3339))
	return nil
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
