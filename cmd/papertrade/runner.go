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
		return fmt.Errorf("encode strategy config: %w", err)
	}
	marketSnapshot, err := latestPaperMarketSnapshot(ctx, mdRepo, config, candles)
	if err != nil {
		return err
	}
	trendRegime, err := loadPaperTrendRegime(ctx, mdRepo, config)
	if err != nil {
		return err
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
		Candles:         candles,
		AllowPaperState: true,
	})
	if err != nil {
		return err
	}
	orderRepo := execution.NewSQLiteRepository(db)
	if paperState.CloseNote != "" && paperState.Position.ID != 0 {
		closeOrder, err := recordPaperCloseOrder(ctx, orderRepo, config, paperState, latestPrice, latestTime)
		if err != nil {
			return err
		}
		closeOrder, err = maybeSubmitOneBullExOrder(ctx, orderRepo, closeOrder, config)
		if err != nil {
			return err
		}
		fmt.Printf("paper_close_order_id=%d status=%s decision=%s exchange_order_id=%s reason=%s\n", closeOrder.ID, closeOrder.Status, closeOrder.RiskDecision, closeOrder.ExchangeOrderID, paperState.CloseNote)
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
	assessment := assessPaperSignal(candles, result, signal, marketSnapshot, config)
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
	assessment.Features["trend_regime"] = trendRegime.Regime
	assessment.Features["trend_regime_reason"] = trendRegime.Reason
	assessment.Features["trend_filter_enabled"] = trendRegime.Enabled
	assessment.Features["trend_allow_long"] = trendRegime.AllowLong
	assessment.Features["trend_allow_short"] = trendRegime.AllowShort
	assessment.Features["trend_timeframe"] = trendRegime.TrendInterval
	assessment.Features["macro_trend_timeframe"] = trendRegime.MacroInterval
	assessment.Features["trend_spread_pct_htf"] = nullableFeature(trendRegime.TrendSpreadPct, trendRegime.Available)
	assessment.Features["macro_trend_spread_pct"] = nullableFeature(trendRegime.MacroSpreadPct, trendRegime.MacroAvailable)
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
		Confidence:      assessment.Score,
		Reason:          paperSignalReason(config.StrategyType, assessment),
		RawFeaturesJSON: rawFeaturesJSON,
	}); err != nil {
		return fmt.Errorf("save signal: %w", err)
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
		effectiveTPSL, err := paperEffectiveTPSL(candles, config)
		if err != nil {
			return err
		}
		side, positionSide, stopPrice, takeProfitPrice, err := paperOrderPlan(action, latestPrice, effectiveTPSL.TakeProfitPct, effectiveTPSL.StopLossPct)
		if err != nil {
			return err
		}
		orderQuantity, err := paperOrderQuantity(latestPrice, stopPrice, config, paperState)
		if err != nil {
			return err
		}
		alignedPlan, err := alignPaperOrderPlan(positionSide, latestPrice, stopPrice, takeProfitPrice, orderQuantity, config)
		if err != nil {
			return err
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
			return fmt.Errorf("record paper dry-run order: %w", err)
		}
		if !strings.EqualFold(string(dryRun.Order.RiskDecision), string(risk.DecisionAllow)) {
			fmt.Printf("paper_order_id=%d status=%s decision=%s stop=%.8f take_profit=%.8f rejected_no_position=true\n", dryRun.Order.ID, dryRun.Order.Status, dryRun.Order.RiskDecision, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
			return nil
		}
		exchangeOrder, err := maybeSubmitOneBullExOrder(ctx, orderRepo, dryRun.Order, config)
		if err != nil {
			return err
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
		})
		if err != nil {
			return err
		}
		fmt.Printf("paper_order_id=%d status=%s decision=%s exchange_order_id=%s tpsl_source=%s stop=%.8f take_profit=%.8f\n", exchangeOrder.ID, exchangeOrder.Status, exchangeOrder.RiskDecision, exchangeOrder.ExchangeOrderID, effectiveTPSL.Source, dryRun.Order.StopPrice, dryRun.Order.TakeProfitPrice)
		fmt.Printf("paper_position_id=%d status=open entry=%.8f mark=%.8f pnl=%.8f\n", opened.ID, opened.EntryPrice, opened.MarkPrice, paperPositionNetPnL(opened, latestPrice, config.FeeRate, config.SlippageRate))
	}

	fmt.Printf("paper_run_id=%d backtest_run_id=%d strategy=%s symbol=%s return_pct=%.4f excess_pct=%.4f drawdown_pct=%.4f trades=%d\n", runID, backtestRunID, config.StrategyID, config.Symbol, result.TotalReturnPct, result.ExcessReturnPct, result.MaxDrawdownPct, len(result.Trades))
	return nil
}
