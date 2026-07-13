package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gogogo/internal/config"
	"log"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/marketdata"
)

func main() {
	var (
		dsn           = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		exchange      = flag.String("exchange", env("EXCHANGE_NAME", "onebullex"), "exchange name")
		marketType    = flag.String("market", "perpetual", "market type: perpetual")
		symbols       = flag.String("symbol", "BTCUSDT", "symbol or comma-separated symbols")
		interval      = flag.String("interval", "5m", "kline interval")
		start         = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time in RFC3339")
		end           = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time in RFC3339")
		strategyType  = flag.String("strategy-type", "scalp-tpsl", "strategy type: scalp-tpsl or sma")
		fast          = flag.String("fast", "3", "fast SMA window or comma-separated windows")
		slow          = flag.String("slow", "9", "slow SMA window or comma-separated windows")
		takeProfitPct = flag.Float64("take-profit-pct", 0.80, "fixed/fallback take profit pct for scalp-tpsl")
		stopLossPct   = flag.Float64("stop-loss-pct", 0.45, "fixed/fallback stop loss pct for scalp-tpsl")
		dynamicTPSL   = flag.Bool("dynamic-tpsl", true, "use ATR-based dynamic take profit and stop loss")
		takeATRMult   = flag.Float64("take-profit-atr-mult", 1.6, "ATR multiplier for dynamic take profit")
		stopATRMult   = flag.Float64("stop-loss-atr-mult", 1.0, "ATR multiplier for dynamic stop loss")
		minTPPct      = flag.Float64("min-take-profit-pct", 0.55, "minimum dynamic take profit pct")
		maxTPPct      = flag.Float64("max-take-profit-pct", 1.40, "maximum dynamic take profit pct; 0 disables cap")
		minSLPct      = flag.Float64("min-stop-loss-pct", 0.30, "minimum dynamic stop loss pct")
		maxSLPct      = flag.Float64("max-stop-loss-pct", 0.75, "maximum dynamic stop loss pct; 0 disables cap")
		cooldownBars  = flag.Int("cooldown-bars", 1, "cooldown bars after scalp-tpsl exit")
		minSpreadPct  = flag.Float64("min-trend-spread-pct", 0.03, "minimum SMA spread pct required to enter scalp-tpsl trades")
		confirmBars   = flag.Int("confirm-bars", 1, "consecutive close direction bars required to enter scalp-tpsl trades")
		atrWindow     = flag.Int("atr-window", 14, "ATR window for scalp-tpsl volatility filter")
		minATRPct     = flag.Float64("min-atr-pct", 0.08, "minimum ATR pct required to enter scalp-tpsl trades")
		maxATRPct     = flag.Float64("max-atr-pct", 1.6, "maximum ATR pct allowed to enter scalp-tpsl trades")
		volumeWindow  = flag.Int("volume-window", 20, "volume average window for scalp-tpsl volume filter")
		minVolume     = flag.Float64("min-volume-ratio", 1.10, "minimum current volume / average volume required to enter scalp-tpsl trades")
		maxExtension  = flag.Float64("max-entry-extension-pct", 0.18, "maximum entry distance from fast SMA pct; zero disables")
		pullbackBars  = flag.Int("pullback-lookback", 5, "recent bars that must touch fast SMA zone before entry; zero disables")
		pullbackTol   = flag.Float64("pullback-tolerance-pct", 0.06, "pullback touch tolerance pct around fast SMA")
		slippageRate  = flag.Float64("slippage-rate", 0.0005, "slippage rate per trade side for scalp-tpsl")
		feeRate       = flag.Float64("fee-rate", 0.0005, "fee rate per entry/exit, e.g. 0.001 = 0.1%")
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
	fastWindows, err := parseInts(*fast)
	if err != nil {
		log.Fatalf("parse fast: %v", err)
	}
	slowWindows, err := parseInts(*slow)
	if err != nil {
		log.Fatalf("parse slow: %v", err)
	}
	configs := buildConfigs(fastWindows, slowWindows, *feeRate)
	if len(configs) == 0 {
		log.Fatal("no valid SMA configs; fast windows must be less than slow windows")
	}
	parsedStrategyType := normalizedStrategyType(*strategyType)

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

	repo := marketdata.NewSQLiteRepository(db)
	backtestRepo := backtest.NewSQLiteRepository(db)

	for _, symbol := range parsedSymbols {
		candles, err := repo.ListCandles(ctx, marketdata.CandleQuery{
			Exchange:   *exchange,
			MarketType: parsedMarketType,
			Symbol:     symbol,
			Interval:   *interval,
			Start:      startTime,
			End:        endTime,
			Limit:      10000,
		})
		if err != nil {
			log.Fatalf("list candles %s: %v", symbol, err)
		}

		for _, config := range configs {
			result, err := runBacktestStrategy(candles, parsedStrategyType, config, backtest.ScalpTPSLConfig{
				FastWindow:           config.FastWindow,
				SlowWindow:           config.SlowWindow,
				TakeProfitPct:        *takeProfitPct,
				StopLossPct:          *stopLossPct,
				DynamicTPSL:          *dynamicTPSL,
				TakeProfitATRMult:    *takeATRMult,
				StopLossATRMult:      *stopATRMult,
				MinTakeProfitPct:     *minTPPct,
				MaxTakeProfitPct:     *maxTPPct,
				MinStopLossPct:       *minSLPct,
				MaxStopLossPct:       *maxSLPct,
				CooldownBars:         *cooldownBars,
				FeeRate:              *feeRate,
				SlippageRate:         *slippageRate,
				AllowShort:           true,
				MinTrendSpreadPct:    *minSpreadPct,
				ConfirmBars:          *confirmBars,
				ATRWindow:            *atrWindow,
				MinATRPct:            *minATRPct,
				MaxATRPct:            *maxATRPct,
				VolumeWindow:         *volumeWindow,
				MinVolumeRatio:       *minVolume,
				MaxEntryExtensionPct: *maxExtension,
				PullbackLookback:     *pullbackBars,
				PullbackTolerancePct: *pullbackTol,
			})
			if err != nil {
				if errors.Is(err, backtest.ErrNotEnoughData) {
					log.Printf("skip %s fast=%d slow=%d: not enough candles got=%d", symbol, config.FastWindow, config.SlowWindow, len(candles))
					continue
				}
				log.Fatalf("run backtest %s fast=%d slow=%d: %v", symbol, config.FastWindow, config.SlowWindow, err)
			}

			runID, err := backtestRepo.SaveRun(ctx, backtest.SaveRunRequest{
				Exchange:   *exchange,
				MarketType: string(parsedMarketType),
				Config:     config,
				Result:     result,
			})
			if err != nil {
				log.Fatalf("save backtest run %s fast=%d slow=%d: %v", symbol, config.FastWindow, config.SlowWindow, err)
			}

			printResult(runID, result)
		}
	}
}

func printResult(runID int64, result backtest.Result) {
	fmt.Printf("run_id=%d\n", runID)
	fmt.Printf("strategy=%s\n", result.StrategyName)
	fmt.Printf("symbol=%s interval=%s\n", result.Symbol, result.Interval)
	fmt.Printf("range=%s -> %s\n", result.Start.Format(time.RFC3339), result.End.Format(time.RFC3339))
	fmt.Printf("initial_equity=%.4f final_equity=%.4f\n", result.InitialEquity, result.FinalEquity)
	fmt.Printf("total_return_pct=%.4f\n", result.TotalReturnPct)
	fmt.Printf("max_drawdown_pct=%.4f\n", result.MaxDrawdownPct)
	fmt.Printf("trades=%d win_rate_pct=%.2f\n", len(result.Trades), result.WinRatePct)
	fmt.Println()
}

func runBacktestStrategy(candles []marketdata.Candle, strategyType string, smaConfig backtest.SMAConfig, scalpConfig backtest.ScalpTPSLConfig) (backtest.Result, error) {
	switch strategyType {
	case "scalp-tpsl":
		return backtest.RunScalpTPSL(candles, scalpConfig)
	case "sma":
		return backtest.RunSMACrossover(candles, smaConfig)
	default:
		return backtest.Result{}, fmt.Errorf("unsupported strategy type %q", strategyType)
	}
}

func normalizedStrategyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "scalp", "scalp_tpsl", "scalp-tpsl", "tpsl":
		return "scalp-tpsl"
	case "sma", "sma-crossover", "sma_crossover":
		return "sma"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func env(key string, fallback string) string {
	return config.Env(key, fallback)
}

func parseMarketType(value string) (marketdata.MarketType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "perpetual", "futures", "future":
		return marketdata.MarketTypePerpetual, nil
	default:
		return "", fmt.Errorf("unsupported market type %q: current strategy only supports perpetual", value)
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

func parseInts(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	values := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var parsed int
		if _, err := fmt.Sscanf(part, "%d", &parsed); err != nil {
			return nil, fmt.Errorf("invalid integer %q", part)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("window must be positive: %d", parsed)
		}
		if _, ok := seen[parsed]; ok {
			continue
		}
		seen[parsed] = struct{}{}
		values = append(values, parsed)
	}
	return values, nil
}

func buildConfigs(fastWindows []int, slowWindows []int, feeRate float64) []backtest.SMAConfig {
	configs := make([]backtest.SMAConfig, 0, len(fastWindows)*len(slowWindows))
	for _, fast := range fastWindows {
		for _, slow := range slowWindows {
			if fast >= slow {
				continue
			}
			configs = append(configs, backtest.SMAConfig{
				FastWindow: fast,
				SlowWindow: slow,
				FeeRate:    feeRate,
			})
		}
	}
	return configs
}
