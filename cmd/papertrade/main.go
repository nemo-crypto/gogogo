package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
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
		dsn           = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID     = flag.String("account", "paper", "paper account id")
		strategyID    = flag.String("strategy", "sma-paper", "strategy id")
		exchange      = flag.String("exchange", "binance", "exchange")
		market        = flag.String("market", "spot", "market type")
		symbol        = flag.String("symbol", "BTCUSDT", "symbol")
		interval      = flag.String("interval", "1h", "interval")
		start         = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time")
		end           = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time")
		strategyType  = flag.String("strategy-type", "sma", "strategy type: sma or scalp-tpsl")
		fast          = flag.Int("fast", 12, "fast SMA")
		slow          = flag.Int("slow", 48, "slow SMA")
		takeProfitPct = flag.Float64("take-profit-pct", 0.8, "take profit pct for scalp-tpsl")
		stopLossPct   = flag.Float64("stop-loss-pct", 0.4, "stop loss pct for scalp-tpsl")
		cooldownBars  = flag.Int("cooldown-bars", 0, "cooldown bars after scalp-tpsl exit")
		feeRate       = flag.Float64("fee-rate", 0.001, "fee rate per trade side")
		slippageRate  = flag.Float64("slippage-rate", 0.0005, "slippage rate per trade side")
		equity        = flag.Float64("equity", 10000, "paper account equity")
		quantity      = flag.Float64("quantity", 0.01, "paper order quantity")
		leverage      = flag.Float64("leverage", 1, "paper position leverage")
		watch         = flag.Bool("watch", false, "keep running paper strategy on latest local market data")
		pollEvery     = flag.Duration("poll-interval", 15*time.Second, "poll interval when -watch is enabled")
	)
	flag.Parse()
	actualStrategyID := *strategyID
	if actualStrategyID == "sma-paper" && strings.EqualFold(*strategyType, "scalp-tpsl") {
		actualStrategyID = "scalp-tpsl-paper"
	}

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

	config := paperRunConfig{
		AccountID:       *accountID,
		StrategyID:      actualStrategyID,
		Exchange:        *exchange,
		MarketType:      *market,
		Symbol:          *symbol,
		Interval:        *interval,
		Start:           startTime,
		End:             endTime,
		StrategyType:    *strategyType,
		FastWindow:      *fast,
		SlowWindow:      *slow,
		TakeProfitPct:   *takeProfitPct,
		StopLossPct:     *stopLossPct,
		CooldownBars:    *cooldownBars,
		FeeRate:         *feeRate,
		SlippageRate:    *slippageRate,
		Equity:          *equity,
		Quantity:        *quantity,
		Leverage:        *leverage,
		Watch:           *watch,
		PollInterval:    *pollEvery,
		LookbackCandles: 120,
	}

	if config.Watch {
		return watchPaperStrategy(context.Background(), db, config)
	}

	return runPaperStrategyOnce(ctx, db, config)
}

type paperRunConfig struct {
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	Interval        string
	Start           time.Time
	End             time.Time
	StrategyType    string
	FastWindow      int
	SlowWindow      int
	TakeProfitPct   float64
	StopLossPct     float64
	CooldownBars    int
	FeeRate         float64
	SlippageRate    float64
	Equity          float64
	Quantity        float64
	Leverage        float64
	Watch           bool
	PollInterval    time.Duration
	LookbackCandles int
}

func watchPaperStrategy(ctx context.Context, db *sql.DB, config paperRunConfig) error {
	if config.PollInterval <= 0 {
		config.PollInterval = 15 * time.Second
	}
	log.Printf("papertrade watch started: strategy=%s symbol=%s interval=%s poll_interval=%s", config.StrategyID, config.Symbol, config.Interval, config.PollInterval)
	for {
		current := config
		current.End = time.Now().UTC()
		current.Start = current.End.Add(-paperLookbackDuration(config.Interval, config.LookbackCandles))
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := runPaperStrategyOnce(runCtx, db, current); err != nil {
			log.Printf("papertrade watch error: %v", err)
		}
		cancel()

		timer := time.NewTimer(config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func runPaperStrategyOnce(ctx context.Context, db *sql.DB, config paperRunConfig) error {
	parsedMarket := marketdata.MarketType(config.MarketType)
	mdRepo := marketdata.NewSQLiteRepository(db)
	candles, err := mdRepo.ListCandles(ctx, marketdata.CandleQuery{
		Exchange:   config.Exchange,
		MarketType: parsedMarket,
		Symbol:     config.Symbol,
		Interval:   config.Interval,
		Start:      config.Start,
		End:        config.End,
		Limit:      10000,
	})
	if err != nil {
		return fmt.Errorf("list candles: %w", err)
	}
	result, err := runPaperBacktest(candles, paperStrategyConfig{
		StrategyType:  config.StrategyType,
		MarketType:    config.MarketType,
		FastWindow:    config.FastWindow,
		SlowWindow:    config.SlowWindow,
		TakeProfitPct: config.TakeProfitPct,
		StopLossPct:   config.StopLossPct,
		CooldownBars:  config.CooldownBars,
		FeeRate:       config.FeeRate,
		SlippageRate:  config.SlippageRate,
	})
	if err != nil {
		return fmt.Errorf("run paper strategy: %w", err)
	}

	backtestRepo := backtest.NewSQLiteRepository(db)
	backtestRunID, err := backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
		Exchange:   config.Exchange,
		MarketType: string(parsedMarket),
		Config: backtest.SMAConfig{
			FastWindow: config.FastWindow,
			SlowWindow: config.SlowWindow,
			FeeRate:    paperBacktestFeeRate(config.StrategyType, config.FeeRate, config.SlippageRate),
		},
		Result: result,
	})
	if err != nil {
		return fmt.Errorf("save paper backtest run: %w", err)
	}

	strategyRepo := strategy.NewSQLiteRepository(db)
	configJSON, err := marshalJSON(map[string]any{
		"strategy_type":   normalizedStrategyType(config.StrategyType),
		"fast":            config.FastWindow,
		"slow":            config.SlowWindow,
		"symbol":          config.Symbol,
		"take_profit_pct": config.TakeProfitPct,
		"stop_loss_pct":   config.StopLossPct,
		"cooldown_bars":   config.CooldownBars,
		"fee_rate":        config.FeeRate,
		"slippage_rate":   config.SlippageRate,
		"market_type":     config.MarketType,
	})
	if err != nil {
		return fmt.Errorf("encode strategy config: %w", err)
	}
	latestPrice, latestTime, err := latestCandlePrice(candles)
	if err != nil {
		return err
	}

	portfolioRepo := portfolio.NewSQLiteRepository(db)
	paperState, err := settlePaperPosition(ctx, portfolioRepo, paperSettleRequest{
		AccountID:       config.AccountID,
		StrategyID:      config.StrategyID,
		Exchange:        config.Exchange,
		MarketType:      config.MarketType,
		Symbol:          config.Symbol,
		MarkPrice:       latestPrice,
		MarkTime:        latestTime,
		Equity:          config.Equity,
		Leverage:        config.Leverage,
		TakeProfitPct:   config.TakeProfitPct,
		StopLossPct:     config.StopLossPct,
		FastWindow:      config.FastWindow,
		SlowWindow:      config.SlowWindow,
		FeeRate:         config.FeeRate,
		SlippageRate:    config.SlippageRate,
		StrategyType:    config.StrategyType,
		Candles:         candles,
		AllowPaperState: true,
	})
	if err != nil {
		return err
	}

	runID, err := strategyRepo.StartRun(ctx, strategy.RunRecord{
		StrategyID: config.StrategyID,
		Mode:       "paper",
		ConfigJSON: configJSON,
	})
	if err != nil {
		return fmt.Errorf("start strategy run: %w", err)
	}
	signal := paperSignalAction(candles, result, paperStrategyConfig{
		StrategyType:  config.StrategyType,
		MarketType:    config.MarketType,
		FastWindow:    config.FastWindow,
		SlowWindow:    config.SlowWindow,
		TakeProfitPct: config.TakeProfitPct,
		StopLossPct:   config.StopLossPct,
	})
	action := signal.Action
	if paperState.Open {
		action = strategy.SignalHold
	}
	rawFeaturesJSON, err := marshalJSON(map[string]any{
		"strategy_name":       result.StrategyName,
		"total_return_pct":    result.TotalReturnPct,
		"excess_return_pct":   result.ExcessReturnPct,
		"max_drawdown_pct":    result.MaxDrawdownPct,
		"trade_count":         len(result.Trades),
		"win_rate_pct":        result.WinRatePct,
		"backtest_run_id":     backtestRunID,
		"take_profit_pct":     config.TakeProfitPct,
		"stop_loss_pct":       config.StopLossPct,
		"latest_signal_input": normalizedStrategyType(config.StrategyType),
		"position_side":       signal.PositionSide,
	})
	if err != nil {
		return fmt.Errorf("encode signal features: %w", err)
	}
	if _, err := strategyRepo.SaveSignal(ctx, strategy.SignalRecord{
		StrategyID:      config.StrategyID,
		RunID:           runID,
		Exchange:        config.Exchange,
		MarketType:      config.MarketType,
		Symbol:          config.Symbol,
		Action:          action,
		Confidence:      confidence(result.ExcessReturnPct),
		Reason:          paperSignalReason(config.StrategyType),
		RawFeaturesJSON: rawFeaturesJSON,
	}); err != nil {
		return fmt.Errorf("save signal: %w", err)
	}
	metricsJSON, err := marshalJSON(map[string]any{
		"total_return_pct":  result.TotalReturnPct,
		"excess_return_pct": result.ExcessReturnPct,
		"trades":            len(result.Trades),
		"win_rate_pct":      result.WinRatePct,
		"take_profit_pct":   config.TakeProfitPct,
		"stop_loss_pct":     config.StopLossPct,
	})
	if err != nil {
		return fmt.Errorf("encode performance metrics: %w", err)
	}
	if _, err := strategyRepo.SavePerformanceSnapshot(ctx, strategy.PerformanceSnapshot{
		StrategyID:  config.StrategyID,
		RunID:       runID,
		Equity:      paperState.Equity,
		PnL:         paperState.TotalPnL,
		DrawdownPct: result.MaxDrawdownPct,
		Exposure:    result.FinalEquity,
		MetricsJSON: metricsJSON,
	}); err != nil {
		return fmt.Errorf("save performance: %w", err)
	}

	if action != strategy.SignalHold {
		side, positionSide, stopPrice, takeProfitPrice, err := paperOrderPlan(action, latestPrice, config.TakeProfitPct, config.StopLossPct)
		if err != nil {
			return err
		}
		orderRepo := execution.NewSQLiteRepository(db)
		dryRun, err := orderRepo.RecordDryRunOrder(ctx, execution.DryRunRequest{
			Account: risk.AccountSnapshot{
				AccountID: config.AccountID,
				Equity:    config.Equity,
			},
			Order: risk.OrderIntent{
				Exchange:   config.Exchange,
				MarketType: risk.MarketType(config.MarketType),
				Symbol:     config.Symbol,
				Side:       side,
				Price:      latestPrice,
				Quantity:   config.Quantity,
				StopPrice:  stopPrice,
				Leverage:   config.Leverage,
			},
			StrategyID:      config.StrategyID,
			ClientOrderID:   fmt.Sprintf("paper-%s-%d", config.Symbol, time.Now().UTC().UnixNano()),
			OrderType:       "market",
			TimeInForce:     "IOC",
			TakeProfitPrice: takeProfitPrice,
		})
		if err != nil {
			return fmt.Errorf("record paper dry-run order: %w", err)
		}
		opened, err := openPaperPosition(ctx, portfolioRepo, paperOpenRequest{
			AccountID:       config.AccountID,
			StrategyID:      config.StrategyID,
			Exchange:        config.Exchange,
			MarketType:      config.MarketType,
			Symbol:          config.Symbol,
			PositionSide:    positionSide,
			Quantity:        config.Quantity,
			EntryPrice:      latestPrice,
			TakeProfitPrice: takeProfitPrice,
			StopLossPrice:   stopPrice,
			Equity:          config.Equity,
			Leverage:        config.Leverage,
			FeeRate:         config.FeeRate,
			SlippageRate:    config.SlippageRate,
			OpenedAt:        latestTime,
		})
		if err != nil {
			return err
		}
		fmt.Printf("paper_order_id=%d status=%s decision=%s stop=%.8f take_profit=%.8f\n", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
		fmt.Printf("paper_position_id=%d status=open entry=%.8f mark=%.8f pnl=%.8f\n", opened.ID, opened.EntryPrice, opened.MarkPrice, paperPositionNetPnL(opened, latestPrice, config.FeeRate, config.SlippageRate))
	}

	fmt.Printf("paper_run_id=%d backtest_run_id=%d strategy=%s symbol=%s return_pct=%.4f excess_pct=%.4f drawdown_pct=%.4f trades=%d\n", runID, backtestRunID, config.StrategyID, config.Symbol, result.TotalReturnPct, result.ExcessReturnPct, result.MaxDrawdownPct, len(result.Trades))
	return nil
}

type paperStrategyConfig struct {
	StrategyType  string
	FastWindow    int
	SlowWindow    int
	TakeProfitPct float64
	StopLossPct   float64
	CooldownBars  int
	FeeRate       float64
	SlippageRate  float64
	MarketType    string
}

type paperSettleRequest struct {
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	StrategyType    string
	MarkPrice       float64
	MarkTime        time.Time
	Equity          float64
	Leverage        float64
	TakeProfitPct   float64
	StopLossPct     float64
	FastWindow      int
	SlowWindow      int
	FeeRate         float64
	SlippageRate    float64
	Candles         []marketdata.Candle
	AllowPaperState bool
}

type paperAccountState struct {
	Open      bool
	Equity    float64
	TotalPnL  float64
	Position  portfolio.PaperPositionRecord
	CloseNote string
}

type paperOpenRequest struct {
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	PositionSide    string
	Quantity        float64
	EntryPrice      float64
	TakeProfitPrice float64
	StopLossPrice   float64
	Equity          float64
	Leverage        float64
	FeeRate         float64
	SlippageRate    float64
	OpenedAt        time.Time
}

func settlePaperPosition(ctx context.Context, repo *portfolio.SQLiteRepository, request paperSettleRequest) (paperAccountState, error) {
	state := paperAccountState{Equity: request.Equity}
	position, err := repo.LatestOpenPaperPosition(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := savePaperAccountSnapshots(ctx, repo, request.AccountID, request.Exchange, request.MarketType, request.Symbol, request.Equity, request.Leverage, portfolio.PaperPositionRecord{}, request.MarkPrice, request.MarkTime, request.FeeRate, request.SlippageRate); err != nil {
				return paperAccountState{}, err
			}
			return state, nil
		}
		return paperAccountState{}, fmt.Errorf("load open paper position: %w", err)
	}

	exitReason := paperExitReason(position, request.MarkPrice)
	if exitReason == "" {
		exitReason = paperTrendExitReason(position, request.Candles, paperStrategyConfig{
			StrategyType: request.StrategyType,
			MarketType:   request.MarketType,
			FastWindow:   request.FastWindow,
			SlowWindow:   request.SlowWindow,
		})
	}
	if exitReason != "" {
		realizedPnL := paperPositionNetPnL(position, request.MarkPrice, request.FeeRate, request.SlippageRate)
		closed, err := repo.ClosePaperPositionWithRealizedPnL(ctx, position.ID, request.MarkPrice, request.MarkTime, realizedPnL)
		if err != nil {
			return paperAccountState{}, fmt.Errorf("close paper position: %w", err)
		}
		state.Position = closed
		state.TotalPnL = closed.RealizedPnL
		state.Equity = request.Equity + state.TotalPnL
		state.CloseNote = exitReason
		if err := savePaperAccountSnapshots(ctx, repo, request.AccountID, request.Exchange, request.MarketType, request.Symbol, state.Equity, request.Leverage, portfolio.PaperPositionRecord{}, request.MarkPrice, request.MarkTime, request.FeeRate, request.SlippageRate); err != nil {
			return paperAccountState{}, err
		}
		fmt.Printf("paper_position_closed id=%d reason=%s exit=%.8f realized_pnl=%.8f\n", closed.ID, exitReason, request.MarkPrice, closed.RealizedPnL)
		return state, nil
	}

	if err := repo.UpdatePaperPositionMark(ctx, position.ID, request.MarkPrice); err != nil {
		return paperAccountState{}, fmt.Errorf("update paper position mark: %w", err)
	}
	position.MarkPrice = request.MarkPrice
	state.Open = true
	state.Position = position
	state.TotalPnL = paperPositionNetPnL(position, request.MarkPrice, request.FeeRate, request.SlippageRate)
	state.Equity = request.Equity + state.TotalPnL
	if err := savePaperAccountSnapshots(ctx, repo, request.AccountID, request.Exchange, request.MarketType, request.Symbol, state.Equity, request.Leverage, position, request.MarkPrice, request.MarkTime, request.FeeRate, request.SlippageRate); err != nil {
		return paperAccountState{}, err
	}
	return state, nil
}

func openPaperPosition(ctx context.Context, repo *portfolio.SQLiteRepository, request paperOpenRequest) (portfolio.PaperPositionRecord, error) {
	id, err := repo.OpenPaperPosition(ctx, portfolio.PaperPositionRecord{
		AccountID:       request.AccountID,
		StrategyID:      request.StrategyID,
		Exchange:        request.Exchange,
		MarketType:      request.MarketType,
		Symbol:          request.Symbol,
		PositionSide:    request.PositionSide,
		Quantity:        request.Quantity,
		EntryPrice:      request.EntryPrice,
		MarkPrice:       request.EntryPrice,
		TakeProfitPrice: request.TakeProfitPrice,
		StopLossPrice:   request.StopLossPrice,
		OpenedAt:        request.OpenedAt,
	})
	if err != nil {
		return portfolio.PaperPositionRecord{}, fmt.Errorf("open paper position: %w", err)
	}
	position, err := repo.LatestOpenPaperPosition(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		return portfolio.PaperPositionRecord{}, fmt.Errorf("reload opened paper position %d: %w", id, err)
	}
	openPnL := paperPositionNetPnL(position, request.EntryPrice, request.FeeRate, request.SlippageRate)
	if err := savePaperAccountSnapshots(ctx, repo, request.AccountID, request.Exchange, request.MarketType, request.Symbol, request.Equity+openPnL, request.Leverage, position, request.EntryPrice, request.OpenedAt, request.FeeRate, request.SlippageRate); err != nil {
		return portfolio.PaperPositionRecord{}, err
	}
	return position, nil
}

func savePaperAccountSnapshots(ctx context.Context, repo *portfolio.SQLiteRepository, accountID string, exchange string, marketType string, symbol string, equity float64, leverage float64, position portfolio.PaperPositionRecord, markPrice float64, snapshotTime time.Time, feeRate float64, slippageRate float64) error {
	if snapshotTime.IsZero() {
		snapshotTime = time.Now().UTC()
	}
	locked := 0.0
	if position.ID != 0 {
		locked = math.Abs(position.Quantity*markPrice) / math.Max(leverage, 1)
	}
	free := equity - locked
	if free < 0 {
		free = 0
	}
	if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
		AccountID:    accountID,
		Exchange:     exchange,
		Asset:        "USDT",
		Free:         free,
		Locked:       locked,
		Total:        equity,
		USDValue:     equity,
		SnapshotTime: snapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper balance snapshot: %w", err)
	}

	if position.ID != 0 {
		pnl := paperPositionNetPnL(position, markPrice, feeRate, slippageRate)
		if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
			AccountID:     accountID,
			Exchange:      exchange,
			MarketType:    marketType,
			Symbol:        symbol,
			PositionSide:  position.PositionSide,
			Quantity:      position.Quantity,
			EntryPrice:    position.EntryPrice,
			MarkPrice:     markPrice,
			Leverage:      leverage,
			MarginMode:    "paper",
			UnrealizedPnL: pnl,
			Notional:      math.Abs(position.Quantity * markPrice),
			SnapshotTime:  snapshotTime,
		}); err != nil {
			return fmt.Errorf("save paper position snapshot: %w", err)
		}
	} else {
		for _, side := range []string{"long", "short"} {
			if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
				AccountID:    accountID,
				Exchange:     exchange,
				MarketType:   marketType,
				Symbol:       symbol,
				PositionSide: side,
				Quantity:     0,
				EntryPrice:   0,
				MarkPrice:    markPrice,
				Leverage:     leverage,
				MarginMode:   "paper",
				SnapshotTime: snapshotTime,
			}); err != nil {
				return fmt.Errorf("save flat paper %s position snapshot: %w", side, err)
			}
		}
	}

	if _, err := repo.SaveMarginSnapshot(ctx, portfolio.MarginSnapshot{
		AccountID:        accountID,
		Exchange:         exchange,
		MarketType:       marketType,
		Equity:           equity,
		MarginBalance:    equity,
		AvailableBalance: free,
		SnapshotTime:     snapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper margin snapshot: %w", err)
	}
	return nil
}

func paperExitReason(position portfolio.PaperPositionRecord, markPrice float64) string {
	if strings.EqualFold(position.PositionSide, "short") {
		if position.TakeProfitPrice > 0 && markPrice <= position.TakeProfitPrice {
			return "take_profit"
		}
		if position.StopLossPrice > 0 && markPrice >= position.StopLossPrice {
			return "stop_loss"
		}
		return ""
	}
	if position.TakeProfitPrice > 0 && markPrice >= position.TakeProfitPrice {
		return "take_profit"
	}
	if position.StopLossPrice > 0 && markPrice <= position.StopLossPrice {
		return "stop_loss"
	}
	return ""
}

func paperTrendExitReason(position portfolio.PaperPositionRecord, candles []marketdata.Candle, config paperStrategyConfig) string {
	if normalizedStrategyType(config.StrategyType) != "scalp-tpsl" {
		return ""
	}
	fastAverage, slowAverage, ok := latestAverages(candles, config.FastWindow, config.SlowWindow)
	if !ok {
		return ""
	}
	if strings.EqualFold(position.PositionSide, "short") {
		if fastAverage > slowAverage {
			return "trend_reversal"
		}
		return ""
	}
	if fastAverage < slowAverage {
		return "trend_reversal"
	}
	return ""
}

func paperOrderPlan(action strategy.SignalAction, price float64, takeProfitPct float64, stopLossPct float64) (risk.Side, string, float64, float64, error) {
	if price <= 0 {
		return "", "", 0, 0, errors.New("price must be positive")
	}
	switch action {
	case strategy.SignalBuy:
		return risk.SideBuy, "long", price * (1 - stopLossPct/100), price * (1 + takeProfitPct/100), nil
	case strategy.SignalShort:
		return risk.SideSell, "short", price * (1 + stopLossPct/100), price * (1 - takeProfitPct/100), nil
	default:
		return "", "", 0, 0, fmt.Errorf("unsupported paper action %q", action)
	}
}

func paperPositionPnL(position portfolio.PaperPositionRecord, markPrice float64) float64 {
	if strings.EqualFold(position.PositionSide, "short") {
		return (position.EntryPrice - markPrice) * math.Abs(position.Quantity)
	}
	return (markPrice - position.EntryPrice) * math.Abs(position.Quantity)
}

func paperPositionNetPnL(position portfolio.PaperPositionRecord, markPrice float64, feeRate float64, slippageRate float64) float64 {
	grossPnL := paperPositionPnL(position, markPrice)
	costRate := math.Max(feeRate, 0) + math.Max(slippageRate, 0)
	if costRate == 0 {
		return grossPnL
	}
	entryCost := paperTradeCost(position.EntryPrice, position.Quantity, costRate)
	exitCost := paperTradeCost(markPrice, position.Quantity, costRate)
	return grossPnL - entryCost - exitCost
}

func paperTradeCost(price float64, quantity float64, costRate float64) float64 {
	if price <= 0 || quantity == 0 || costRate <= 0 {
		return 0
	}
	return math.Abs(price*quantity) * costRate
}

func latestCandlePrice(candles []marketdata.Candle) (float64, time.Time, error) {
	if len(candles) == 0 {
		return 0, time.Time{}, backtest.ErrNotEnoughData
	}
	last := candles[len(candles)-1]
	price, err := strconv.ParseFloat(last.Close, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("parse latest close price: %w", err)
	}
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, time.Time{}, fmt.Errorf("invalid latest close price %s", last.Close)
	}
	return price, last.OpenTime, nil
}

func paperLookbackDuration(interval string, candles int) time.Duration {
	if candles <= 0 {
		candles = 120
	}
	step, err := marketdata.IntervalDuration(interval)
	if err != nil {
		return 2 * time.Hour
	}
	return step * time.Duration(candles)
}

func runPaperBacktest(candles []marketdata.Candle, config paperStrategyConfig) (backtest.Result, error) {
	switch normalizedStrategyType(config.StrategyType) {
	case "scalp-tpsl":
		return backtest.RunScalpTPSL(candles, backtest.ScalpTPSLConfig{
			FastWindow:    config.FastWindow,
			SlowWindow:    config.SlowWindow,
			TakeProfitPct: config.TakeProfitPct,
			StopLossPct:   config.StopLossPct,
			CooldownBars:  config.CooldownBars,
			FeeRate:       config.FeeRate,
			SlippageRate:  config.SlippageRate,
			AllowShort:    strings.EqualFold(config.MarketType, "perpetual"),
		})
	case "sma":
		return backtest.RunSMACrossover(candles, backtest.SMAConfig{
			FastWindow: config.FastWindow,
			SlowWindow: config.SlowWindow,
			FeeRate:    config.FeeRate,
		})
	default:
		return backtest.Result{}, fmt.Errorf("unsupported strategy type %q", config.StrategyType)
	}
}

func paperBacktestFeeRate(strategyType string, feeRate float64, slippageRate float64) float64 {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		return feeRate + slippageRate
	}
	return feeRate
}

type paperSignal struct {
	Action       strategy.SignalAction
	PositionSide string
}

func paperSignalAction(candles []marketdata.Candle, result backtest.Result, config paperStrategyConfig) paperSignal {
	switch normalizedStrategyType(config.StrategyType) {
	case "scalp-tpsl":
		return latestScalpSignal(candles, config)
	default:
		if result.TotalReturnPct > 0 {
			return paperSignal{Action: strategy.SignalBuy, PositionSide: "long"}
		}
		return paperSignal{Action: strategy.SignalHold}
	}
}

func latestScalpSignal(candles []marketdata.Candle, config paperStrategyConfig) paperSignal {
	if config.FastWindow <= 0 || config.SlowWindow <= 0 || config.FastWindow >= config.SlowWindow || len(candles) < config.SlowWindow+1 {
		return paperSignal{Action: strategy.SignalHold}
	}
	closes := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil || closePrice <= 0 || math.IsNaN(closePrice) || math.IsInf(closePrice, 0) {
			return paperSignal{Action: strategy.SignalHold}
		}
		closes = append(closes, closePrice)
	}
	latest := len(closes) - 1
	fastAverage := average(closes[latest-config.FastWindow+1 : latest+1])
	slowAverage := average(closes[latest-config.SlowWindow+1 : latest+1])
	if fastAverage > slowAverage && closes[latest] > closes[latest-1] {
		return paperSignal{Action: strategy.SignalBuy, PositionSide: "long"}
	}
	if strings.EqualFold(config.MarketType, "perpetual") && fastAverage < slowAverage && closes[latest] < closes[latest-1] {
		return paperSignal{Action: strategy.SignalShort, PositionSide: "short"}
	}
	return paperSignal{Action: strategy.SignalHold}
}

func latestAverages(candles []marketdata.Candle, fastWindow int, slowWindow int) (float64, float64, bool) {
	if fastWindow <= 0 || slowWindow <= 0 || fastWindow >= slowWindow || len(candles) < slowWindow {
		return 0, 0, false
	}
	closes := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := strconv.ParseFloat(candle.Close, 64)
		if err != nil || closePrice <= 0 || math.IsNaN(closePrice) || math.IsInf(closePrice, 0) {
			return 0, 0, false
		}
		closes = append(closes, closePrice)
	}
	latest := len(closes) - 1
	fastAverage := average(closes[latest-fastWindow+1 : latest+1])
	slowAverage := average(closes[latest-slowWindow+1 : latest+1])
	return fastAverage, slowAverage, true
}

func average(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func normalizedStrategyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "scalp", "scalping", "scalp-tpsl", "tpsl":
		return "scalp-tpsl"
	default:
		return "sma"
	}
}

func paperSignalReason(strategyType string) string {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		return "paper_scalp_tpsl_latest_momentum"
	}
	return "paper_sma_snapshot"
}

func marshalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
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
