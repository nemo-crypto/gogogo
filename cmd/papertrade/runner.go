package main

import (
	"context"
	"database/sql"
	"fmt"
	"gogogo/internal/backtest"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/risk"
	"gogogo/internal/strategy"
	"log"
	"strings"
	"time"
)

type paperRunOptions struct {
	SaveBacktest           bool
	SaveObservation        bool
	SaveAccountSnapshot    bool
	LastObservedCandleTime time.Time
}

type paperRunSummary struct {
	Action           strategy.SignalAction
	LatestCandleTime time.Time
	RunID            int64
	BacktestRunID    int64
	ObservationSaved bool
	BacktestSaved    bool
	AccountSaved     bool
	OpenedOrder      bool
	ClosedOrder      bool
	TotalReturnPct   float64
	ExcessReturnPct  float64
	MaxDrawdownPct   float64
	TradeCount       int
}

type paperWatchState struct {
	lastObservationAt time.Time
	lastBacktestAt    time.Time
	lastCandleTime    time.Time
}

func (s paperWatchState) nextOptions(now time.Time, config paperRunConfig) paperRunOptions {
	return paperRunOptions{
		SaveBacktest:           shouldPersistByInterval(now, s.lastBacktestAt, config.BacktestInterval),
		SaveObservation:        shouldPersistByInterval(now, s.lastObservationAt, config.PersistInterval),
		SaveAccountSnapshot:    shouldPersistByInterval(now, s.lastObservationAt, config.PersistInterval),
		LastObservedCandleTime: s.lastCandleTime,
	}
}

func (s *paperWatchState) observe(now time.Time, summary paperRunSummary) {
	if summary.ObservationSaved {
		s.lastObservationAt = now
		if !summary.LatestCandleTime.IsZero() {
			s.lastCandleTime = summary.LatestCandleTime
		}
	}
	if summary.BacktestSaved {
		s.lastBacktestAt = now
	}
}

func shouldPersistByInterval(now time.Time, last time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return true
	}
	if last.IsZero() {
		return true
	}
	return !now.Before(last.Add(interval))
}

func latestPaperCandleTime(candles []marketdata.Candle) time.Time {
	if len(candles) == 0 {
		return time.Time{}
	}
	return candles[len(candles)-1].OpenTime.UTC()
}

func shouldSavePaperObservation(options paperRunOptions, latestCandleTime time.Time, action strategy.SignalAction, closeNote string) bool {
	if options.SaveObservation {
		return true
	}
	if strings.TrimSpace(closeNote) != "" {
		return true
	}
	if action != strategy.SignalHold {
		return true
	}
	return !latestCandleTime.IsZero() && latestCandleTime.After(options.LastObservedCandleTime)
}

func watchPaperStrategy(ctx context.Context, db *sql.DB, config paperRunConfig) error {
	if config.PollInterval <= 0 {
		config.PollInterval = 15 * time.Second
	}
	if config.PersistInterval <= 0 {
		config.PersistInterval = time.Minute
	}
	if config.BacktestInterval < 0 {
		config.BacktestInterval = 0
	}
	log.Printf("papertrade watch started: strategy=%s symbol=%s interval=%s poll_interval=%s persist_interval=%s backtest_interval=%s", config.StrategyID, config.Symbol, config.Interval, config.PollInterval, config.PersistInterval, config.BacktestInterval)
	state := paperWatchState{}
	for {
		current := config
		current.End = time.Now().UTC()
		current.Start = current.End.Add(-paperLookbackDuration(config.Interval, config.LookbackCandles))
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		summary, err := runPaperStrategy(runCtx, db, current, state.nextOptions(current.End, config))
		if err != nil {
			log.Printf("papertrade watch error: %v", err)
		} else {
			state.observe(current.End, summary)
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
	_, err := runPaperStrategy(ctx, db, config, paperRunOptions{
		SaveBacktest:        true,
		SaveObservation:     true,
		SaveAccountSnapshot: true,
	})
	return err
}

func runPaperStrategy(ctx context.Context, db *sql.DB, config paperRunConfig, options paperRunOptions) (paperRunSummary, error) {
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
		return paperRunSummary{}, fmt.Errorf("list candles: %w", err)
	}
	signalCandles := closedPaperCandles(candles, config.End)
	ignoredOpenCandles := len(candles) - len(signalCandles)
	result, err := runPaperBacktest(signalCandles, paperStrategyConfig{
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
	})
	if err != nil {
		return paperRunSummary{}, fmt.Errorf("run paper strategy: %w", err)
	}
	latestCandleTime := latestPaperCandleTime(signalCandles)

	summary := paperRunSummary{
		LatestCandleTime: latestCandleTime,
		TotalReturnPct:   result.TotalReturnPct,
		ExcessReturnPct:  result.ExcessReturnPct,
		MaxDrawdownPct:   result.MaxDrawdownPct,
		TradeCount:       len(result.Trades),
	}
	backtestRunID := int64(0)
	if options.SaveBacktest {
		backtestRepo := backtest.NewSQLiteRepository(db)
		backtestRunID, err = backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
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
			return paperRunSummary{}, fmt.Errorf("save paper backtest run: %w", err)
		}
		summary.BacktestRunID = backtestRunID
		summary.BacktestSaved = true
	}

	strategyRepo := strategy.NewSQLiteRepository(db)
	configJSON, err := marshalJSON(map[string]any{
		"strategy_type":            normalizedStrategyType(config.StrategyType),
		"profile":                  config.Profile,
		"fast":                     config.FastWindow,
		"slow":                     config.SlowWindow,
		"symbol":                   config.Symbol,
		"take_profit_pct":          config.TakeProfitPct,
		"stop_loss_pct":            config.StopLossPct,
		"dynamic_tpsl":             config.DynamicTPSL,
		"take_profit_atr_mult":     config.TakeProfitATRMult,
		"stop_loss_atr_mult":       config.StopLossATRMult,
		"min_take_profit_pct":      config.MinTakeProfitPct,
		"max_take_profit_pct":      config.MaxTakeProfitPct,
		"min_stop_loss_pct":        config.MinStopLossPct,
		"max_stop_loss_pct":        config.MaxStopLossPct,
		"breakeven_stop_enabled":   config.BreakevenStopEnabled,
		"breakeven_trigger_r":      config.BreakevenTriggerR,
		"trailing_stop_enabled":    config.TrailingStopEnabled,
		"trailing_activation_r":    config.TrailingActivationR,
		"trailing_atr_mult":        config.TrailingATRMult,
		"cooldown_bars":            config.CooldownBars,
		"fee_rate":                 config.FeeRate,
		"slippage_rate":            config.SlippageRate,
		"market_type":              config.MarketType,
		"min_trend_spread_pct":     config.MinTrendSpreadPct,
		"confirm_bars":             config.ConfirmBars,
		"atr_window":               config.ATRWindow,
		"min_atr_pct":              config.MinATRPct,
		"max_atr_pct":              config.MaxATRPct,
		"volume_window":            config.VolumeWindow,
		"min_volume_ratio":         config.MinVolumeRatio,
		"max_entry_extension_pct":  config.MaxEntryExtensionPct,
		"pullback_lookback":        config.PullbackLookback,
		"pullback_tolerance_pct":   config.PullbackTolerancePct,
		"risk_pct":                 config.RiskPct,
		"max_notional_pct":         config.MaxNotionalPct,
		"max_margin_pct":           config.MaxMarginPct,
		"max_balance_use_pct":      config.MaxBalanceUsePct,
		"min_liq_distance_pct":     config.MinLiqDistancePct,
		"maintenance_margin_pct":   config.MaintMarginPct,
		"max_order_risk_pct":       config.MaxOrderRiskPct,
		"max_daily_loss_pct":       config.MaxDailyLossPct,
		"max_consecutive_losses":   config.MaxConsecutiveLosses,
		"max_abs_funding_rate_pct": config.MaxAbsFundingRatePct,
		"max_leverage":             config.MaxLeverage,
		"leverage":                 config.Leverage,
		"signal_filter_enabled":    config.SignalFilterEnabled,
		"min_signal_score":         config.MinSignalScore,
		"trend_filter_enabled":     config.TrendFilterEnabled,
		"trend_interval":           config.TrendInterval,
		"macro_trend_interval":     config.MacroTrendInterval,
		"trend_fast":               config.TrendFastWindow,
		"trend_slow":               config.TrendSlowWindow,
		"trend_min_spread_pct":     config.TrendMinSpreadPct,
		"max_market_data_age":      config.MaxMarketDataAge.String(),
		"max_candle_age":           config.MaxCandleAge.String(),
		"max_mark_price_age":       config.MaxMarkPriceAge.String(),
		"require_mark_price":       config.RequireMarkPrice,
		"min_order_quantity":       config.MinOrderQuantity,
		"quantity_step":            config.QuantityStep,
		"price_tick_size":          config.PriceTickSize,
		"submit_exchange":          config.SubmitExchange,
	})
	if err != nil {
		return paperRunSummary{}, fmt.Errorf("encode strategy config: %w", err)
	}
	marketSnapshot, err := latestPaperMarketSnapshot(ctx, mdRepo, config, signalCandles)
	if err != nil {
		return paperRunSummary{}, err
	}
	trendRegime, err := loadPaperTrendRegime(ctx, mdRepo, config)
	if err != nil {
		return paperRunSummary{}, err
	}
	latestPrice := marketSnapshot.Price
	latestTime := marketSnapshot.PriceTime

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
		MaintMarginPct:  config.MaintMarginPct,
		TakeProfitPct:   config.TakeProfitPct,
		StopLossPct:     config.StopLossPct,
		PriceTickSize:   config.PriceTickSize,
		FastWindow:      config.FastWindow,
		SlowWindow:      config.SlowWindow,
		ATRWindow:       config.ATRWindow,
		BreakevenStop:   config.BreakevenStopEnabled,
		BreakevenR:      config.BreakevenTriggerR,
		TrailingStop:    config.TrailingStopEnabled,
		TrailingR:       config.TrailingActivationR,
		TrailingATRMult: config.TrailingATRMult,
		FeeRate:         config.FeeRate,
		SlippageRate:    config.SlippageRate,
		StrategyType:    config.StrategyType,
		Candles:         signalCandles,
		AllowPaperState: true,
		SaveSnapshots:   options.SaveAccountSnapshot && shouldSavePaperAccountSnapshots(config),
	})
	if err != nil {
		return paperRunSummary{}, err
	}
	liveAccount, err := maybeApplyLiveAccountBalance(ctx, portfolioRepo, config, &paperState)
	if err != nil {
		return paperRunSummary{}, err
	}
	orderRepo := execution.NewSQLiteRepository(db)
	if paperState.CloseNote != "" && paperState.Position.ID != 0 {
		closeOrder, err := recordPaperCloseOrder(ctx, orderRepo, config, paperState, latestPrice, latestTime)
		if err != nil {
			return paperRunSummary{}, err
		}
		closeOrder, err = maybeSubmitOneBullExOrder(ctx, orderRepo, closeOrder, config)
		if err != nil {
			return paperRunSummary{}, err
		}
		summary.ClosedOrder = true
		log.Printf("paper_close_order_id=%d status=%s decision=%s exchange_order_id=%s reason=%s", closeOrder.ID, closeOrder.Status, closeOrder.RiskDecision, closeOrder.ExchangeOrderID, paperState.CloseNote)
	}
	signal := paperSignalAction(signalCandles, result, paperStrategyConfig{
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
	})
	candidateAction := signal.Action
	assessment := assessPaperSignal(signalCandles, result, signal, marketSnapshot, config)
	action := candidateAction
	if paperState.Open {
		action = strategy.SignalHold
		assessment.AllowEntry = false
		assessment.Reason = "open_position"
	} else if isEntryAction(action) && !trendRegimeAllowsAction(trendRegime, action) {
		action = strategy.SignalHold
		assessment.AllowEntry = false
		assessment.Reason = trendRegimeBlockReason(trendRegime, candidateAction)
	} else if isEntryAction(action) && !assessment.AllowEntry {
		action = strategy.SignalHold
	}
	if isEntryAction(action) {
		if halt := paperEntryRiskHalt(config, paperState); halt.Reason != "" {
			action = strategy.SignalHold
			assessment.AllowEntry = false
			assessment.Reason = halt.Reason
			assessment.Features["risk_halt_reason"] = halt.Reason
			assessment.Features["risk_halt_message"] = halt.Message
			appendEntryBlocker(assessment.Features, halt.Reason)
		}
	}
	assessment.Features["trend_regime"] = trendRegime.Regime
	assessment.Features["trend_regime_reason"] = trendRegime.Reason
	assessment.Features["trend_filter_enabled"] = trendRegime.Enabled
	assessment.Features["trend_allow_long"] = trendRegime.AllowLong
	assessment.Features["trend_allow_short"] = trendRegime.AllowShort
	assessment.Features["trend_timeframe"] = trendRegime.TrendInterval
	assessment.Features["macro_trend_timeframe"] = trendRegime.MacroInterval
	assessment.Features["trend_spread_pct_htf"] = nullableFeature(trendRegime.TrendSpreadPct, trendRegime.Available)
	assessment.Features["macro_trend_spread_pct"] = nullableFeature(trendRegime.MacroSpreadPct, trendRegime.MacroAvailable)
	summary.Action = action
	saveObservation := shouldSavePaperObservation(options, latestCandleTime, action, paperState.CloseNote)
	runID := int64(0)
	rawFeaturesJSON, err := marshalJSON(map[string]any{
		"strategy_name":           result.StrategyName,
		"total_return_pct":        result.TotalReturnPct,
		"excess_return_pct":       result.ExcessReturnPct,
		"max_drawdown_pct":        result.MaxDrawdownPct,
		"trade_count":             len(result.Trades),
		"win_rate_pct":            result.WinRatePct,
		"backtest_run_id":         backtestRunID,
		"profile":                 config.Profile,
		"take_profit_pct":         config.TakeProfitPct,
		"stop_loss_pct":           config.StopLossPct,
		"dynamic_tpsl":            config.DynamicTPSL,
		"take_profit_atr_mult":    config.TakeProfitATRMult,
		"stop_loss_atr_mult":      config.StopLossATRMult,
		"min_take_profit_pct":     config.MinTakeProfitPct,
		"max_take_profit_pct":     config.MaxTakeProfitPct,
		"min_stop_loss_pct":       config.MinStopLossPct,
		"max_stop_loss_pct":       config.MaxStopLossPct,
		"breakeven_stop_enabled":  config.BreakevenStopEnabled,
		"breakeven_trigger_r":     config.BreakevenTriggerR,
		"trailing_stop_enabled":   config.TrailingStopEnabled,
		"trailing_activation_r":   config.TrailingActivationR,
		"trailing_atr_mult":       config.TrailingATRMult,
		"min_trend_spread_pct":    config.MinTrendSpreadPct,
		"confirm_bars":            config.ConfirmBars,
		"atr_window":              config.ATRWindow,
		"min_atr_pct":             config.MinATRPct,
		"max_atr_pct":             config.MaxATRPct,
		"volume_window":           config.VolumeWindow,
		"min_volume_ratio":        config.MinVolumeRatio,
		"max_entry_extension_pct": config.MaxEntryExtensionPct,
		"pullback_lookback":       config.PullbackLookback,
		"pullback_tolerance_pct":  config.PullbackTolerancePct,
		"risk_pct":                config.RiskPct,
		"max_notional_pct":        config.MaxNotionalPct,
		"max_margin_pct":          config.MaxMarginPct,
		"max_balance_use_pct":     config.MaxBalanceUsePct,
		"min_liq_distance_pct":    config.MinLiqDistancePct,
		"max_daily_loss_pct":      config.MaxDailyLossPct,
		"max_consecutive_losses":  config.MaxConsecutiveLosses,
		"daily_realized_pnl":      paperState.DailyRealizedPnL,
		"consecutive_losses":      paperState.ConsecutiveLosses,
		"latest_signal_input":     normalizedStrategyType(config.StrategyType),
		"signal_closed_candles":   len(signalCandles),
		"ignored_open_candles":    ignoredOpenCandles,
		"candidate_action":        candidateAction,
		"final_action":            action,
		"position_side":           signal.PositionSide,
		"signal_filter_enabled":   config.SignalFilterEnabled,
		"min_signal_score":        config.MinSignalScore,
		"signal_score":            assessment.Score,
		"signal_allowed":          assessment.AllowEntry,
		"signal_filter_reason":    assessment.Reason,
		"signal_features":         assessment.Features,
		"trend_regime":            trendRegime.Regime,
		"trend_regime_reason":     trendRegime.Reason,
		"trend_allow_long":        trendRegime.AllowLong,
		"trend_allow_short":       trendRegime.AllowShort,
		"price_source":            marketSnapshot.PriceSource,
		"price_time":              marketSnapshot.PriceTime,
		"candle_close_price":      marketSnapshot.CandleClosePrice,
		"candle_close_time":       marketSnapshot.CandleCloseTime,
		"mark_price":              marketSnapshot.MarkPrice,
		"mark_price_time":         marketSnapshot.MarkPriceTime,
		"funding_rate_pct":        marketSnapshot.LatestFundingRatePct,
		"funding_rate_time":       marketSnapshot.FundingRateTime,
		"funding_rate_source":     marketSnapshot.FundingRateSource,
		"live_account_applied":    liveAccount.Applied,
		"live_account_source":     liveAccount.Source,
		"live_account_equity":     liveAccount.Equity,
		"live_account_available":  liveAccount.AvailableBalance,
		"live_account_time":       liveAccount.SnapshotTime,
	})
	if err != nil {
		return paperRunSummary{}, fmt.Errorf("encode signal features: %w", err)
	}
	if saveObservation {
		runID, err = strategyRepo.StartRun(ctx, strategy.RunRecord{
			StrategyID: config.StrategyID,
			Mode:       "paper",
			Status:     strategy.RunStatusFinished,
			FinishedAt: time.Now().UTC(),
			ConfigJSON: configJSON,
		})
		if err != nil {
			return paperRunSummary{}, fmt.Errorf("start strategy run: %w", err)
		}
		if _, err := strategyRepo.SaveSignal(ctx, strategy.SignalRecord{
			StrategyID:      config.StrategyID,
			RunID:           runID,
			Exchange:        config.Exchange,
			MarketType:      config.MarketType,
			Symbol:          config.Symbol,
			Action:          action,
			Confidence:      assessment.Score,
			Reason:          paperSignalReason(config.StrategyType, assessment),
			RawFeaturesJSON: rawFeaturesJSON,
		}); err != nil {
			return paperRunSummary{}, fmt.Errorf("save signal: %w", err)
		}
		metricsJSON, err := marshalJSON(map[string]any{
			"total_return_pct":         result.TotalReturnPct,
			"excess_return_pct":        result.ExcessReturnPct,
			"trades":                   len(result.Trades),
			"win_rate_pct":             result.WinRatePct,
			"profile":                  config.Profile,
			"take_profit_pct":          config.TakeProfitPct,
			"stop_loss_pct":            config.StopLossPct,
			"dynamic_tpsl":             config.DynamicTPSL,
			"take_profit_atr_mult":     config.TakeProfitATRMult,
			"stop_loss_atr_mult":       config.StopLossATRMult,
			"min_take_profit_pct":      config.MinTakeProfitPct,
			"max_take_profit_pct":      config.MaxTakeProfitPct,
			"min_stop_loss_pct":        config.MinStopLossPct,
			"max_stop_loss_pct":        config.MaxStopLossPct,
			"breakeven_stop_enabled":   config.BreakevenStopEnabled,
			"breakeven_trigger_r":      config.BreakevenTriggerR,
			"trailing_stop_enabled":    config.TrailingStopEnabled,
			"trailing_activation_r":    config.TrailingActivationR,
			"trailing_atr_mult":        config.TrailingATRMult,
			"min_trend_spread_pct":     config.MinTrendSpreadPct,
			"confirm_bars":             config.ConfirmBars,
			"atr_window":               config.ATRWindow,
			"min_atr_pct":              config.MinATRPct,
			"max_atr_pct":              config.MaxATRPct,
			"volume_window":            config.VolumeWindow,
			"min_volume_ratio":         config.MinVolumeRatio,
			"max_entry_extension_pct":  config.MaxEntryExtensionPct,
			"pullback_lookback":        config.PullbackLookback,
			"pullback_tolerance_pct":   config.PullbackTolerancePct,
			"risk_pct":                 config.RiskPct,
			"max_notional_pct":         config.MaxNotionalPct,
			"max_margin_pct":           config.MaxMarginPct,
			"max_balance_use_pct":      config.MaxBalanceUsePct,
			"min_liq_distance_pct":     config.MinLiqDistancePct,
			"max_daily_loss_pct":       config.MaxDailyLossPct,
			"max_consecutive_losses":   config.MaxConsecutiveLosses,
			"daily_realized_pnl":       paperState.DailyRealizedPnL,
			"consecutive_losses":       paperState.ConsecutiveLosses,
			"max_abs_funding_rate_pct": config.MaxAbsFundingRatePct,
			"signal_filter_enabled":    config.SignalFilterEnabled,
			"min_signal_score":         config.MinSignalScore,
			"trend_filter_enabled":     config.TrendFilterEnabled,
			"trend_regime":             trendRegime.Regime,
			"trend_regime_reason":      trendRegime.Reason,
			"signal_score":             assessment.Score,
			"signal_allowed":           assessment.AllowEntry,
			"signal_filter_reason":     assessment.Reason,
			"price_source":             marketSnapshot.PriceSource,
			"price_time":               marketSnapshot.PriceTime,
			"funding_rate_pct":         marketSnapshot.LatestFundingRatePct,
			"funding_rate_time":        marketSnapshot.FundingRateTime,
			"funding_rate_source":      marketSnapshot.FundingRateSource,
		})
		if err != nil {
			return paperRunSummary{}, fmt.Errorf("encode performance metrics: %w", err)
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
			return paperRunSummary{}, fmt.Errorf("save performance: %w", err)
		}
		summary.RunID = runID
		summary.ObservationSaved = true
		summary.AccountSaved = options.SaveAccountSnapshot && shouldSavePaperAccountSnapshots(config)
	}

	if action != strategy.SignalHold {
		effectiveTPSL, err := paperEffectiveTPSL(signalCandles, config)
		if err != nil {
			return paperRunSummary{}, err
		}
		side, positionSide, stopPrice, takeProfitPrice, err := paperOrderPlan(action, latestPrice, effectiveTPSL.TakeProfitPct, effectiveTPSL.StopLossPct)
		if err != nil {
			return paperRunSummary{}, err
		}
		orderQuantity, err := paperOrderQuantity(latestPrice, stopPrice, config, paperState)
		if err != nil {
			return paperRunSummary{}, err
		}
		alignedPlan, err := alignPaperOrderPlan(positionSide, latestPrice, stopPrice, takeProfitPrice, orderQuantity, config)
		if err != nil {
			return paperRunSummary{}, err
		}
		latestPrice = alignedPlan.EntryPrice
		stopPrice = alignedPlan.StopPrice
		takeProfitPrice = alignedPlan.TakeProfitPrice
		orderQuantity = alignedPlan.Quantity
		liquidationPrice := risk.EstimateLiquidationPrice(latestPrice, side, config.Leverage, config.MaintMarginPct)
		dryRun, err := orderRepo.RecordDryRunOrder(ctx, execution.DryRunRequest{
			Account: paperRiskAccountSnapshot(config, paperState),
			Order: risk.OrderIntent{
				Exchange:             config.Exchange,
				MarketType:           risk.MarketType(config.MarketType),
				Symbol:               config.Symbol,
				Side:                 side,
				Price:                latestPrice,
				Quantity:             orderQuantity,
				StopPrice:            stopPrice,
				Leverage:             config.Leverage,
				LiquidationPrice:     liquidationPrice,
				LatestFundingRatePct: marketSnapshot.LatestFundingRatePct,
			},
			RiskConfig:      paperRiskConfig(config),
			StrategyID:      config.StrategyID,
			ClientOrderID:   fmt.Sprintf("paper-%s-%d", config.Symbol, time.Now().UTC().UnixNano()),
			OrderType:       "market",
			TimeInForce:     "IOC",
			PositionModel:   config.PositionModel,
			TakeProfitPrice: takeProfitPrice,
		})
		if err != nil {
			return paperRunSummary{}, fmt.Errorf("record paper dry-run order: %w", err)
		}
		if !strings.EqualFold(string(dryRun.Order.RiskDecision), string(risk.DecisionAllow)) {
			log.Printf("paper_order_id=%d status=%s decision=%s stop=%.8f take_profit=%.8f rejected_no_position=true", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
			return summary, nil
		}
		exchangeOrder, err := maybeSubmitOneBullExOrder(ctx, orderRepo, dryRun.Order, config)
		if err != nil {
			return paperRunSummary{}, err
		}
		opened, err := openPaperPosition(ctx, portfolioRepo, paperOpenRequest{
			AccountID:       config.AccountID,
			StrategyID:      config.StrategyID,
			Exchange:        config.Exchange,
			MarketType:      config.MarketType,
			Symbol:          config.Symbol,
			PositionSide:    positionSide,
			Quantity:        orderQuantity,
			EntryPrice:      latestPrice,
			TakeProfitPrice: takeProfitPrice,
			StopLossPrice:   stopPrice,
			Equity:          paperState.Equity,
			Leverage:        config.Leverage,
			MaintMarginPct:  config.MaintMarginPct,
			FeeRate:         config.FeeRate,
			SlippageRate:    config.SlippageRate,
			OpenedAt:        latestTime,
			SaveSnapshots:   shouldSavePaperAccountSnapshots(config),
		})
		if err != nil {
			return paperRunSummary{}, err
		}
		summary.OpenedOrder = true
		log.Printf("paper_order_id=%d status=%s decision=%s exchange_order_id=%s tpsl_source=%s stop=%.8f take_profit=%.8f", exchangeOrder.ID, exchangeOrder.Status, exchangeOrder.RiskDecision, exchangeOrder.ExchangeOrderID, effectiveTPSL.Source, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
		log.Printf("paper_position_id=%d status=open entry=%.8f mark=%.8f pnl=%.8f", opened.ID, opened.EntryPrice, opened.MarkPrice, paperPositionNetPnL(opened, latestPrice, config.FeeRate, config.SlippageRate))
	}

	logPaperTick(runID, backtestRunID, summary, action, candidateAction, assessment, trendRegime, marketSnapshot, liveAccount, paperState, config, ignoredOpenCandles, result)
	return summary, nil
}

func logPaperTick(runID int64, backtestRunID int64, summary paperRunSummary, action strategy.SignalAction, candidate strategy.SignalAction, assessment paperSignalAssessment, trendRegime paperTrendRegime, snapshot paperMarketSnapshot, liveAccount liveAccountBalanceOverride, paperState paperAccountState, config paperRunConfig, ignoredOpenCandles int, result backtest.Result) {
	features := assessment.Features
	accountSource := liveAccount.Source
	accountEquity := paperState.Equity
	accountAvailable := paperState.AvailableBalance
	if liveAccount.Applied {
		accountEquity = liveAccount.Equity
		accountAvailable = liveAccount.AvailableBalance
	}
	log.Printf(
		"paper_tick run_id=%d backtest_run_id=%d observation_saved=%t backtest_saved=%t action=%s candidate=%s reason=%s score=%.4f allowed=%t blockers=%v trend=%s trend_reason=%s trend_long=%t trend_short=%t spread=%.4f min_spread=%.4f atr=%.4f min_atr=%.4f volume=%.4f min_volume=%.4f ignored_open_candles=%d price_source=%s signal_candle_close=%s account_source=%s account_equity=%.8f account_available=%.8f strategy=%s symbol=%s return_pct=%.4f excess_pct=%.4f drawdown_pct=%.4f backtest_trades=%d",
		runID,
		backtestRunID,
		summary.ObservationSaved,
		summary.BacktestSaved,
		action,
		candidate,
		assessment.Reason,
		assessment.Score,
		assessment.AllowEntry,
		features["entry_blockers"],
		trendRegime.Regime,
		trendRegime.Reason,
		trendRegime.AllowLong,
		trendRegime.AllowShort,
		logFeatureFloat(features, "trend_spread_pct"),
		config.MinTrendSpreadPct,
		logFeatureFloat(features, "atr_pct"),
		config.MinATRPct,
		logFeatureFloat(features, "volume_ratio"),
		config.MinVolumeRatio,
		ignoredOpenCandles,
		snapshot.PriceSource,
		logTime(snapshot.CandleCloseTime),
		accountSource,
		accountEquity,
		accountAvailable,
		config.StrategyID,
		config.Symbol,
		result.TotalReturnPct,
		result.ExcessReturnPct,
		result.MaxDrawdownPct,
		len(result.Trades),
	)
}

type paperRiskHalt struct {
	Reason  string
	Message string
}

func paperEntryRiskHalt(config paperRunConfig, state paperAccountState) paperRiskHalt {
	equity := state.Equity
	if equity <= 0 {
		equity = config.Equity
	}
	if equity > 0 && config.MaxDailyLossPct > 0 && state.DailyRealizedPnL < 0 {
		lossPct := -state.DailyRealizedPnL / equity * 100
		if lossPct >= config.MaxDailyLossPct {
			return paperRiskHalt{
				Reason:  "daily_loss_halt",
				Message: fmt.Sprintf("daily loss %.2f%% reached limit %.2f%%", lossPct, config.MaxDailyLossPct),
			}
		}
	}
	if config.MaxConsecutiveLosses > 0 && state.ConsecutiveLosses >= config.MaxConsecutiveLosses {
		return paperRiskHalt{
			Reason:  "consecutive_loss_halt",
			Message: fmt.Sprintf("consecutive losses %d reached limit %d", state.ConsecutiveLosses, config.MaxConsecutiveLosses),
		}
	}
	return paperRiskHalt{}
}

func appendEntryBlocker(features map[string]any, blocker string) {
	if features == nil || blocker == "" {
		return
	}
	switch existing := features["entry_blockers"].(type) {
	case []string:
		for _, value := range existing {
			if value == blocker {
				return
			}
		}
		features["entry_blockers"] = append(existing, blocker)
	case []any:
		for _, value := range existing {
			if text, ok := value.(string); ok && text == blocker {
				return
			}
		}
		features["entry_blockers"] = append(existing, blocker)
	default:
		features["entry_blockers"] = []string{blocker}
	}
}

func logFeatureFloat(features map[string]any, key string) float64 {
	if features == nil {
		return 0
	}
	switch value := features[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func logTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
