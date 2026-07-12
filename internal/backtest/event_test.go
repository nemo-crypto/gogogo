package backtest

import (
	"math"
	"testing"
)

func TestCostModel(t *testing.T) {
	t.Parallel()

	model := CostModel{FeeRate: 0.001, SlippageRate: 0.0005, MinNotional: 10, FundingRate: 0.0001}
	if cost := model.EntryCost(1000); cost != 1.5 {
		t.Fatalf("entry cost = %f, want 1.5", cost)
	}
	if !model.MeetsMinimum(10) {
		t.Fatal("meets minimum = false, want true")
	}
	if model.MeetsMinimum(9.99) {
		t.Fatal("meets minimum = true, want false")
	}
	if cost := model.FundingCost(1000, 3); math.Abs(cost-0.3) > 0.0000001 {
		t.Fatalf("funding cost = %f, want 0.3", cost)
	}
}
