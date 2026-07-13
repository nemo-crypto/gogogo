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
	"strconv"
	"strings"
	"time"

	"gogogo/internal/backtest"
	"gogogo/internal/config"
	"gogogo/internal/exchange/onebullex"
	"gogogo/internal/execution"
	"gogogo/internal/marketdata"
	"gogogo/internal/portfolio"
	"gogogo/internal/risk"
	"gogogo/internal/storage"
	"gogogo/internal/strategy"
)

const (
	defaultPaperStrategyID      = "perp-trend-scalp-v2-paper"
	defaultPaperInterval        = "5m"
	defaultPaperFastWindow      = 3
	defaultPaperSlowWindow      = 9
	defaultPaperTakeProfitPct   = 0.80
	defaultPaperStopLossPct     = 0.45
	defaultPaperDynamicTPSL     = true
	defaultPaperTakeATRMult     = 1.6
	defaultPaperStopATRMult     = 1.0
	defaultPaperMinTakeProfit   = 0.55
	defaultPaperMaxTakeProfit   = 1.40
	defaultPaperMinStopLoss     = 0.30
	defaultPaperMaxStopLoss     = 0.75
	defaultPaperCooldownBars    = 1
	defaultPaperFeeRate         = 0.0005
	defaultPaperSlippageRate    = 0.0005
	defaultPaperRiskPct         = 1
	defaultPaperQuantity        = 0.001
	defaultPaperMaxDataAge      = 0
	defaultPaperMaxCandleAge    = 7 * time.Minute
	defaultPaperMaxMarkAge      = 45 * time.Second
	defaultPaperMinQuantity     = 0.001
	defaultPaperQuantityStep    = 0.001
	defaultPaperPriceTickSize   = 0.1
	defaultPaperMinTrendSpread  = 0.03
	defaultPaperATRWindow       = 14
	defaultPaperMinATRPct       = 0.08
	defaultPaperMaxATRPct       = 1.6
	defaultPaperVolumeWindow    = 20
	defaultPaperMinVolumeRatio  = 1.10
	defaultPaperMaxExtensionPct = 0.18
	defaultPaperPullbackBars    = 5
	defaultPaperPullbackTolPct  = 0.06
	defaultPaperSignalFilter    = true
	defaultPaperMinSignalScore  = 0.55
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

type paperProfileFlags struct {
	strategyID    *string
	market        *string
	interval      *string
	strategyType  *string
	fast          *int
	slow          *int
	takeProfitPct *float64
	stopLossPct   *float64
	dynamicTPSL   *bool
	takeATRMult   *float64
	stopATRMult   *float64
	minTPPct      *float64
	maxTPPct      *float64
	minSLPct      *float64
	maxSLPct      *float64
	cooldownBars  *int
	minSpreadPct  *float64
	confirmBars   *int
	atrWindow     *int
	minATRPct     *float64
	maxATRPct     *float64
	volumeWindow  *int
	minVolume     *float64
	maxExtension  *float64
	pullbackBars  *int
	pullbackTol   *float64
	feeRate       *float64
	slippageRate  *float64
	riskPct       *float64
	maxNotional   *float64
	maxMargin     *float64
	maxBalanceUse *float64
	minLiqDist    *float64
	maxOrderRisk  *float64
	maxLeverage   *float64
	leverage      *float64
	signalFilter  *bool
	minSignal     *float64
}

func run() error {
	var (
		dsn            = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		accountID      = flag.String("account", "paper", "paper account id")
		strategyID     = flag.String("strategy", defaultPaperStrategyID, "strategy id")
		profile        = flag.String("profile", "", "paper strategy profile: aggressive or empty/manual")
		exchange       = flag.String("exchange", env("EXCHANGE_NAME", onebullex.ExchangeName), "exchange")
		market         = flag.String("market", "perpetual", "market type")
		symbol         = flag.String("symbol", "BTCUSDT", "symbol")
		interval       = flag.String("interval", defaultPaperInterval, "interval")
		start          = flag.String("start", time.Now().UTC().Add(-30*24*time.Hour).Format(time.RFC3339), "start time")
		end            = flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time")
		strategyType   = flag.String("strategy-type", "scalp-tpsl", "strategy type: scalp-tpsl or sma")
		fast           = flag.Int("fast", defaultPaperFastWindow, "fast SMA")
		slow           = flag.Int("slow", defaultPaperSlowWindow, "slow SMA")
		takeProfitPct  = flag.Float64("take-profit-pct", defaultPaperTakeProfitPct, "take profit pct for scalp-tpsl")
		stopLossPct    = flag.Float64("stop-loss-pct", defaultPaperStopLossPct, "stop loss pct for scalp-tpsl")
		dynamicTPSL    = flag.Bool("dynamic-tpsl", defaultPaperDynamicTPSL, "use ATR-based dynamic take profit and stop loss")
		takeATRMult    = flag.Float64("take-profit-atr-mult", defaultPaperTakeATRMult, "ATR multiplier for dynamic take profit")
		stopATRMult    = flag.Float64("stop-loss-atr-mult", defaultPaperStopATRMult, "ATR multiplier for dynamic stop loss")
		minTPPct       = flag.Float64("min-take-profit-pct", defaultPaperMinTakeProfit, "minimum dynamic take profit pct")
		maxTPPct       = flag.Float64("max-take-profit-pct", defaultPaperMaxTakeProfit, "maximum dynamic take profit pct; 0 disables cap")
		minSLPct       = flag.Float64("min-stop-loss-pct", defaultPaperMinStopLoss, "minimum dynamic stop loss pct")
		maxSLPct       = flag.Float64("max-stop-loss-pct", defaultPaperMaxStopLoss, "maximum dynamic stop loss pct; 0 disables cap")
		cooldownBars   = flag.Int("cooldown-bars", defaultPaperCooldownBars, "cooldown bars after scalp-tpsl exit")
		minSpreadPct   = flag.Float64("min-trend-spread-pct", defaultPaperMinTrendSpread, "minimum SMA spread pct required to enter scalp-tpsl trades")
		confirmBars    = flag.Int("confirm-bars", 1, "consecutive close direction bars required to enter scalp-tpsl trades")
		atrWindow      = flag.Int("atr-window", defaultPaperATRWindow, "ATR window for scalp-tpsl volatility filter")
		minATRPct      = flag.Float64("min-atr-pct", defaultPaperMinATRPct, "minimum ATR pct required to enter scalp-tpsl trades")
		maxATRPct      = flag.Float64("max-atr-pct", defaultPaperMaxATRPct, "maximum ATR pct allowed to enter scalp-tpsl trades")
		volumeWindow   = flag.Int("volume-window", defaultPaperVolumeWindow, "volume average window for scalp-tpsl volume filter")
		minVolume      = flag.Float64("min-volume-ratio", defaultPaperMinVolumeRatio, "minimum current volume / average volume required to enter scalp-tpsl trades")
		maxExtension   = flag.Float64("max-entry-extension-pct", defaultPaperMaxExtensionPct, "maximum entry distance from fast SMA pct; zero disables")
		pullbackBars   = flag.Int("pullback-lookback", defaultPaperPullbackBars, "recent bars that must touch fast SMA zone before entry; zero disables")
		pullbackTol    = flag.Float64("pullback-tolerance-pct", defaultPaperPullbackTolPct, "pullback touch tolerance pct around fast SMA")
		feeRate        = flag.Float64("fee-rate", defaultPaperFeeRate, "fee rate per trade side")
		slippageRate   = flag.Float64("slippage-rate", defaultPaperSlippageRate, "slippage rate per trade side")
		equity         = flag.Float64("equity", 10000, "paper account equity")
		quantity       = flag.Float64("quantity", defaultPaperQuantity, "paper order quantity")
		riskPct        = flag.Float64("risk-pct", defaultPaperRiskPct, "risk pct of equity per trade; overrides -quantity when positive")
		maxNotional    = flag.Float64("max-notional-pct", 100, "maximum notional as pct of equity for paper order sizing")
		maxMargin      = flag.Float64("max-margin-pct", risk.DefaultConfig().MaxInitialMarginPct, "maximum total initial margin as pct of equity")
		maxBalanceUse  = flag.Float64("max-balance-use-pct", risk.DefaultConfig().MaxAvailableBalanceUsePct, "maximum order initial margin as pct of available balance")
		minLiqDist     = flag.Float64("min-liquidation-distance-pct", risk.DefaultConfig().MinLiquidationDistancePct, "minimum estimated liquidation distance pct")
		maintMargin    = flag.Float64("maintenance-margin-rate-pct", risk.DefaultConfig().MaintenanceMarginRatePct, "maintenance margin rate pct used for paper liquidation estimate")
		maxOrderRisk   = flag.Float64("max-order-risk-pct", risk.DefaultConfig().MaxOrderRiskPct, "maximum order stop loss risk as pct of equity")
		maxFunding     = flag.Float64("max-abs-funding-rate-pct", risk.DefaultConfig().MaxAbsFundingRatePct, "maximum absolute funding rate pct allowed for new perpetual entries")
		maxLeverage    = flag.Float64("max-leverage", risk.DefaultConfig().MaxLeverage, "maximum allowed paper leverage")
		leverage       = flag.Float64("leverage", 1, "paper position leverage")
		signalFilter   = flag.Bool("signal-filter", defaultPaperSignalFilter, "filter low-quality entry signals using a feature score")
		minSignal      = flag.Float64("min-signal-score", defaultPaperMinSignalScore, "minimum 0-1 signal quality score required for new entries")
		maxDataAge     = flag.Duration("max-market-data-age", defaultPaperMaxDataAge, "legacy maximum age for latest candle and mark price; 0 uses split candle/mark limits")
		maxCandleAge   = flag.Duration("max-candle-age", defaultPaperMaxCandleAge, "maximum allowed age for latest closed signal candle; 0 disables candle freshness checks")
		maxMarkAge     = flag.Duration("max-mark-price-age", defaultPaperMaxMarkAge, "maximum allowed age for latest mark price; 0 disables mark freshness checks")
		requireMark    = flag.Bool("require-mark-price", true, "require latest OneBullEx mark price before paper settlement or entry")
		minQuantity    = flag.Float64("min-order-quantity", defaultPaperMinQuantity, "minimum order quantity before opening paper positions")
		quantityStep   = flag.Float64("quantity-step", defaultPaperQuantityStep, "quantity step used to round paper order size down; 0 disables")
		priceTick      = flag.Float64("price-tick-size", defaultPaperPriceTickSize, "price tick size used to align paper entry/TP/SL prices; 0 disables")
		submitExchange = flag.Bool("submit-exchange", false, "submit allowed open/close orders to OneBullEx; also requires ONEBULLEX_LIVE_TRADING=true")
		watch          = flag.Bool("watch", false, "keep running paper strategy on latest local market data")
		pollEvery      = flag.Duration("poll-interval", 15*time.Second, "poll interval when -watch is enabled")
	)
	flag.Parse()
	setFlags := visitedFlagNames()
	if err := applyPaperProfile(*profile, setFlags, paperProfileFlags{
		strategyID:    strategyID,
		market:        market,
		interval:      interval,
		strategyType:  strategyType,
		fast:          fast,
		slow:          slow,
		takeProfitPct: takeProfitPct,
		stopLossPct:   stopLossPct,
		dynamicTPSL:   dynamicTPSL,
		takeATRMult:   takeATRMult,
		stopATRMult:   stopATRMult,
		minTPPct:      minTPPct,
		maxTPPct:      maxTPPct,
		minSLPct:      minSLPct,
		maxSLPct:      maxSLPct,
		cooldownBars:  cooldownBars,
		minSpreadPct:  minSpreadPct,
		confirmBars:   confirmBars,
		atrWindow:     atrWindow,
		minATRPct:     minATRPct,
		maxATRPct:     maxATRPct,
		volumeWindow:  volumeWindow,
		minVolume:     minVolume,
		maxExtension:  maxExtension,
		pullbackBars:  pullbackBars,
		pullbackTol:   pullbackTol,
		feeRate:       feeRate,
		slippageRate:  slippageRate,
		riskPct:       riskPct,
		maxNotional:   maxNotional,
		maxMargin:     maxMargin,
		maxBalanceUse: maxBalanceUse,
		minLiqDist:    minLiqDist,
		maxOrderRisk:  maxOrderRisk,
		maxLeverage:   maxLeverage,
		leverage:      leverage,
		signalFilter:  signalFilter,
		minSignal:     minSignal,
	}); err != nil {
		return err
	}
	actualStrategyID := *strategyID
	if (actualStrategyID == "sma-paper" || actualStrategyID == "scalp-tpsl-paper") && strings.EqualFold(*strategyType, "scalp-tpsl") {
		actualStrategyID = defaultPaperStrategyID
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
		AccountID:            *accountID,
		StrategyID:           actualStrategyID,
		Profile:              normalizedPaperProfile(*profile),
		Exchange:             normalizeExchangeName(*exchange),
		MarketType:           *market,
		Symbol:               *symbol,
		Interval:             *interval,
		Start:                startTime,
		End:                  endTime,
		StrategyType:         *strategyType,
		FastWindow:           *fast,
		SlowWindow:           *slow,
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
		Equity:               *equity,
		Quantity:             *quantity,
		RiskPct:              *riskPct,
		MaxNotionalPct:       *maxNotional,
		MaxMarginPct:         *maxMargin,
		MaxBalanceUsePct:     *maxBalanceUse,
		MinLiqDistancePct:    *minLiqDist,
		MaintMarginPct:       *maintMargin,
		MaxOrderRiskPct:      *maxOrderRisk,
		MaxAbsFundingRatePct: *maxFunding,
		MaxLeverage:          *maxLeverage,
		Leverage:             *leverage,
		SignalFilterEnabled:  *signalFilter,
		MinSignalScore:       *minSignal,
		MaxMarketDataAge:     *maxDataAge,
		MaxCandleAge:         *maxCandleAge,
		MaxMarkPriceAge:      *maxMarkAge,
		RequireMarkPrice:     *requireMark,
		MinOrderQuantity:     *minQuantity,
		QuantityStep:         *quantityStep,
		PriceTickSize:        *priceTick,
		SubmitExchange:       *submitExchange,
		Watch:                *watch,
		PollInterval:         *pollEvery,
		LookbackCandles:      120,
	}
	if config.Exchange != onebullex.ExchangeName {
		return fmt.Errorf("paper strategy currently supports %s only", onebullex.ExchangeName)
	}
	if config.MinSignalScore < 0 || config.MinSignalScore > 1 {
		return fmt.Errorf("min signal score must be between 0 and 1")
	}

	if config.Watch {
		return watchPaperStrategy(context.Background(), db, config)
	}

	return runPaperStrategyOnce(ctx, db, config)
}

func visitedFlagNames() map[string]struct{} {
	visited := make(map[string]struct{})
	flag.Visit(func(f *flag.Flag) {
		visited[f.Name] = struct{}{}
	})
	return visited
}

func applyPaperProfile(profile string, visited map[string]struct{}, flags paperProfileFlags) error {
	switch normalizedPaperProfile(profile) {
	case "":
		return nil
	case "aggressive":
		setStringFlag(visited, "strategy", flags.strategyID, defaultPaperStrategyID)
		setStringFlag(visited, "market", flags.market, "perpetual")
		setStringFlag(visited, "interval", flags.interval, "3m")
		setStringFlag(visited, "strategy-type", flags.strategyType, "scalp-tpsl")
		setIntFlag(visited, "fast", flags.fast, defaultPaperFastWindow)
		setIntFlag(visited, "slow", flags.slow, defaultPaperSlowWindow)
		setFloatFlag(visited, "take-profit-pct", flags.takeProfitPct, defaultPaperTakeProfitPct)
		setFloatFlag(visited, "stop-loss-pct", flags.stopLossPct, defaultPaperStopLossPct)
		setBoolFlag(visited, "dynamic-tpsl", flags.dynamicTPSL, true)
		setFloatFlag(visited, "take-profit-atr-mult", flags.takeATRMult, 1.8)
		setFloatFlag(visited, "stop-loss-atr-mult", flags.stopATRMult, 1.0)
		setFloatFlag(visited, "min-take-profit-pct", flags.minTPPct, 0.60)
		setFloatFlag(visited, "max-take-profit-pct", flags.maxTPPct, 1.60)
		setFloatFlag(visited, "min-stop-loss-pct", flags.minSLPct, defaultPaperMinStopLoss)
		setFloatFlag(visited, "max-stop-loss-pct", flags.maxSLPct, defaultPaperMaxStopLoss)
		setIntFlag(visited, "cooldown-bars", flags.cooldownBars, defaultPaperCooldownBars)
		setFloatFlag(visited, "min-trend-spread-pct", flags.minSpreadPct, defaultPaperMinTrendSpread)
		setIntFlag(visited, "confirm-bars", flags.confirmBars, 1)
		setIntFlag(visited, "atr-window", flags.atrWindow, defaultPaperATRWindow)
		setFloatFlag(visited, "min-atr-pct", flags.minATRPct, defaultPaperMinATRPct)
		setFloatFlag(visited, "max-atr-pct", flags.maxATRPct, defaultPaperMaxATRPct)
		setIntFlag(visited, "volume-window", flags.volumeWindow, defaultPaperVolumeWindow)
		setFloatFlag(visited, "min-volume-ratio", flags.minVolume, 1.15)
		setFloatFlag(visited, "max-entry-extension-pct", flags.maxExtension, defaultPaperMaxExtensionPct)
		setIntFlag(visited, "pullback-lookback", flags.pullbackBars, defaultPaperPullbackBars)
		setFloatFlag(visited, "pullback-tolerance-pct", flags.pullbackTol, defaultPaperPullbackTolPct)
		setFloatFlag(visited, "fee-rate", flags.feeRate, defaultPaperFeeRate)
		setFloatFlag(visited, "slippage-rate", flags.slippageRate, defaultPaperSlippageRate)
		setFloatFlag(visited, "risk-pct", flags.riskPct, 2)
		setFloatFlag(visited, "max-notional-pct", flags.maxNotional, 220)
		setFloatFlag(visited, "max-margin-pct", flags.maxMargin, 65)
		setFloatFlag(visited, "max-balance-use-pct", flags.maxBalanceUse, 90)
		setFloatFlag(visited, "min-liquidation-distance-pct", flags.minLiqDist, 15)
		setFloatFlag(visited, "max-order-risk-pct", flags.maxOrderRisk, 2.5)
		setFloatFlag(visited, "max-leverage", flags.maxLeverage, 3)
		setFloatFlag(visited, "leverage", flags.leverage, 3)
		setBoolFlag(visited, "signal-filter", flags.signalFilter, true)
		setFloatFlag(visited, "min-signal-score", flags.minSignal, 0.50)
		return nil
	default:
		return fmt.Errorf("unsupported paper profile %q", profile)
	}
}

func normalizedPaperProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", "manual", "none", "default":
		return ""
	case "aggressive", "small-aggressive", "small_aggressive", "small":
		return "aggressive"
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}

func setStringFlag(visited map[string]struct{}, name string, target *string, value string) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setIntFlag(visited map[string]struct{}, name string, target *int, value int) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setFloatFlag(visited map[string]struct{}, name string, target *float64, value float64) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

func setBoolFlag(visited map[string]struct{}, name string, target *bool, value bool) {
	if _, ok := visited[name]; !ok {
		*target = value
	}
}

type paperRunConfig struct {
	AccountID            string
	StrategyID           string
	Profile              string
	Exchange             string
	MarketType           string
	Symbol               string
	Interval             string
	Start                time.Time
	End                  time.Time
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
	Equity               float64
	Quantity             float64
	RiskPct              float64
	MaxNotionalPct       float64
	MaxMarginPct         float64
	MaxBalanceUsePct     float64
	MinLiqDistancePct    float64
	MaintMarginPct       float64
	MaxOrderRiskPct      float64
	MaxAbsFundingRatePct float64
	MaxLeverage          float64
	Leverage             float64
	SignalFilterEnabled  bool
	MinSignalScore       float64
	MaxMarketDataAge     time.Duration
	MaxCandleAge         time.Duration
	MaxMarkPriceAge      time.Duration
	RequireMarkPrice     bool
	MinOrderQuantity     float64
	QuantityStep         float64
	PriceTickSize        float64
	SubmitExchange       bool
	Watch                bool
	PollInterval         time.Duration
	LookbackCandles      int
}

type paperMarketSnapshot struct {
	Price                float64
	PriceTime            time.Time
	PriceSource          string
	CandleClosePrice     float64
	CandleCloseTime      time.Time
	MarkPrice            float64
	MarkPriceTime        time.Time
	LatestFundingRatePct float64
	FundingRateTime      time.Time
	FundingRateSource    string
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
		"max_abs_funding_rate_pct": config.MaxAbsFundingRatePct,
		"max_leverage":             config.MaxLeverage,
		"leverage":                 config.Leverage,
		"signal_filter_enabled":    config.SignalFilterEnabled,
		"min_signal_score":         config.MinSignalScore,
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
	} else if isEntryAction(action) && !assessment.AllowEntry {
		action = strategy.SignalHold
	}
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
		"max_abs_funding_rate_pct": config.MaxAbsFundingRatePct,
		"signal_filter_enabled":    config.SignalFilterEnabled,
		"min_signal_score":         config.MinSignalScore,
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
	FastWindow      int
	SlowWindow      int
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
	state := paperAccountState{
		Equity:           request.Equity + realizedPnL,
		AvailableBalance: request.Equity + realizedPnL,
		TotalPnL:         realizedPnL,
		RealizedPnL:      realizedPnL,
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
		closed, err := repo.ClosePaperPositionWithRealizedPnL(ctx, position.ID, request.MarkPrice, request.MarkTime, realizedPnL)
		if err != nil {
			return paperAccountState{}, fmt.Errorf("close paper position: %w", err)
		}
		state.Position = closed
		state.RealizedPnL += closed.RealizedPnL
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
			return "stop_loss"
		}
		return ""
	}
	// Long TP/SL: take profit when mark price rises to TP; stop loss when it falls to SL.
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
		MaxDailyLossPct:           defaultRisk.MaxDailyLossPct,
		MaxConsecutiveLosses:      defaultRisk.MaxConsecutiveLosses,
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

func latestPaperMarketSnapshot(ctx context.Context, repo *marketdata.SQLiteRepository, config paperRunConfig, candles []marketdata.Candle) (paperMarketSnapshot, error) {
	candlePrice, candleTime, err := latestCandlePrice(candles)
	if err != nil {
		return paperMarketSnapshot{}, err
	}
	snapshot := paperMarketSnapshot{
		Price:             candlePrice,
		PriceTime:         candleTime,
		PriceSource:       "candle_close_fallback",
		CandleClosePrice:  candlePrice,
		CandleCloseTime:   candleTime,
		FundingRateSource: "missing",
	}

	mark, err := repo.LatestMarkPrice(ctx, config.Exchange, config.Symbol)
	if err == nil {
		markPrice, parseErr := parsePositiveFloat(mark.MarkPrice, "latest mark price")
		if parseErr != nil {
			return paperMarketSnapshot{}, parseErr
		}
		snapshot.Price = markPrice
		snapshot.PriceTime = mark.EventTime
		snapshot.PriceSource = "latest_mark_price"
		snapshot.MarkPrice = markPrice
		snapshot.MarkPriceTime = mark.EventTime
	} else if config.RequireMarkPrice || !errors.Is(err, marketdata.ErrNotFound) {
		return paperMarketSnapshot{}, fmt.Errorf("load latest mark price: %w", err)
	}

	rate, err := repo.LatestFundingRate(ctx, config.Exchange, config.Symbol)
	if err == nil {
		fundingRatePct, parseErr := parseFundingRatePct(rate.FundingRate)
		if parseErr != nil {
			return paperMarketSnapshot{}, parseErr
		}
		snapshot.LatestFundingRatePct = fundingRatePct
		snapshot.FundingRateTime = rate.FundingTime
		snapshot.FundingRateSource = "latest_funding_rate"
	} else if !errors.Is(err, marketdata.ErrNotFound) {
		return paperMarketSnapshot{}, fmt.Errorf("load latest funding rate: %w", err)
	}

	if err := validatePaperMarketFreshness(snapshot, time.Now().UTC(), config); err != nil {
		return paperMarketSnapshot{}, err
	}
	return snapshot, nil
}

func validatePaperMarketFreshness(snapshot paperMarketSnapshot, now time.Time, config paperRunConfig) error {
	candleAgeLimit := config.MaxCandleAge
	markAgeLimit := config.MaxMarkPriceAge
	if config.MaxMarketDataAge > 0 {
		candleAgeLimit = config.MaxMarketDataAge
		markAgeLimit = config.MaxMarketDataAge
	}
	if candleAgeLimit > 0 {
		if err := validatePaperDataAge("latest candle close", snapshot.CandleCloseTime, now, candleAgeLimit); err != nil {
			return err
		}
	}
	if snapshot.PriceSource == "latest_mark_price" && markAgeLimit > 0 {
		return validatePaperDataAge(snapshot.PriceSource, snapshot.PriceTime, now, markAgeLimit)
	}
	return nil
}

func validatePaperDataAge(label string, eventTime time.Time, now time.Time, maxAge time.Duration) error {
	if eventTime.IsZero() {
		return fmt.Errorf("%s time is empty", label)
	}
	age := now.Sub(eventTime)
	if age < 0 {
		age = -age
	}
	if age > maxAge {
		return fmt.Errorf("%s is stale: age=%s max=%s", label, age.Truncate(time.Second), maxAge)
	}
	return nil
}

func latestCandlePrice(candles []marketdata.Candle) (float64, time.Time, error) {
	if len(candles) == 0 {
		return 0, time.Time{}, backtest.ErrNotEnoughData
	}
	last := candles[len(candles)-1]
	price, err := parsePositiveFloat(last.Close, "latest close price")
	if err != nil {
		return 0, time.Time{}, err
	}
	eventTime := last.CloseTime
	if eventTime.IsZero() {
		eventTime = last.OpenTime
	}
	return price, eventTime, nil
}

func parsePositiveFloat(value string, name string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid %s %s", name, value)
	}
	return parsed, nil
}

func parseFundingRatePct(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("parse funding rate: %w", err)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid funding rate %s", value)
	}
	if math.Abs(parsed) <= 1 {
		return parsed * 100, nil
	}
	return parsed, nil
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
		return backtest.RunScalpTPSL(candles, paperBacktestScalpConfig(config))
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

func paperBacktestScalpConfig(config paperStrategyConfig) backtest.ScalpTPSLConfig {
	return backtest.ScalpTPSLConfig{
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
		AllowShort:           strings.EqualFold(config.MarketType, "perpetual"),
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

type paperSignalAssessment struct {
	Score      float64
	AllowEntry bool
	Reason     string
	Features   map[string]any
}

type paperSignalFeatureValues struct {
	LatestClose       float64
	FastAverage       float64
	SlowAverage       float64
	TrendSpreadPct    float64
	ATRPct            float64
	VolumeRatio       float64
	EntryExtensionPct float64
	FundingAbsPct     float64
	MarkBasisPct      float64
	HasATR            bool
	HasVolume         bool
	HasMarkBasis      bool
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
	side, ok, err := backtest.LatestScalpTPSLSignal(candles, paperBacktestScalpConfig(config))
	if err != nil || !ok {
		return paperSignal{Action: strategy.SignalHold}
	}
	switch side {
	case "long":
		return paperSignal{Action: strategy.SignalBuy, PositionSide: "long"}
	case "short":
		return paperSignal{Action: strategy.SignalShort, PositionSide: "short"}
	}
	return paperSignal{Action: strategy.SignalHold}
}

func assessPaperSignal(candles []marketdata.Candle, result backtest.Result, signal paperSignal, snapshot paperMarketSnapshot, config paperRunConfig) paperSignalAssessment {
	features, values, ok := paperSignalFeatures(candles, signal, snapshot, config)
	if !isEntryAction(signal.Action) {
		return paperSignalAssessment{
			Score:      0,
			AllowEntry: false,
			Reason:     "no_entry_signal",
			Features:   features,
		}
	}
	if !ok {
		return paperSignalAssessment{
			Score:      0.35,
			AllowEntry: !config.SignalFilterEnabled,
			Reason:     "feature_unavailable",
			Features:   features,
		}
	}

	score := 0.50
	score += paperTrendScore(values.TrendSpreadPct, config.MinTrendSpreadPct)
	score += paperATRScore(values.ATRPct, values.HasATR, config.MinATRPct, config.MaxATRPct)
	score += paperVolumeScore(values.VolumeRatio, values.HasVolume, config.MinVolumeRatio)
	score += paperExtensionScore(values.EntryExtensionPct, config.MaxEntryExtensionPct)
	score += paperFundingScore(values.FundingAbsPct, config.MaxAbsFundingRatePct)
	score += paperMarkBasisScore(values.MarkBasisPct, values.HasMarkBasis)
	score += paperBacktestScore(result)
	score = clamp01(score)

	allowed := !config.SignalFilterEnabled || score >= config.MinSignalScore
	reason := "score_pass"
	if !config.SignalFilterEnabled {
		reason = "filter_disabled"
	} else if !allowed {
		reason = "score_below_min"
	}
	features["score_components_version"] = "paper_signal_filter_v1"
	return paperSignalAssessment{
		Score:      score,
		AllowEntry: allowed,
		Reason:     reason,
		Features:   features,
	}
}

func paperSignalFeatures(candles []marketdata.Candle, signal paperSignal, snapshot paperMarketSnapshot, config paperRunConfig) (map[string]any, paperSignalFeatureValues, bool) {
	features := map[string]any{
		"feature_version": "paper_signal_features_v1",
	}
	values := paperSignalFeatureValues{}
	if len(candles) == 0 {
		features["feature_error"] = "no_candles"
		return features, values, false
	}
	last := candles[len(candles)-1]
	closePrice, err := parsePositiveFloat(last.Close, "latest close price")
	if err != nil {
		features["feature_error"] = "invalid_close"
		return features, values, false
	}
	fastAverage, slowAverage, ok := latestAverages(candles, config.FastWindow, config.SlowWindow)
	if !ok {
		features["feature_error"] = "not_enough_averages"
		return features, values, false
	}

	direction := 1.0
	if signal.Action == strategy.SignalShort {
		direction = -1
	}
	values.LatestClose = closePrice
	values.FastAverage = fastAverage
	values.SlowAverage = slowAverage
	values.TrendSpreadPct = math.Abs(fastAverage-slowAverage) / closePrice * 100
	values.EntryExtensionPct = (closePrice - fastAverage) / closePrice * 100
	if direction < 0 {
		values.EntryExtensionPct = (fastAverage - closePrice) / closePrice * 100
	}
	values.FundingAbsPct = math.Abs(snapshot.LatestFundingRatePct)
	if snapshot.MarkPrice > 0 {
		values.MarkBasisPct = math.Abs(snapshot.MarkPrice-closePrice) / closePrice * 100
		values.HasMarkBasis = true
	}
	if atrPct, ok := latestPaperATRPct(candles, config.ATRWindow); ok {
		values.ATRPct = atrPct
		values.HasATR = true
	}
	if ratio, ok := latestPaperVolumeRatio(candles, config.VolumeWindow); ok {
		values.VolumeRatio = ratio
		values.HasVolume = true
	}

	features["latest_close"] = values.LatestClose
	features["fast_average"] = values.FastAverage
	features["slow_average"] = values.SlowAverage
	features["trend_spread_pct"] = values.TrendSpreadPct
	features["entry_extension_pct"] = values.EntryExtensionPct
	features["funding_abs_pct"] = values.FundingAbsPct
	features["atr_pct"] = nullableFeature(values.ATRPct, values.HasATR)
	features["volume_ratio"] = nullableFeature(values.VolumeRatio, values.HasVolume)
	features["mark_basis_pct"] = nullableFeature(values.MarkBasisPct, values.HasMarkBasis)
	features["direction"] = signal.PositionSide
	return features, values, true
}

func nullableFeature(value float64, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func paperTrendScore(spreadPct float64, minSpreadPct float64) float64 {
	if minSpreadPct <= 0 {
		if spreadPct > 0.03 {
			return 0.08
		}
		return 0
	}
	if spreadPct < minSpreadPct {
		return -0.20
	}
	if spreadPct >= minSpreadPct*2 {
		return 0.14
	}
	if spreadPct >= minSpreadPct*1.5 {
		return 0.10
	}
	return 0.06
}

func paperATRScore(atrPct float64, ok bool, minATRPct float64, maxATRPct float64) float64 {
	if !ok {
		return -0.08
	}
	if minATRPct > 0 && atrPct < minATRPct {
		return -0.18
	}
	if maxATRPct > 0 && atrPct > maxATRPct {
		return -0.16
	}
	return 0.10
}

func paperVolumeScore(volumeRatio float64, ok bool, minVolumeRatio float64) float64 {
	if minVolumeRatio <= 0 {
		return 0
	}
	if !ok {
		return -0.06
	}
	if volumeRatio >= minVolumeRatio*1.5 {
		return 0.10
	}
	if volumeRatio >= minVolumeRatio {
		return 0.07
	}
	if volumeRatio >= 1 {
		return 0.02
	}
	return -0.12
}

func paperExtensionScore(extensionPct float64, maxExtensionPct float64) float64 {
	if maxExtensionPct <= 0 {
		return 0
	}
	if extensionPct <= maxExtensionPct {
		return 0.08
	}
	if extensionPct <= maxExtensionPct*1.5 {
		return -0.04
	}
	return -0.14
}

func paperFundingScore(absFundingPct float64, maxAbsFundingPct float64) float64 {
	if maxAbsFundingPct <= 0 {
		return 0
	}
	if absFundingPct <= maxAbsFundingPct*0.5 {
		return 0.05
	}
	if absFundingPct <= maxAbsFundingPct {
		return 0.01
	}
	return -0.15
}

func paperMarkBasisScore(markBasisPct float64, ok bool) float64 {
	if !ok {
		return -0.03
	}
	if markBasisPct <= 0.03 {
		return 0.04
	}
	if markBasisPct <= 0.10 {
		return 0
	}
	return -0.08
}

func paperBacktestScore(result backtest.Result) float64 {
	score := 0.0
	if result.ExcessReturnPct > 0 {
		score += math.Min(result.ExcessReturnPct/100, 0.06)
	} else if result.ExcessReturnPct < 0 {
		score -= math.Min(math.Abs(result.ExcessReturnPct)/100, 0.08)
	}
	if len(result.Trades) >= 5 {
		if result.WinRatePct >= 55 {
			score += 0.04
		} else if result.WinRatePct < 40 {
			score -= 0.05
		}
	}
	return score
}

func latestPaperATRPct(candles []marketdata.Candle, window int) (float64, bool) {
	if window <= 0 || len(candles) < window+1 {
		return 0, false
	}
	closes := make([]float64, 0, len(candles))
	highs := make([]float64, 0, len(candles))
	lows := make([]float64, 0, len(candles))
	for _, candle := range candles {
		closePrice, err := parsePositiveFloat(candle.Close, "close price")
		if err != nil {
			return 0, false
		}
		high, err := parsePositiveFloat(candle.High, "high price")
		if err != nil {
			return 0, false
		}
		low, err := parsePositiveFloat(candle.Low, "low price")
		if err != nil || low > high {
			return 0, false
		}
		closes = append(closes, closePrice)
		highs = append(highs, high)
		lows = append(lows, low)
	}
	index := len(candles) - 1
	total := 0.0
	for i := index - window + 1; i <= index; i++ {
		highLow := highs[i] - lows[i]
		highPrevClose := math.Abs(highs[i] - closes[i-1])
		lowPrevClose := math.Abs(lows[i] - closes[i-1])
		total += math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
	}
	return total / float64(window) / closes[index] * 100, true
}

func latestPaperVolumeRatio(candles []marketdata.Candle, window int) (float64, bool) {
	if window <= 0 || len(candles) < window+1 {
		return 0, false
	}
	index := len(candles) - 1
	total := 0.0
	for i := index - window; i < index; i++ {
		volume, err := strconv.ParseFloat(candles[i].Volume, 64)
		if err != nil || volume < 0 || math.IsNaN(volume) || math.IsInf(volume, 0) {
			return 0, false
		}
		total += volume
	}
	current, err := strconv.ParseFloat(candles[index].Volume, 64)
	if err != nil || current < 0 || math.IsNaN(current) || math.IsInf(current, 0) {
		return 0, false
	}
	avg := total / float64(window)
	if avg <= 0 {
		return 0, false
	}
	return current / avg, true
}

func isEntryAction(action strategy.SignalAction) bool {
	return action == strategy.SignalBuy || action == strategy.SignalShort
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
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

func paperSignalReason(strategyType string, assessment paperSignalAssessment) string {
	if normalizedStrategyType(strategyType) == "scalp-tpsl" {
		if assessment.Reason != "" && assessment.Reason != "score_pass" {
			return "paper_scalp_tpsl_" + assessment.Reason
		}
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
	return config.Env(key, fallback)
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(env(key, "")))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func normalizeExchangeName(exchangeName string) string {
	exchangeName = strings.ToLower(strings.TrimSpace(exchangeName))
	if exchangeName == "onebull" || exchangeName == "1bullex" {
		return onebullex.ExchangeName
	}
	return exchangeName
}
