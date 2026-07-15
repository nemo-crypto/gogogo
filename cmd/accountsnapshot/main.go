package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gogogo/internal/config"
	exchangemodel "gogogo/internal/exchange"
	"gogogo/internal/exchange/onebullex"
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
		dsn              = flag.String("dsn", env("DATABASE_DSN", "data.db"), "sqlite database path")
		accountID        = flag.String("account", "research", "account id")
		exchange         = flag.String("exchange", env("EXCHANGE_NAME", onebullex.ExchangeName), "exchange name")
		market           = flag.String("market", "perpetual", "market type")
		syncLive         = flag.Bool("sync-live", false, "sync readonly account snapshot from exchange API")
		watch            = flag.Bool("watch", false, "keep syncing live account snapshots")
		pollEvery        = flag.Duration("poll-interval", 1*time.Minute, "poll interval when -watch is enabled")
		syncConfigs      = flag.Bool("sync-position-configs", true, "sync position config metadata with live account snapshots")
		asset            = flag.String("asset", "USDT", "balance asset")
		free             = flag.Float64("free", 0, "free balance")
		locked           = flag.Float64("locked", 0, "locked balance")
		usdValue         = flag.Float64("usd-value", 0, "balance USD value")
		symbol           = flag.String("symbol", "", "optional position symbol")
		positionSide     = flag.String("position-side", "long", "position side")
		positionModel    = flag.String("position-model", "", "position model: AGGREGATION for one-way or DISAGGREGATION for hedge")
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
	exchangeName := normalizeExchangeName(*exchange)

	if *syncLive {
		if *watch {
			return watchLiveAccountSnapshot(context.Background(), repo, *accountID, exchangeName, *market, *symbol, *pollEvery, *syncConfigs)
		}
		return syncLiveAccountSnapshot(ctx, repo, *accountID, exchangeName, *market, *symbol, *syncConfigs)
	}

	if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
		AccountID:    *accountID,
		Exchange:     exchangeName,
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
			Exchange:         exchangeName,
			MarketType:       *market,
			Symbol:           *symbol,
			PositionSide:     *positionSide,
			PositionModel:    *positionModel,
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
		Exchange:          exchangeName,
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

	log.Printf("account_snapshot_saved account=%s exchange=%s time=%s", *accountID, exchangeName, now.Format(time.RFC3339))
	return nil
}

func env(key string, fallback string) string {
	return config.Env(key, fallback)
}

func syncLiveAccountSnapshot(ctx context.Context, repo *portfolio.SQLiteRepository, accountID string, exchangeName string, market string, symbol string, syncConfigs bool) error {
	if exchangeName != onebullex.ExchangeName {
		return fmt.Errorf("live snapshot currently supports %s only", onebullex.ExchangeName)
	}
	client := onebullex.NewClient(
		onebullex.WithBaseURL(env("ONEBULLEX_BASE_URL", "")),
		onebullex.WithCredentials(env("ONEBULLEX_API_KEY", ""), env("ONEBULLEX_SECRET_KEY", "")),
	)
	snapshot, err := client.AccountSnapshot(ctx, accountID)
	if err != nil {
		return fmt.Errorf("fetch onebullex account snapshot: %w", err)
	}
	if err := saveExchangeSnapshot(ctx, repo, snapshot, market); err != nil {
		return err
	}
	configs := 0
	if syncConfigs {
		configs, err = syncOneBullExPositionConfigs(ctx, repo, client, accountID, snapshot, symbol)
		if err != nil {
			return err
		}
	}
	log.Printf("live_account_snapshot_saved account=%s exchange=%s balances=%d positions=%d time=%s readonly=%t",
		snapshot.AccountID, snapshot.Exchange, len(snapshot.Balances), len(snapshot.Positions), snapshot.SnapshotTime.Format(time.RFC3339), snapshot.ReadOnly)
	if configs > 0 {
		log.Printf("live_position_configs_saved account=%s exchange=%s configs=%d", snapshot.AccountID, snapshot.Exchange, configs)
	}
	return nil
}

func watchLiveAccountSnapshot(ctx context.Context, repo *portfolio.SQLiteRepository, accountID string, exchangeName string, market string, symbol string, pollEvery time.Duration, syncConfigs bool) error {
	if pollEvery <= 0 {
		pollEvery = time.Minute
	}
	log.Printf("live account snapshot watch started: account=%s exchange=%s symbol=%s poll_interval=%s", accountID, exchangeName, symbol, pollEvery)
	for {
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := syncLiveAccountSnapshot(runCtx, repo, accountID, exchangeName, market, symbol, syncConfigs); err != nil {
			log.Printf("live account snapshot watch error: %v", err)
		}
		cancel()

		timer := time.NewTimer(pollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func saveExchangeSnapshot(ctx context.Context, repo *portfolio.SQLiteRepository, snapshot exchangemodel.AccountSnapshot, market string) error {
	for _, balance := range snapshot.Balances {
		if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
			AccountID:             snapshot.AccountID,
			Exchange:              snapshot.Exchange,
			Asset:                 balance.Asset,
			Free:                  parseFloat(balance.Free),
			Locked:                parseFloat(balance.Locked),
			Total:                 parseFloat(balance.Total),
			USDValue:              parseFloat(firstNonEmpty(balance.USDValue, balance.Total)),
			WalletBalance:         balance.WalletBalance,
			OpenOrderMarginFrozen: balance.OpenOrderMarginFrozen,
			IsolatedMargin:        balance.IsolatedMargin,
			CrossedMargin:         balance.CrossedMargin,
			AvailableBalance:      balance.AvailableBalance,
			Bonus:                 balance.Bonus,
			RawJSON:               balance.RawJSON,
			SnapshotTime:          snapshot.SnapshotTime,
		}); err != nil {
			return fmt.Errorf("save live balance %s: %w", balance.Asset, err)
		}
	}

	totalEquity, available := balanceTotals(snapshot.Balances)
	for _, position := range snapshot.Positions {
		quantity := parseFloat(position.Quantity)
		if quantity == 0 {
			continue
		}
		if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
			AccountID:          snapshot.AccountID,
			Exchange:           snapshot.Exchange,
			MarketType:         market,
			Symbol:             position.Symbol,
			ExchangePositionID: position.PositionID,
			PositionSide:       position.PositionSide,
			PositionModel:      position.PositionModel,
			Quantity:           quantity,
			CloseOrderSize:     position.CloseOrderSize,
			AvailableCloseSize: position.AvailableClose,
			EntryPrice:         parseFloat(position.EntryPrice),
			MarkPrice:          parseFloat(position.MarkPrice),
			LiquidationPrice:   parseFloat(position.LiquidationPrice),
			Leverage:           float64(position.Leverage),
			MarginMode:         position.MarginMode,
			IsolatedMargin:     position.IsolatedMargin,
			OpenOrderMargin:    position.OrderMargin,
			RealizedProfit:     position.RealizedProfit,
			AutoMargin:         position.AutoMargin,
			ContractSize:       position.ContractSize,
			UnrealizedPnL:      parseFloat(position.UnrealizedPnL),
			RawJSON:            position.RawJSON,
			SnapshotTime:       snapshot.SnapshotTime,
		}); err != nil {
			return fmt.Errorf("save live position %s %s: %w", position.Symbol, position.PositionSide, err)
		}
	}

	if _, err := repo.SaveMarginSnapshot(ctx, portfolio.MarginSnapshot{
		AccountID:        snapshot.AccountID,
		Exchange:         snapshot.Exchange,
		MarketType:       market,
		Equity:           totalEquity,
		MarginBalance:    totalEquity,
		AvailableBalance: available,
		SnapshotTime:     snapshot.SnapshotTime,
	}); err != nil {
		return fmt.Errorf("save live margin snapshot: %w", err)
	}
	return nil
}

func syncOneBullExPositionConfigs(ctx context.Context, repo *portfolio.SQLiteRepository, client *onebullex.Client, accountID string, snapshot exchangemodel.AccountSnapshot, explicitSymbol string) (int, error) {
	symbols := make(map[string]struct{})
	if strings.TrimSpace(explicitSymbol) != "" {
		symbols[strings.ToUpper(strings.TrimSpace(explicitSymbol))] = struct{}{}
	}
	for _, position := range snapshot.Positions {
		if strings.TrimSpace(position.Symbol) != "" {
			symbols[strings.ToUpper(strings.TrimSpace(position.Symbol))] = struct{}{}
		}
	}

	saved := 0
	for symbol := range symbols {
		configs, err := client.PositionConfigs(ctx, accountID, symbol)
		if err != nil {
			return saved, fmt.Errorf("fetch onebullex position configs %s: %w", symbol, err)
		}
		for _, config := range configs {
			if _, err := repo.SavePositionConfig(ctx, config); err != nil {
				return saved, fmt.Errorf("save onebullex position config %s %s: %w", config.Symbol, config.PositionSide, err)
			}
			saved++
		}
	}
	return saved, nil
}

func balanceTotals(balances []exchangemodel.Balance) (float64, float64) {
	totalEquity := 0.0
	available := 0.0
	for _, balance := range balances {
		if strings.EqualFold(balance.Asset, "USDT") {
			totalEquity += parseFloat(balance.Total)
			available += parseFloat(balance.Free)
		}
	}
	return totalEquity, available
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeExchangeName(exchangeName string) string {
	exchangeName = strings.ToLower(strings.TrimSpace(exchangeName))
	if exchangeName == "onebull" || exchangeName == "1bullex" {
		return onebullex.ExchangeName
	}
	return exchangeName
}
