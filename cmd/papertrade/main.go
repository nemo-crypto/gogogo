package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/risk"
	"gogogo/internal/storage"
	"gogogo/internal/strategy"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dsn        = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID  = flag.String("account", "paper", "paper account id")
		strategyID = flag.String("strategy", "sma-paper", "strategy id")
		exchange   = flag.String("exchange", "binance", "exchange")
		market     = flag.String("market", "spot", "market type")
		symbol     = flag.String("symbol", "BTCUSDT", "symbol")
		interval   = flag.String("interval", "1h", "interval")
		start      = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time")
		end        = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time")
		fast       = flag.Int("fast", 12, "fast SMA")
		slow       = flag.Int("slow", 48, "slow SMA")
		equity     = flag.Float64("equity", 10000, "paper account equity")
		quantity   = flag.Float64("quantity", 0.01, "paper order quantity")
	)
	flag.Parse()

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, *end)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	parsedMarket := marketdata.MarketType(*market)
	mdRepo := marketdata.NewSQLiteRepository(db)
	candles, err := mdRepo.ListCandles(ctx, marketdata.CandleQuery{
		Exchange:   *exchange,
		MarketType: parsedMarket,
		Symbol:     *symbol,
		Interval:   *interval,
		Start:      startTime,
		End:        endTime,
		Limit:      10000,
	})
	if err != nil {
		return fmt.Errorf("list candles: %w", err)
	}
	result, err := backtest.RunSMACrossover(candles, backtest.SMAConfig{FastWindow: *fast, SlowWindow: *slow, FeeRate: 0.001})
	if err != nil {
		return fmt.Errorf("run paper strategy: %w", err)
	}

	strategyRepo := strategy.NewSQLiteRepository(db)
	runID, err := strategyRepo.StartRun(ctx, strategy.RunRecord{
		StrategyID: *strategyID,
		Mode:       "paper",
		ConfigJSON: fmt.Sprintf(`{"fast":%d,"slow":%d,"symbol":%q}`, *fast, *slow, *symbol),
	})
	if err != nil {
		return fmt.Errorf("start strategy run: %w", err)
	}
	action := strategy.SignalHold
	if result.TotalReturnPct > 0 {
		action = strategy.SignalBuy
	}
	if _, err := strategyRepo.SaveSignal(ctx, strategy.SignalRecord{
		StrategyID: *strategyID,
		RunID:      runID,
		Exchange:   *exchange,
		MarketType: *market,
		Symbol:     *symbol,
		Action:     action,
		Confidence: confidence(result.ExcessReturnPct),
		Reason:     "paper_sma_snapshot",
	}); err != nil {
		return fmt.Errorf("save signal: %w", err)
	}
	if _, err := strategyRepo.SavePerformanceSnapshot(ctx, strategy.PerformanceSnapshot{
		StrategyID:  *strategyID,
		RunID:       runID,
		Equity:      *equity * result.FinalEquity,
		PnL:         *equity * (result.FinalEquity - 1),
		DrawdownPct: result.MaxDrawdownPct,
		Exposure:    result.FinalEquity,
		MetricsJSON: fmt.Sprintf(`{"total_return_pct":%.6f,"excess_return_pct":%.6f,"trades":%d}`, result.TotalReturnPct, result.ExcessReturnPct, len(result.Trades)),
	}); err != nil {
		return fmt.Errorf("save performance: %w", err)
	}

	if len(candles) > 0 && action != strategy.SignalHold {
		last := candles[len(candles)-1]
		price, err := strconv.ParseFloat(last.Close, 64)
		if err != nil {
			return fmt.Errorf("parse latest close price: %w", err)
		}
		orderRepo := execution.NewSQLiteRepository(db)
		dryRun, err := orderRepo.RecordDryRunOrder(ctx, execution.DryRunRequest{
			Account: risk.AccountSnapshot{
				AccountID: *accountID,
				Equity:    *equity,
			},
			Order: risk.OrderIntent{
				Exchange:   *exchange,
				MarketType: risk.MarketType(*market),
				Symbol:     *symbol,
				Side:       risk.SideBuy,
				Price:      price,
				Quantity:   *quantity,
				StopPrice:  price * 0.98,
				Leverage:   1,
			},
			StrategyID:    *strategyID,
			ClientOrderID: fmt.Sprintf("paper-%s-%d", *symbol, time.Now().UTC().UnixNano()),
		})
		if err != nil {
			return fmt.Errorf("record paper dry-run order: %w", err)
		}
		fmt.Printf("paper_order_id=%d status=%s decision=%s\n", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision)
	}

	fmt.Printf("paper_run_id=%d strategy=%s symbol=%s return_pct=%.4f excess_pct=%.4f drawdown_pct=%.4f\n", runID, *strategyID, *symbol, result.TotalReturnPct, result.ExcessReturnPct, result.MaxDrawdownPct)
	return nil
}

func confidence(excess float64) float64 {
	if excess <= 0 {
		return 0.4
	}
	if excess >= 10 {
		return 0.9
	}
	return 0.5 + excess/25
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
