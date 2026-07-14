package risk

import "testing"

func TestEvaluateOrderAllowsConservativeSpotOrder(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID:             "research",
		Equity:                10_000,
		CurrentTotalExposure:  2_000,
		CurrentSymbolExposure: 1_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypeSpot,
		Symbol:     "btcusdt",
		Side:       SideBuy,
		Price:      60_000,
		Quantity:   0.02,
		StopPrice:  58_000,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("decision = %s, want allow, events=%v", result.Decision, result.Events)
	}
	if result.OrderNotional != 1_200 {
		t.Fatalf("order notional = %f, want 1200", result.OrderNotional)
	}
	if result.OrderRisk != 40 {
		t.Fatalf("order risk = %f, want 40", result.OrderRisk)
	}
}

func TestEvaluateOrderRejectsExposureLimit(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID:             "research",
		Equity:                10_000,
		CurrentTotalExposure:  9_000,
		CurrentSymbolExposure: 2_900,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypeSpot,
		Symbol:     "ETHUSDT",
		Side:       SideBuy,
		Price:      3_000,
		Quantity:   1,
		StopPrice:  2_950,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionReject {
		t.Fatalf("decision = %s, want reject", result.Decision)
	}
	if !hasEvent(result, "symbol_exposure_limit") {
		t.Fatalf("missing symbol exposure event: %v", result.Events)
	}
	if !hasEvent(result, "total_exposure_limit") {
		t.Fatalf("missing total exposure event: %v", result.Events)
	}
}

func TestEvaluateOrderReduceOnlyBypassesExposureIncrease(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID:             "research",
		Equity:                10_000,
		CurrentTotalExposure:  12_000,
		CurrentSymbolExposure: 5_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Side:       SideSell,
		Price:      60_000,
		Quantity:   0.05,
		Leverage:   2,
		ReduceOnly: true,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("decision = %s, want allow, events=%v", result.Decision, result.Events)
	}
}

func TestEvaluateOrderRejectsPerpetualLeverageAndLiquidationRisk(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID: "research",
		Equity:    10_000,
	}, OrderIntent{
		Exchange:             "onebullex",
		MarketType:           MarketTypePerpetual,
		Symbol:               "SOLUSDT",
		Side:                 SideBuy,
		Price:                150,
		Quantity:             10,
		StopPrice:            148,
		Leverage:             5,
		LiquidationPrice:     140,
		LatestFundingRatePct: 0.08,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionReject {
		t.Fatalf("decision = %s, want reject", result.Decision)
	}
	for _, eventType := range []string{"leverage_limit", "liquidation_distance_limit", "funding_rate_limit"} {
		if !hasEvent(result, eventType) {
			t.Fatalf("missing event %s: %v", eventType, result.Events)
		}
	}
}

func TestEvaluateOrderRejectsPerpetualMarginAndAvailableBalanceLimits(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.MaxSymbolExposurePct = 200
	config.MaxTotalExposurePct = 200
	config.MaxInitialMarginPct = 35
	config.MaxAvailableBalanceUsePct = 50
	result, err := EvaluateOrder(config, AccountSnapshot{
		AccountID:            "research",
		Equity:               10_000,
		AvailableBalance:     1_000,
		CurrentInitialMargin: 3_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Side:       SideBuy,
		Price:      60_000,
		Quantity:   0.02,
		StopPrice:  59_500,
		Leverage:   2,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionReject {
		t.Fatalf("decision = %s, want reject", result.Decision)
	}
	for _, eventType := range []string{"available_balance_use_limit", "initial_margin_limit"} {
		if !hasEvent(result, eventType) {
			t.Fatalf("missing event %s: %v", eventType, result.Events)
		}
	}
	if result.OrderInitialMargin != 600 {
		t.Fatalf("order initial margin = %f, want 600", result.OrderInitialMargin)
	}
}

func TestEvaluateOrderRejectsExchangeRuleViolations(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.MinQuantity = 0.001
	config.QuantityStep = 0.001
	config.PriceTickSize = 0.1
	result, err := EvaluateOrder(config, AccountSnapshot{
		AccountID:        "research",
		Equity:           10_000,
		AvailableBalance: 10_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Side:       SideBuy,
		Price:      60000.03,
		Quantity:   0.0009,
		StopPrice:  59899.97,
		Leverage:   2,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionReject {
		t.Fatalf("decision = %s, want reject", result.Decision)
	}
	for _, eventType := range []string{"min_quantity_limit", "quantity_step_limit", "price_tick_limit", "stop_price_tick_limit"} {
		if !hasEvent(result, eventType) {
			t.Fatalf("missing event %s: %v", eventType, result.Events)
		}
	}
}

func TestEvaluateOrderUsesEstimatedLiquidationPrice(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.MaxSymbolExposurePct = 200
	config.MinLiquidationDistancePct = 40
	result, err := EvaluateOrder(config, AccountSnapshot{
		AccountID: "research",
		Equity:    10_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Side:       SideSell,
		Price:      60_000,
		Quantity:   0.01,
		StopPrice:  60_500,
		Leverage:   3,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionReject {
		t.Fatalf("decision = %s, want reject", result.Decision)
	}
	if !hasEvent(result, "liquidation_distance_limit") {
		t.Fatalf("missing liquidation distance event: %v", result.Events)
	}
}

func TestEvaluateOrderHaltsOnDailyLoss(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID:        "research",
		Equity:           10_000,
		DailyRealizedPnL: -250,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypeSpot,
		Symbol:     "BNBUSDT",
		Side:       SideBuy,
		Price:      500,
		Quantity:   1,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionHalt {
		t.Fatalf("decision = %s, want halt", result.Decision)
	}
	if !hasEvent(result, "daily_loss_halt") {
		t.Fatalf("missing daily loss event: %v", result.Events)
	}
}

func TestEvaluateOrderAllowsReduceOnlyDuringDailyLossHalt(t *testing.T) {
	t.Parallel()

	result, err := EvaluateOrder(DefaultConfig(), AccountSnapshot{
		AccountID:             "research",
		Equity:                10_000,
		DailyRealizedPnL:      -250,
		ConsecutiveLosses:     3,
		CurrentTotalExposure:  2_000,
		CurrentSymbolExposure: 2_000,
	}, OrderIntent{
		Exchange:   "onebullex",
		MarketType: MarketTypePerpetual,
		Symbol:     "BTCUSDT",
		Side:       SideSell,
		Price:      60_000,
		Quantity:   0.01,
		Leverage:   2,
		ReduceOnly: true,
	})
	if err != nil {
		t.Fatalf("evaluate order: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("decision = %s, want allow, events=%v", result.Decision, result.Events)
	}
}

func hasEvent(result Result, eventType string) bool {
	for _, event := range result.Events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
