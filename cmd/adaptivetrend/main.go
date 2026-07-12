package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
)

func main() {
	var (
		dsn                 = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange            = flag.String("exchange", "binance", "exchange name")
		marketType          = flag.String("market", "spot", "market type: spot or perpetual")
		symbols             = flag.String("symbol", "BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT", "comma-separated symbols")
		interval            = flag.String("interval", "1h", "kline interval")
		start               = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end                 = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		momentumWindow      = flag.Int("momentum-window", 24, "momentum lookback in candles")
		trendWindow         = flag.Int("trend-window", 72, "trend SMA window in candles")
		breakoutWindow      = flag.Int("breakout-window", 48, "breakout lookback in candles")
		volatilityWindow    = flag.Int("volatility-window", 24, "realized volatility window in candles")
		rebalanceWindow     = flag.Int("rebalance-window", 6, "rebalance interval in candles")
		topN                = flag.Int("top-n", 2, "number of strongest symbols to hold")
		targetVolatilityPct = flag.Float64("target-volatility-pct", 1, "target per-candle volatility pct for position sizing")
		maxPositionPct      = flag.Float64("max-position-pct", 30, "max position pct per symbol")
		trailingStopPct     = flag.Float64("trailing-stop-pct", 6, "trailing stop pct from position peak")
		maxFundingRatePct   = flag.Float64("max-funding-rate-pct", 0.05, "skip long entries when latest funding pct is above this value")
		feeRate             = flag.Float64("fee-rate", 0.001, "fee rate per entry/exit")
		slippageRate        = flag.Float64("slippage-rate", 0.0005, "slippage rate per entry/exit")
		save                = flag.Bool("save", true, "save result into backtest_runs")
	)
	flag.Parse()

	startTime, err := time.Parse(time.RFC3339, *start)
	if err != nil {
		log.Fatalf("parse start: %v", err)
	}
	endTime, err := time.Parse(time.RFC3339, *end)
	if err != nil {
		log.Fatalf("parse end: %v", err)
	}
	parsedMarketType, err := parseMarketType(*marketType)
	if err != nil {
		log.Fatal(err)
	}
	parsedSymbols := parseSymbols(*symbols)
	if len(parsedSymbols) == 0 {
		log.Fatal("at least one symbol is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()
	if err := backtest.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init backtest schema: %v", err)
	}

	mdRepo := marketdata.NewSQLiteRepository(db)
	candlesBySymbol := make(map[string][]marketdata.Candle, len(parsedSymbols))
	fundingBySymbol := make(map[string][]marketdata.FundingRate, len(parsedSymbols))
	for _, symbol := range parsedSymbols {
		candles, err := mdRepo.ListCandlesFull(ctx, marketdata.CandleQuery{
			Exchange:   *exchange,
			MarketType: parsedMarketType,
			Symbol:     symbol,
			Interval:   *interval,
			Start:      startTime,
			End:        endTime,
		})
		if err != nil {
			log.Fatalf("list candles %s: %v", symbol, err)
		}
		if len(candles) == 0 {
			log.Printf("skip %s: no candles", symbol)
			continue
		}
		candlesBySymbol[symbol] = candles

		rates, err := mdRepo.ListFundingRates(ctx, marketdata.FundingRateQuery{
			Exchange: *exchange,
			Symbol:   symbol,
			Start:    startTime,
			End:      endTime,
			Limit:    10000,
		})
		if err != nil {
			log.Fatalf("list funding rates %s: %v", symbol, err)
		}
		fundingBySymbol[symbol] = rates
	}

	result, err := backtest.RunAdaptiveTrendRotation(candlesBySymbol, fundingBySymbol, backtest.AdaptiveTrendConfig{
		MomentumWindow:      *momentumWindow,
		TrendWindow:         *trendWindow,
		BreakoutWindow:      *breakoutWindow,
		VolatilityWindow:    *volatilityWindow,
		RebalanceWindow:     *rebalanceWindow,
		TopN:                *topN,
		TargetVolatilityPct: *targetVolatilityPct,
		MaxPositionPct:      *maxPositionPct,
		TrailingStopPct:     *trailingStopPct,
		MaxFundingRatePct:   *maxFundingRatePct,
		FeeRate:             *feeRate,
		SlippageRate:        *slippageRate,
	})
	if err != nil {
		if errors.Is(err, backtest.ErrNotEnoughData) {
			log.Fatalf("not enough aligned candles for adaptive trend strategy: %v", err)
		}
		log.Fatalf("run adaptive trend strategy: %v", err)
	}

	runID := int64(0)
	if *save {
		backtestRepo := backtest.NewSQLiteRepository(db)
		runID, err = backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
			Exchange:   *exchange,
			MarketType: string(parsedMarketType),
			Config: backtest.SMAConfig{
				FastWindow: *momentumWindow,
				SlowWindow: *trendWindow,
				FeeRate:    *feeRate,
			},
			Result: result,
		})
		if err != nil {
			log.Fatalf("save adaptive trend run: %v", err)
		}
	}

	printResult(runID, result)
}

func printResult(runID int64, result backtest.Result) {
	if runID > 0 {
		fmt.Printf("run_id=%d\n", runID)
	}
	fmt.Printf("strategy=%s\n", result.StrategyName)
	fmt.Printf("symbols=%s interval=%s\n", result.Symbol, result.Interval)
	fmt.Printf("range=%s -> %s\n", result.Start.Format(time.RFC3339), result.End.Format(time.RFC3339))
	fmt.Printf("initial_equity=%.4f final_equity=%.4f\n", result.InitialEquity, result.FinalEquity)
	fmt.Printf("total_return_pct=%.4f\n", result.TotalReturnPct)
	fmt.Printf("buy_hold_return_pct=%.4f excess_return_pct=%.4f\n", result.BuyHoldReturnPct, result.ExcessReturnPct)
	fmt.Printf("max_drawdown_pct=%.4f\n", result.MaxDrawdownPct)
	fmt.Printf("trades=%d win_rate_pct=%.2f\n", len(result.Trades), result.WinRatePct)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func parseMarketType(value string) (marketdata.MarketType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "spot":
		return marketdata.MarketTypeSpot, nil
	case "perpetual", "futures", "future":
		return marketdata.MarketTypePerpetual, nil
	default:
		return "", fmt.Errorf("unsupported market type %q", value)
	}
}

func parseSymbols(value string) []string {
	parts := strings.Split(value, ",")
	symbols := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	return symbols
}
