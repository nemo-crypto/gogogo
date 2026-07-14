package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gogogo/internal/backtest"
	"gogogo/internal/exchange/onebullex"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/risk"
	"gogogo/internal/strategy"
	"math"
	"strings"
	"time"
)

type paperStrategyConfig struct {
	StrategyType         string
	FastWindow           int
	SlowWindow           int
	TakeProfitPct        float64
	StopLossPct          float64
	DynamicTPSL          bool
	TakeProfitATRMult    float64
	StopLossATRMult      float64
	MinTakeProfitPct     float64
	MaxTakeProfitPct     float64
	MinStopLossPct       float64
	MaxStopLossPct       float64
	CooldownBars         int
	FeeRate              float64
	SlippageRate         float64
	MarketType           string
	MinTrendSpreadPct    float64
	ConfirmBars          int
	ATRWindow            int
	MinATRPct            float64
	MaxATRPct            float64
	VolumeWindow         int
	MinVolumeRatio       float64
	MaxEntryExtensionPct float64
	PullbackLookback     int
	PullbackTolerancePct float64
}

type paperTPSLPercents struct {
	TakeProfitPct float64
	StopLossPct   float64
	Source        string
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
	MaintMarginPct  float64
	TakeProfitPct   float64
	StopLossPct     float64
	PriceTickSize   float64
	FastWindow      int
	SlowWindow      int
	ATRWindow       int
	BreakevenStop   bool
	BreakevenR      float64
	TrailingStop    bool
	TrailingR       float64
	TrailingATRMult float64
	FeeRate         float64
	SlippageRate    float64
	Candles         []marketdata.Candle
	AllowPaperState bool
}

type paperAccountState struct {
	Open                  bool
	Equity                float64
	AvailableBalance      float64
	TotalPnL              float64
	RealizedPnL           float64
	DailyRealizedPnL      float64
	ConsecutiveLosses     int
	UnrealizedPnL         float64
	CurrentExposure       float64
	CurrentSymbolExposure float64
	CurrentInitialMargin  float64
	CurrentMaintMargin    float64
	Position              portfolio.PaperPositionRecord
	CloseNote             string
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
	MaintMarginPct  float64
	FeeRate         float64
	SlippageRate    float64
	OpenedAt        time.Time
}

func settlePaperPosition(ctx context.Context, repo *portfolio.SQLiteRepository, request paperSettleRequest) (paperAccountState, error) {
	realizedPnL, err := repo.SumClosedPaperPositionRealizedPnL(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		return paperAccountState{}, fmt.Errorf("sum closed paper pnl: %w", err)
	}
	dayStart := paperTradingDayStart(request.MarkTime)
	dailyRealizedPnL, err := repo.SumClosedPaperPositionRealizedPnLSince(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol, dayStart)
	if err != nil {
		return paperAccountState{}, fmt.Errorf("sum daily paper pnl: %w", err)
	}
	consecutiveLosses, err := repo.CountConsecutiveClosedPaperPositionLosses(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		return paperAccountState{}, fmt.Errorf("count consecutive paper losses: %w", err)
	}
	state := paperAccountState{
		Equity:            request.Equity + realizedPnL,
		AvailableBalance:  request.Equity + realizedPnL,
		TotalPnL:          realizedPnL,
		RealizedPnL:       realizedPnL,
		DailyRealizedPnL:  dailyRealizedPnL,
		ConsecutiveLosses: consecutiveLosses,
	}
	position, err := repo.LatestOpenPaperPosition(ctx, request.AccountID, request.StrategyID, request.Exchange, request.MarketType, request.Symbol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
				AccountID:      request.AccountID,
				Exchange:       request.Exchange,
				MarketType:     request.MarketType,
				Symbol:         request.Symbol,
				Equity:         state.Equity,
				Leverage:       request.Leverage,
				MaintMarginPct: request.MaintMarginPct,
				MarkPrice:      request.MarkPrice,
				SnapshotTime:   request.MarkTime,
				FeeRate:        request.FeeRate,
				SlippageRate:   request.SlippageRate,
			}); err != nil {
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
		closed, err := repo.ClosePaperPositionWithRealizedPnLAndReason(ctx, position.ID, request.MarkPrice, request.MarkTime, realizedPnL, exitReason)
		if err != nil {
			return paperAccountState{}, fmt.Errorf("close paper position: %w", err)
		}
		state.Position = closed
		state.RealizedPnL += closed.RealizedPnL
		if !request.MarkTime.Before(dayStart) {
			state.DailyRealizedPnL += closed.RealizedPnL
		}
		if closed.RealizedPnL < 0 {
			state.ConsecutiveLosses++
		} else {
			state.ConsecutiveLosses = 0
		}
		state.TotalPnL = state.RealizedPnL
		state.Equity = request.Equity + state.TotalPnL
		state.AvailableBalance = state.Equity
		state.CloseNote = exitReason
		if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
			AccountID:      request.AccountID,
			Exchange:       request.Exchange,
			MarketType:     request.MarketType,
			Symbol:         request.Symbol,
			Equity:         state.Equity,
			Leverage:       request.Leverage,
			MaintMarginPct: request.MaintMarginPct,
			MarkPrice:      request.MarkPrice,
			SnapshotTime:   request.MarkTime,
			FeeRate:        request.FeeRate,
			SlippageRate:   request.SlippageRate,
		}); err != nil {
			return paperAccountState{}, err
		}
		fmt.Printf("paper_position_closed id=%d reason=%s exit=%.8f realized_pnl=%.8f\n", closed.ID, exitReason, request.MarkPrice, closed.RealizedPnL)
		return state, nil
	}

	if adjustedStop, reason, ok := paperProtectiveStopLoss(position, request); ok {
		if err := repo.UpdatePaperPositionStopLoss(ctx, position.ID, adjustedStop); err != nil {
			return paperAccountState{}, fmt.Errorf("update protective stop loss: %w", err)
		}
		position.StopLossPrice = adjustedStop
		fmt.Printf("paper_position_stop_adjusted id=%d reason=%s stop=%.8f mark=%.8f\n", position.ID, reason, adjustedStop, request.MarkPrice)
	}

	if err := repo.UpdatePaperPositionMark(ctx, position.ID, request.MarkPrice); err != nil {
		return paperAccountState{}, fmt.Errorf("update paper position mark: %w", err)
	}
	position.MarkPrice = request.MarkPrice
	state.Open = true
	state.Position = position
	state.UnrealizedPnL = paperPositionNetPnL(position, request.MarkPrice, request.FeeRate, request.SlippageRate)
	state.TotalPnL = state.RealizedPnL + state.UnrealizedPnL
	state.Equity = request.Equity + state.TotalPnL
	state.CurrentExposure = paperPositionNotional(position, request.MarkPrice)
	state.CurrentSymbolExposure = state.CurrentExposure
	state.CurrentInitialMargin = paperInitialMargin(state.CurrentExposure, request.Leverage)
	state.CurrentMaintMargin = paperMaintenanceMargin(state.CurrentExposure, request.MaintMarginPct)
	state.AvailableBalance = paperAvailableBalance(state.Equity, state.CurrentInitialMargin)
	if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
		AccountID:      request.AccountID,
		Exchange:       request.Exchange,
		MarketType:     request.MarketType,
		Symbol:         request.Symbol,
		Equity:         state.Equity,
		Leverage:       request.Leverage,
		MaintMarginPct: request.MaintMarginPct,
		Position:       position,
		MarkPrice:      request.MarkPrice,
		SnapshotTime:   request.MarkTime,
		FeeRate:        request.FeeRate,
		SlippageRate:   request.SlippageRate,
	}); err != nil {
		return paperAccountState{}, err
	}
	return state, nil
}

func paperTradingDayStart(markTime time.Time) time.Time {
	if markTime.IsZero() {
		markTime = time.Now().UTC()
	}
	year, month, day := markTime.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
	if err := savePaperAccountSnapshots(ctx, repo, paperSnapshotRequest{
		AccountID:      request.AccountID,
		Exchange:       request.Exchange,
		MarketType:     request.MarketType,
		Symbol:         request.Symbol,
		Equity:         request.Equity + openPnL,
		Leverage:       request.Leverage,
		MaintMarginPct: request.MaintMarginPct,
		Position:       position,
		MarkPrice:      request.EntryPrice,
		SnapshotTime:   request.OpenedAt,
		FeeRate:        request.FeeRate,
		SlippageRate:   request.SlippageRate,
	}); err != nil {
		return portfolio.PaperPositionRecord{}, err
	}
	return position, nil
}

func recordPaperCloseOrder(ctx context.Context, repo *execution.SQLiteRepository, config paperRunConfig, state paperAccountState, latestPrice float64, latestTime time.Time) (execution.OrderRecord, error) {
	position := state.Position
	quantity := math.Abs(position.Quantity)
	if quantity <= 0 {
		return execution.OrderRecord{}, errors.New("close order quantity must be positive")
	}
	quantity = paperRoundQuantity(quantity, config.QuantityStep)
	if err := validatePaperQuantity(quantity, config); err != nil {
		return execution.OrderRecord{}, fmt.Errorf("close order quantity: %w", err)
	}
	latestPrice = paperRoundPriceNearest(latestPrice, config.PriceTickSize)
	side := risk.SideSell
	if strings.EqualFold(position.PositionSide, "short") {
		side = risk.SideBuy
	}
	result, err := repo.RecordDryRunOrder(ctx, execution.DryRunRequest{
		Account: paperRiskAccountSnapshot(config, state),
		Order: risk.OrderIntent{
			Exchange:   config.Exchange,
			MarketType: risk.MarketType(config.MarketType),
			Symbol:     config.Symbol,
			Side:       side,
			Price:      latestPrice,
			Quantity:   quantity,
			Leverage:   config.Leverage,
			ReduceOnly: true,
			StopPrice:  0,
		},
		RiskConfig:    paperRiskConfig(config),
		StrategyID:    config.StrategyID,
		ClientOrderID: fmt.Sprintf("paper-close-%s-%d", config.Symbol, latestTime.UTC().UnixNano()),
		OrderType:     "market",
		TimeInForce:   "IOC",
		PositionModel: config.PositionModel,
	})
	if err != nil {
		return execution.OrderRecord{}, fmt.Errorf("record paper close order: %w", err)
	}
	return result.Order, nil
}

func maybeSubmitOneBullExOrder(ctx context.Context, repo *execution.SQLiteRepository, order execution.OrderRecord, config paperRunConfig) (execution.OrderRecord, error) {
	if !config.SubmitExchange {
		return order, nil
	}
	if normalizeExchangeName(config.Exchange) != onebullex.ExchangeName {
		return order, fmt.Errorf("exchange submit currently supports %s only", onebullex.ExchangeName)
	}
	if order.RiskDecision != risk.DecisionAllow {
		return order, nil
	}
	updated, err := repo.SubmitOrderToExchange(ctx, oneBullExClientFromEnv(), order)
	if err != nil {
		return order, fmt.Errorf("submit onebullex order %s: %w", order.ClientOrderID, err)
	}
	return updated, nil
}

func oneBullExClientFromEnv() *onebullex.Client {
	return onebullex.NewClient(
		onebullex.WithBaseURL(env("ONEBULLEX_BASE_URL", "")),
		onebullex.WithCredentials(env("ONEBULLEX_API_KEY", ""), env("ONEBULLEX_SECRET_KEY", "")),
		onebullex.WithTradingEnabled(envBool("ONEBULLEX_LIVE_TRADING", false)),
	)
}

type paperSnapshotRequest struct {
	AccountID      string
	Exchange       string
	MarketType     string
	Symbol         string
	Equity         float64
	Leverage       float64
	MaintMarginPct float64
	Position       portfolio.PaperPositionRecord
	MarkPrice      float64
	SnapshotTime   time.Time
	FeeRate        float64
	SlippageRate   float64
}

func savePaperAccountSnapshots(ctx context.Context, repo *portfolio.SQLiteRepository, request paperSnapshotRequest) error {
	if request.SnapshotTime.IsZero() {
		request.SnapshotTime = time.Now().UTC()
	}
	notional := paperPositionNotional(request.Position, request.MarkPrice)
	locked := paperInitialMargin(notional, request.Leverage)
	free := paperAvailableBalance(request.Equity, locked)
	maintenanceMargin := paperMaintenanceMargin(notional, request.MaintMarginPct)
	marginRatio := 0.0
	if request.Equity > 0 {
		marginRatio = maintenanceMargin / request.Equity * 100
	}
	if _, err := repo.SaveBalanceSnapshot(ctx, portfolio.BalanceSnapshot{
		AccountID:    request.AccountID,
		Exchange:     request.Exchange,
		Asset:        "USDT",
		Free:         free,
		Locked:       locked,
		Total:        request.Equity,
		USDValue:     request.Equity,
		SnapshotTime: request.SnapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper balance snapshot: %w", err)
	}

	if request.Position.ID != 0 {
		pnl := paperPositionNetPnL(request.Position, request.MarkPrice, request.FeeRate, request.SlippageRate)
		liqPrice := paperLiquidationPrice(request.Position.PositionSide, request.Position.EntryPrice, request.Leverage, request.MaintMarginPct)
		if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
			AccountID:        request.AccountID,
			Exchange:         request.Exchange,
			MarketType:       request.MarketType,
			Symbol:           request.Symbol,
			PositionSide:     request.Position.PositionSide,
			Quantity:         request.Position.Quantity,
			EntryPrice:       request.Position.EntryPrice,
			MarkPrice:        request.MarkPrice,
			LiquidationPrice: liqPrice,
			Leverage:         request.Leverage,
			MarginMode:       "isolated-paper",
			UnrealizedPnL:    pnl,
			Notional:         notional,
			SnapshotTime:     request.SnapshotTime,
		}); err != nil {
			return fmt.Errorf("save paper position snapshot: %w", err)
		}
	} else {
		for _, side := range []string{"long", "short"} {
			if _, err := repo.SavePositionSnapshot(ctx, portfolio.PositionSnapshot{
				AccountID:    request.AccountID,
				Exchange:     request.Exchange,
				MarketType:   request.MarketType,
				Symbol:       request.Symbol,
				PositionSide: side,
				Quantity:     0,
				EntryPrice:   0,
				MarkPrice:    request.MarkPrice,
				Leverage:     request.Leverage,
				MarginMode:   "isolated-paper",
				SnapshotTime: request.SnapshotTime,
			}); err != nil {
				return fmt.Errorf("save flat paper %s position snapshot: %w", side, err)
			}
		}
	}

	if _, err := repo.SaveMarginSnapshot(ctx, portfolio.MarginSnapshot{
		AccountID:         request.AccountID,
		Exchange:          request.Exchange,
		MarketType:        request.MarketType,
		Equity:            request.Equity,
		MarginBalance:     request.Equity,
		InitialMargin:     locked,
		MaintenanceMargin: maintenanceMargin,
		MarginRatio:       marginRatio,
		AvailableBalance:  free,
		SnapshotTime:      request.SnapshotTime,
	}); err != nil {
		return fmt.Errorf("save paper margin snapshot: %w", err)
	}
	return nil
}

func paperExitReason(position portfolio.PaperPositionRecord, markPrice float64) string {
	if strings.EqualFold(position.PositionSide, "short") {
		// Short TP/SL: take profit when mark price falls to TP; stop loss when it rises to SL.
		if position.TakeProfitPrice > 0 && markPrice <= position.TakeProfitPrice {
			return "take_profit"
		}
		if position.StopLossPrice > 0 && markPrice >= position.StopLossPrice {
			return paperStopExitReason(position)
		}
		return ""
	}
	// Long TP/SL: take profit when mark price rises to TP; stop loss when it falls to SL.
	if position.TakeProfitPrice > 0 && markPrice >= position.TakeProfitPrice {
		return "take_profit"
	}
	if position.StopLossPrice > 0 && markPrice <= position.StopLossPrice {
		return paperStopExitReason(position)
	}
	return ""
}

func paperStopExitReason(position portfolio.PaperPositionRecord) string {
	if strings.EqualFold(position.PositionSide, "short") {
		if position.StopLossPrice > 0 && position.EntryPrice > 0 && position.StopLossPrice <= position.EntryPrice {
			return "protective_stop"
		}
		return "stop_loss"
	}
	if position.StopLossPrice > 0 && position.EntryPrice > 0 && position.StopLossPrice >= position.EntryPrice {
		return "protective_stop"
	}
	return "stop_loss"
}

func paperProtectiveStopLoss(position portfolio.PaperPositionRecord, request paperSettleRequest) (float64, string, bool) {
	if position.EntryPrice <= 0 || position.StopLossPrice <= 0 || request.MarkPrice <= 0 {
		return 0, "", false
	}
	initialStop := position.InitialStopLoss
	if initialStop <= 0 {
		initialStop = position.StopLossPrice
	}
	initialRisk := math.Abs(position.EntryPrice - initialStop)
	if initialRisk <= 0 {
		return 0, "", false
	}

	stop := position.StopLossPrice
	reason := ""
	costBuffer := 2 * (request.FeeRate + request.SlippageRate)
	if request.BreakevenStop && request.BreakevenR > 0 {
		if candidate, ok := paperBreakevenStop(position, request.MarkPrice, initialRisk, request.BreakevenR, costBuffer); ok {
			stop, reason = paperMoreProtectiveStop(position.PositionSide, stop, candidate, reason, "breakeven_stop")
		}
	}
	if request.TrailingStop && request.TrailingR > 0 && request.TrailingATRMult > 0 {
		if candidate, ok := paperTrailingStop(position, request, initialRisk); ok {
			stop, reason = paperMoreProtectiveStop(position.PositionSide, stop, candidate, reason, "atr_trailing_stop")
		}
	}
	if reason == "" || math.Abs(stop-position.StopLossPrice) < 1e-12 {
		return 0, "", false
	}
	if strings.EqualFold(position.PositionSide, "short") {
		stop = paperRoundPriceUp(stop, request.PriceTickSize)
		if stop >= position.StopLossPrice || stop <= request.MarkPrice {
			return 0, "", false
		}
		return stop, reason, true
	}
	stop = paperRoundPriceDown(stop, request.PriceTickSize)
	if stop <= position.StopLossPrice || stop >= request.MarkPrice {
		return 0, "", false
	}
	return stop, reason, true
}

func paperBreakevenStop(position portfolio.PaperPositionRecord, markPrice float64, initialRisk float64, triggerR float64, costBuffer float64) (float64, bool) {
	if strings.EqualFold(position.PositionSide, "short") {
		if position.EntryPrice-markPrice < initialRisk*triggerR {
			return 0, false
		}
		return position.EntryPrice * (1 - costBuffer), true
	}
	if markPrice-position.EntryPrice < initialRisk*triggerR {
		return 0, false
	}
	return position.EntryPrice * (1 + costBuffer), true
}

func paperTrailingStop(position portfolio.PaperPositionRecord, request paperSettleRequest, initialRisk float64) (float64, bool) {
	atrPct, ok := latestPaperATRPct(request.Candles, request.ATRWindow)
	if !ok || atrPct <= 0 {
		return 0, false
	}
	atr := request.MarkPrice * atrPct / 100
	if atr <= 0 {
		return 0, false
	}
	if strings.EqualFold(position.PositionSide, "short") {
		if position.EntryPrice-request.MarkPrice < initialRisk*request.TrailingR {
			return 0, false
		}
		return request.MarkPrice + atr*request.TrailingATRMult, true
	}
	if request.MarkPrice-position.EntryPrice < initialRisk*request.TrailingR {
		return 0, false
	}
	return request.MarkPrice - atr*request.TrailingATRMult, true
}

func paperMoreProtectiveStop(positionSide string, current float64, candidate float64, currentReason string, candidateReason string) (float64, string) {
	if candidate <= 0 {
		return current, currentReason
	}
	if strings.EqualFold(positionSide, "short") {
		if current <= 0 || candidate < current {
			return candidate, candidateReason
		}
		return current, currentReason
	}
	if current <= 0 || candidate > current {
		return candidate, candidateReason
	}
	return current, currentReason
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

func paperEffectiveTPSL(candles []marketdata.Candle, config paperRunConfig) (paperTPSLPercents, error) {
	if !config.DynamicTPSL {
		return paperTPSLPercents{
			TakeProfitPct: config.TakeProfitPct,
			StopLossPct:   config.StopLossPct,
			Source:        "fixed_pct",
		}, nil
	}
	takeProfitPct, stopLossPct, ok, err := backtest.LatestScalpTPSLPercents(candles, paperBacktestScalpConfig(paperStrategyConfig{
		StrategyType:         config.StrategyType,
		MarketType:           config.MarketType,
		FastWindow:           config.FastWindow,
		SlowWindow:           config.SlowWindow,
		TakeProfitPct:        config.TakeProfitPct,
		StopLossPct:          config.StopLossPct,
		DynamicTPSL:          config.DynamicTPSL,
		TakeProfitATRMult:    config.TakeProfitATRMult,
		StopLossATRMult:      config.StopLossATRMult,
		MinTakeProfitPct:     config.MinTakeProfitPct,
		MaxTakeProfitPct:     config.MaxTakeProfitPct,
		MinStopLossPct:       config.MinStopLossPct,
		MaxStopLossPct:       config.MaxStopLossPct,
		CooldownBars:         config.CooldownBars,
		FeeRate:              config.FeeRate,
		SlippageRate:         config.SlippageRate,
		MinTrendSpreadPct:    config.MinTrendSpreadPct,
		ConfirmBars:          config.ConfirmBars,
		ATRWindow:            config.ATRWindow,
		MinATRPct:            config.MinATRPct,
		MaxATRPct:            config.MaxATRPct,
		VolumeWindow:         config.VolumeWindow,
		MinVolumeRatio:       config.MinVolumeRatio,
		MaxEntryExtensionPct: config.MaxEntryExtensionPct,
		PullbackLookback:     config.PullbackLookback,
		PullbackTolerancePct: config.PullbackTolerancePct,
	}))
	if err != nil {
		return paperTPSLPercents{}, fmt.Errorf("compute dynamic tpsl: %w", err)
	}
	if !ok {
		return paperTPSLPercents{}, backtest.ErrNotEnoughData
	}
	return paperTPSLPercents{
		TakeProfitPct: takeProfitPct,
		StopLossPct:   stopLossPct,
		Source:        "atr_dynamic",
	}, nil
}

func paperOrderPlan(action strategy.SignalAction, price float64, takeProfitPct float64, stopLossPct float64) (risk.Side, string, float64, float64, error) {
	if price <= 0 {
		return "", "", 0, 0, errors.New("price must be positive")
	}
	switch action {
	case strategy.SignalBuy:
		// Long entry: stop below entry price, take profit above entry price.
		return risk.SideBuy, "long", price * (1 - stopLossPct/100), price * (1 + takeProfitPct/100), nil
	case strategy.SignalShort:
		// Short entry: stop above entry price, take profit below entry price.
		return risk.SideSell, "short", price * (1 + stopLossPct/100), price * (1 - takeProfitPct/100), nil
	default:
		return "", "", 0, 0, fmt.Errorf("unsupported paper action %q", action)
	}
}

func paperOrderQuantity(entryPrice float64, stopPrice float64, config paperRunConfig, state paperAccountState) (float64, error) {
	if entryPrice <= 0 || stopPrice <= 0 {
		return 0, errors.New("entry and stop price must be positive")
	}
	equity := state.Equity
	if equity <= 0 {
		equity = config.Equity
	}
	availableBalance := state.AvailableBalance
	if availableBalance <= 0 {
		availableBalance = equity
	}
	if equity <= 0 {
		return 0, errors.New("equity must be positive")
	}
	if config.RiskPct <= 0 {
		if config.Quantity <= 0 {
			return 0, errors.New("quantity must be positive")
		}
		quantity := paperCapQuantity(entryPrice, config.Quantity, config, equity, availableBalance, state.CurrentInitialMargin)
		if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
			return 0, errors.New("computed quantity must be positive and finite")
		}
		return paperFinalizeOrderQuantity(quantity, config)
	}
	riskDistance := math.Abs(entryPrice - stopPrice)
	if riskDistance <= 0 {
		return 0, errors.New("stop price must be different from entry price")
	}
	riskBudget := equity * config.RiskPct / 100
	quantity := riskBudget / riskDistance
	quantity = paperCapQuantity(entryPrice, quantity, config, equity, availableBalance, state.CurrentInitialMargin)
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, errors.New("computed quantity must be positive and finite")
	}
	return paperFinalizeOrderQuantity(quantity, config)
}

func paperFinalizeOrderQuantity(quantity float64, config paperRunConfig) (float64, error) {
	quantity = paperRoundQuantity(quantity, config.QuantityStep)
	if err := validatePaperQuantity(quantity, config); err != nil {
		return 0, err
	}
	return quantity, nil
}

type alignedPaperOrderPlan struct {
	EntryPrice      float64
	StopPrice       float64
	TakeProfitPrice float64
	Quantity        float64
}

func alignPaperOrderPlan(positionSide string, entryPrice float64, stopPrice float64, takeProfitPrice float64, quantity float64, config paperRunConfig) (alignedPaperOrderPlan, error) {
	entryPrice = paperRoundPriceNearest(entryPrice, config.PriceTickSize)
	quantity = paperRoundQuantity(quantity, config.QuantityStep)
	if err := validatePaperQuantity(quantity, config); err != nil {
		return alignedPaperOrderPlan{}, err
	}
	switch strings.ToLower(strings.TrimSpace(positionSide)) {
	case "long":
		stopPrice = paperRoundPriceDown(stopPrice, config.PriceTickSize)
		takeProfitPrice = paperRoundPriceDown(takeProfitPrice, config.PriceTickSize)
	case "short":
		stopPrice = paperRoundPriceUp(stopPrice, config.PriceTickSize)
		takeProfitPrice = paperRoundPriceUp(takeProfitPrice, config.PriceTickSize)
	default:
		return alignedPaperOrderPlan{}, fmt.Errorf("unsupported position side %q", positionSide)
	}
	if entryPrice <= 0 || stopPrice <= 0 || takeProfitPrice <= 0 {
		return alignedPaperOrderPlan{}, errors.New("aligned entry, stop, and take profit prices must be positive")
	}
	return alignedPaperOrderPlan{
		EntryPrice:      entryPrice,
		StopPrice:       stopPrice,
		TakeProfitPrice: takeProfitPrice,
		Quantity:        quantity,
	}, nil
}

func validatePaperQuantity(quantity float64, config paperRunConfig) error {
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return errors.New("computed quantity must be positive and finite")
	}
	if config.MinOrderQuantity > 0 && quantity+1e-12 < config.MinOrderQuantity {
		return fmt.Errorf("computed quantity %.8f below minimum %.8f", quantity, config.MinOrderQuantity)
	}
	return nil
}

func paperRoundQuantity(quantity float64, step float64) float64 {
	if step <= 0 || quantity <= 0 {
		return quantity
	}
	return math.Floor((quantity+1e-12)/step) * step
}

func paperRoundPriceNearest(price float64, tick float64) float64 {
	if tick <= 0 || price <= 0 {
		return price
	}
	return math.Round(price/tick) * tick
}

func paperRoundPriceDown(price float64, tick float64) float64 {
	if tick <= 0 || price <= 0 {
		return price
	}
	return math.Floor((price+1e-12)/tick) * tick
}

func paperRoundPriceUp(price float64, tick float64) float64 {
	if tick <= 0 || price <= 0 {
		return price
	}
	return math.Ceil((price-1e-12)/tick) * tick
}

func paperCapQuantity(entryPrice float64, quantity float64, config paperRunConfig, equity float64, availableBalance float64, currentInitialMargin float64) float64 {
	if quantity <= 0 || entryPrice <= 0 || equity <= 0 {
		return 0
	}
	if config.MaxNotionalPct > 0 {
		maxNotional := equity * config.MaxNotionalPct / 100
		maxQuantity := maxNotional / entryPrice
		if quantity > maxQuantity {
			quantity = maxQuantity
		}
	}
	if config.Leverage > 0 {
		if config.MaxBalanceUsePct > 0 && availableBalance > 0 {
			maxOrderMargin := availableBalance * config.MaxBalanceUsePct / 100
			maxQuantity := maxOrderMargin * config.Leverage / entryPrice
			if quantity > maxQuantity {
				quantity = maxQuantity
			}
		}
		if config.MaxMarginPct > 0 {
			maxTotalMargin := equity * config.MaxMarginPct / 100
			remainingMargin := maxTotalMargin - currentInitialMargin
			if remainingMargin < 0 {
				remainingMargin = 0
			}
			maxQuantity := remainingMargin * config.Leverage / entryPrice
			if quantity > maxQuantity {
				quantity = maxQuantity
			}
		}
	}
	return quantity
}

func paperRiskConfig(config paperRunConfig) risk.Config {
	defaultRisk := risk.DefaultConfig()
	maxSymbolExposurePct := config.MaxNotionalPct
	if maxSymbolExposurePct <= 0 {
		maxSymbolExposurePct = defaultRisk.MaxSymbolExposurePct
	}
	maxTotalExposurePct := math.Max(defaultRisk.MaxTotalExposurePct, maxSymbolExposurePct)
	return risk.Config{
		MaxOrderRiskPct:           paperPositiveOrDefault(config.MaxOrderRiskPct, defaultRisk.MaxOrderRiskPct),
		MaxSymbolExposurePct:      maxSymbolExposurePct,
		MaxTotalExposurePct:       maxTotalExposurePct,
		MaxInitialMarginPct:       paperPositiveOrDefault(config.MaxMarginPct, defaultRisk.MaxInitialMarginPct),
		MaxAvailableBalanceUsePct: paperNonNegativeOrDefault(config.MaxBalanceUsePct, defaultRisk.MaxAvailableBalanceUsePct),
		MaxLeverage:               paperPositiveOrDefault(config.MaxLeverage, defaultRisk.MaxLeverage),
		MaxDailyLossPct:           paperPositiveOrDefault(config.MaxDailyLossPct, defaultRisk.MaxDailyLossPct),
		MaxConsecutiveLosses:      paperPositiveIntOrDefault(config.MaxConsecutiveLosses, defaultRisk.MaxConsecutiveLosses),
		MinLiquidationDistancePct: paperNonNegativeOrDefault(config.MinLiqDistancePct, defaultRisk.MinLiquidationDistancePct),
		MaintenanceMarginRatePct:  paperNonNegativeOrDefault(config.MaintMarginPct, defaultRisk.MaintenanceMarginRatePct),
		MaxAbsFundingRatePct:      paperNonNegativeOrDefault(config.MaxAbsFundingRatePct, defaultRisk.MaxAbsFundingRatePct),
		MinQuantity:               paperNonNegativeOrDefault(config.MinOrderQuantity, defaultRisk.MinQuantity),
		QuantityStep:              paperNonNegativeOrDefault(config.QuantityStep, defaultRisk.QuantityStep),
		PriceTickSize:             paperNonNegativeOrDefault(config.PriceTickSize, defaultRisk.PriceTickSize),
	}
}

func paperRiskAccountSnapshot(config paperRunConfig, state paperAccountState) risk.AccountSnapshot {
	equity := state.Equity
	if equity <= 0 {
		equity = config.Equity
	}
	availableBalance := state.AvailableBalance
	if availableBalance <= 0 {
		availableBalance = equity
	}
	return risk.AccountSnapshot{
		AccountID:             config.AccountID,
		Equity:                equity,
		AvailableBalance:      availableBalance,
		DailyRealizedPnL:      state.DailyRealizedPnL,
		ConsecutiveLosses:     state.ConsecutiveLosses,
		CurrentTotalExposure:  state.CurrentExposure,
		CurrentSymbolExposure: state.CurrentSymbolExposure,
		CurrentInitialMargin:  state.CurrentInitialMargin,
		CurrentMaintMargin:    state.CurrentMaintMargin,
		SnapshotTime:          time.Now().UTC(),
	}
}

func paperPositionNotional(position portfolio.PaperPositionRecord, markPrice float64) float64 {
	if position.ID == 0 || markPrice <= 0 {
		return 0
	}
	return math.Abs(position.Quantity * markPrice)
}

func paperInitialMargin(notional float64, leverage float64) float64 {
	if notional <= 0 {
		return 0
	}
	return notional / math.Max(leverage, 1)
}

func paperMaintenanceMargin(notional float64, maintenanceMarginPct float64) float64 {
	if notional <= 0 || maintenanceMarginPct <= 0 {
		return 0
	}
	return notional * maintenanceMarginPct / 100
}

func paperAvailableBalance(equity float64, initialMargin float64) float64 {
	available := equity - initialMargin
	if available < 0 {
		return 0
	}
	return available
}

func paperLiquidationPrice(positionSide string, entryPrice float64, leverage float64, maintenanceMarginPct float64) float64 {
	side := risk.SideBuy
	if strings.EqualFold(positionSide, "short") {
		side = risk.SideSell
	}
	return risk.EstimateLiquidationPrice(entryPrice, side, leverage, maintenanceMarginPct)
}

func paperPositiveOrDefault(value float64, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func paperPositiveIntOrDefault(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func paperNonNegativeOrDefault(value float64, fallback float64) float64 {
	if value >= 0 {
		return value
	}
	return fallback
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
